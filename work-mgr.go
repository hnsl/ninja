package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path"
	"regexp"
	"strconv"
	"strings"
)

type areaID string
type turtleID string

// An itemID is: item.name + "/" + item.metadata
type itemID string

type turtle struct {
	NewKernel      bool `json:"new_kernel"`
	Version        int
	Label          turtleID
	CurAction      string  `json:"cur_action"`
	CurDst         vec3    `json:"cur_dst"`
	CurBestDist    int     `json:"cur_best_dist"`
	CurPivot       []int   `json:"cur_pivot"`
	CurFrustration int     `json:"cur_frustration"`
	CurPos         vec3    `json:"cur_pos"`
	CurRot         vec3    `json:"cur_rot"`
	CurWork        *work   `json:"cur_work"`
	FatalErr       string  `json:"fatal_err"`
	RefuelErr      float64 `json:"refuel_err"`
	FuelLvl        int     `json:"fuel_lvl"`
	InvCount       icount  `json:"inv_count"`
}

type work struct {
	ID       workID
	Type     string
	Complete bool
}

type workID int

const (
	// Special work ID for temporary jobs that don't need to be cancelled
	// or tracked, e.g. go to coordinate.
	workIDTmp = workID(-1)

	// Special work IDs for low priority jobs that should be cancelable,
	// e.g. queue indefinitely or wait until something happens.

	workIDInvImpQueue = workID(-2) // Inventory import queueing.
	workIDInvImpSuck  = workID(-3) // Inventory import sucking.
)

func (w workID) isLowPriority() bool {
	switch w {
	case workIDInvImpQueue, workIDInvImpSuck:
		return true
	default:
		return false
	}
}

type icount struct {
	FreeSlots int `json:"free_slots"`
	Grouped   map[itemID]int
}

type qCoords struct {
	face_dir  vec3 // direction to face when reached queue end
	origin    vec3 // final position in queue
	q_dir     vec3 // direction of queue
	o_q0_dir  vec3 // o -> q0 direction
	q0_t0_dir vec3 // q0 -> t0 direction
}

var areas = map[areaID]interface{}{}

type workRequest struct {
	t      turtle
	rsp_ch chan *string
}

var work_mgr_ch = make(chan interface{}, 0)

func decideWork(t turtle) *string {
	req := workRequest{
		t:      t,
		rsp_ch: make(chan *string, 1),
	}
	work_mgr_ch <- req
	return <-req.rsp_ch
}

type exportRequest struct {
	ItemID itemID `json:"item_id"`
	Count  int
	AreaID areaID    `json:"area_id"`
	rsp_ch chan bool `json:"-"`
}

func exportItems(er exportRequest) bool {
	er.rsp_ch = make(chan bool, 1)
	work_mgr_ch <- er
	return <-er.rsp_ch
}

type exitRequest struct{}

func workMgrExit() {
	// Because channel has size 0 this blocks until exit request is received
	// and it's safe to return from function.
	work_mgr_ch <- exitRequest{}
}

func workMgrGo() {
	for {
		req := <-work_mgr_ch
		switch req := req.(type) {
		case workRequest:
			req.rsp_ch <- mgrDecideWork(req.t)
		case exportRequest:
			req.rsp_ch <- mgrHandleExport(req)
		case exitRequest:
			return
		}
	}
}

func mgrHandleExport(er exportRequest) bool {
	area := areas[er.AreaID]
	if area == nil {
		log.Printf("handle export: error: invalid area id: %v", er.AreaID)
		return false
	}
	switch area := area.(type) {
	case *storageArea:
		s := area
		new_count := s.Exporting[er.ItemID] + er.Count
		n_total := 0
		for _, box := range s.Boxes {
			if box.Amount < 0 {
				continue
			}
			if box.Name == er.ItemID {
				n_total += box.Amount
			}
		}
		if new_count > n_total {
			new_count = n_total
		}
		if new_count <= 0 {
			delete(s.Exporting, er.ItemID)
		} else {
			s.Exporting[er.ItemID] = new_count
		}
		s.store()
		return true
	default:
		log.Printf("handle export: error: do not understand area type: %T", area)
		return false
	}
}

func mgrDecideWork(t turtle) *string {
	label_parts := strings.Split(string(t.Label), ".")
	if len(label_parts) != 3 {
		log.Printf("decide work: error: invalid turtle id: %v", t.Label)
		return nil
	}
	area_id := areaID(strings.Join(label_parts[0:2], "."))
	area := areas[area_id]
	if len(label_parts) != 3 {
		log.Printf("decide work: error: invalid turtle area id: %v", t.Label)
		return nil
	}
	switch area := area.(type) {
	case *storageArea:
		return mgrDecideStorageWork(t, area)
	case *mineArea:
		return mgrDecideMineWork(t, area)
	default:
		log.Printf("decide work: error: do not understand area type: %T", area)
		return nil
	}
}

func pathSyncKey(fs_path string) string {
	return fmt.Sprintf("%s/%s", path.Base(path.Dir(fs_path)), path.Base(fs_path))
}

func storeJSON(fs_path string, src interface{}) {
	raw, err := json.MarshalIndent(src, "", "\t")
	check(err)
	syncNotify(pathSyncKey(fs_path), string(raw))
	err = ioutil.WriteFile(fs_path, raw, 0644)
	check(err)
}

func loadJSON(fs_path string, dst interface{}) {
	raw, err := ioutil.ReadFile(fs_path)
	check(err)
	syncNotify(pathSyncKey(fs_path), string(raw))
	err = json.Unmarshal(raw, dst)
	check(err)
}

var rgx_row = regexp.MustCompile("^plane.([0-9]+)$")

func loadArea(area_id areaID, area_dir string) {
	area_parts := strings.Split(string(area_id), ".")
	if len(area_parts) != 2 {
		panic(fmt.Sprintf("invalid area id: %v", area_id))
	}
	area_type := area_parts[0]
	switch area_type {
	case "mine":
		m := new(mineArea)
		m.Path = fmt.Sprintf("%s/details", area_dir)
		loadJSON(m.Path, m)
		if m.ID != area_id {
			panic(fmt.Sprintf("invalid mine id: %v, expected: %v", m.ID, area_id))
		}
		if m.MineProgress == nil {
			m.MineProgress = map[string][5]boreholeState{}
		}
		if m.MineAllocs == nil {
			m.MineAllocs = map[turtleID]*mineOrder{}
		}
		// Write new area.
		areas[area_id] = m
		log.Printf("loaded mine: %v\n", m.ID)
	case "storage":
		s := new(storageArea)
		s.Path = fmt.Sprintf("%s/details", area_dir)
		loadJSON(s.Path, s)
		if s.ID != area_id {
			panic(fmt.Sprintf("invalid storage id: %v, expected: %v", s.ID, area_id))
		}
		if s.LoadOrders == nil {
			s.LoadOrders = map[turtleID]*loadOrder{}
		}
		if s.Exporting == nil {
			s.Exporting = map[itemID]int{}
		}
		if s.ExportAllocs == nil {
			s.ExportAllocs = map[turtleID]map[itemID]int{}
		}
		s.Boxes = make([]storageBox, s.nBoxes())
		files, err := ioutil.ReadDir(area_dir)
		check(err)
		// Read boxes.
		for _, file := range files {
			m := rgx_row.FindStringSubmatch(file.Name())
			if m == nil {
				continue
			}
			plane_id, err := strconv.Atoi(m[1])
			check(err)
			var row_boxes []storageBox
			loadJSON(fmt.Sprintf("%s/%s", area_dir, file.Name()), &row_boxes)
			for i, box := range row_boxes {
				s.Boxes[s.nBoxesPerPlane()*plane_id+i] = box
			}
		}
		// Initialize I/O hole.
		s.Boxes[4].Amount = -1
		s.Boxes[5].Amount = -1
		s.Boxes[6].Amount = -1
		// Write new area.
		areas[area_id] = s
		log.Printf("loaded storage: %v\n", s.ID)
		//fmt.Printf("loaded storage: %#v", s)
	default:
		panic(fmt.Sprintf("unknown area type: %v", area_type))
	}
}

func loadState(state_dir string) {
	area_dirs, err := ioutil.ReadDir(state_dir)
	check(err)
	for _, area_dir := range area_dirs {
		if !area_dir.IsDir() {
			continue
		}
		area_id := areaID(area_dir.Name())
		area_dir := fmt.Sprintf("%s/%s", state_dir, area_id)
		loadArea(area_id, area_dir)
	}
}
