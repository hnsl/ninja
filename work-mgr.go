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

type storageArea struct {
	Path string `json:"-"`
	ID   areaID
	// sequence counter for work ids
	WorkIDSeq int `json:"work_id_seq"`
	Pos       vec3
	XLen      int
	ZLen      int
	Rows      int
	// map from turtle labels to load orders that where assigned to them
	LoadOrders map[turtleID]*loadOrder `json:"load_orders"`
	Boxes      []storageBox            `json:"-"`
	// map from item ids to number of items to export
	Exporting map[itemID]int `json:"-"`
	// map from turtle ids to items and amount to export
	// an alloc here subtracts the corresponding exporting value
	ExportAllocs map[turtleID]map[itemID]int `json:"export_allocs"`
}

func (s storageArea) store() {
	storeJSON(s.Path, s)
}

func (s storageArea) nBoxesPerPlane() int {
	return (s.XLen*2 + s.ZLen*2)
}

func (s storageArea) nBoxes() int {
	return s.nBoxesPerPlane() * s.Rows
}

type qCoords struct {
	face_dir  vec3 // direction to face when reached queue end
	origin    vec3 // final position in queue
	q_dir     vec3 // direction of queue
	o_q0_dir  vec3 // o -> q0 direction
	q0_t0_dir vec3 // q0 -> t0 direction
}

func (s storageArea) getExportQ() qCoords {
	return qCoords{
		face_dir: vec3{0, 1, 0},
		origin: vec3{
			s.Pos[0] - 2,
			s.Pos[1] - 1,
			s.Pos[2] + 1,
		},
		q_dir:     vec3{0, 0, -1},
		o_q0_dir:  vec3{0, -1, 0},
		q0_t0_dir: vec3{-1, 0, 0},
	}
}

func (s storageArea) getImportQ() qCoords {
	return qCoords{
		face_dir: vec3{0, 1, 0},
		origin: vec3{
			s.Pos[0] - 4,
			s.Pos[1] - 1,
			s.Pos[2] + 1,
		},
		q_dir:     vec3{0, 0, -1},
		o_q0_dir:  vec3{0, -1, 0},
		q0_t0_dir: vec3{1, 0, 0},
	}
}

type boxOrient struct {
	boxPos  vec3
	loadDir vec3
}

func (bo boxOrient) loadPos() vec3 {
	return vec3Sub(bo.boxPos, bo.loadDir)
}

func (s storageArea) getBoxOrient(id int) boxOrient {
	out := boxOrient{}
	pp := s.nBoxesPerPlane()
	out.boxPos[1] = -id/pp - 2
	plane_id := id % pp
	if plane_id < s.XLen*2 {
		out.boxPos[0] = -s.XLen + (plane_id % s.XLen)
		if plane_id < s.XLen {
			out.boxPos[2] = -s.ZLen - 1
			out.loadDir = vec3{0, 0, -1}
		} else {
			out.boxPos[2] = 0
			out.loadDir = vec3{0, 0, 1}
		}
	} else {
		z_id := (plane_id - s.XLen*2)
		out.boxPos[2] = -s.ZLen + (z_id % s.ZLen)
		if z_id < s.ZLen {
			out.boxPos[0] = -s.XLen - 1
			out.loadDir = vec3{-1, 0, 0}
		} else {
			out.boxPos[0] = 0
			out.loadDir = vec3{1, 0, 0}
		}
	}
	// Convert relative position of box in inventory to world coordinate.
	out.boxPos = vec3Add(s.Pos, out.boxPos)
	// fmt.Printf("box orient %d %#v\n", id, out)
	return out
}

type boxCandidate struct {
	dist    int
	id      int
	item_id itemID
}

func (s storageArea) closestBox(pos vec3, item_id itemID, drop bool) *boxCandidate {
	var new, used *boxCandidate
	for id, box := range s.Boxes {
		if box.Amount < 0 {
			continue
		}
		getCand := func() *boxCandidate {
			return &boxCandidate{
				dist:    vec3L1Dist(pos, s.getBoxOrient(id).loadPos()),
				id:      id,
				item_id: item_id,
			}
		}
		if (drop && box.Amount == 0) || (!drop && box.Amount >= box.Capacity() && box.Name == item_id) {
			// New box candidate.
			// TODO: For drop we likely want to record a soft/temporary virtual
			// selection to avoid contention when dropping multiple resources
			// into new boxes by multiple turtles.
			cand := getCand()
			if new == nil || cand.dist < new.dist {
				new = cand
			}
		} else if box.Amount > 0 && box.Amount < box.Capacity() && box.Name == item_id {
			// Used box candidate.
			cand := getCand()
			if used == nil || cand.dist < used.dist {
				used = cand
			}
		}
	}
	// Prioritize used box candidates before new box candidates.
	if used != nil {
		return used
	}
	return new
}

func (s *storageArea) updateBox(box_id int, item_id itemID, delta int) {
	box := &s.Boxes[box_id]
	if box.Amount < 0 {
		panic("attempting to update hole box")
	}
	box.Amount += delta
	if box.Amount <= 0 {
		box.Amount = 0
		box.Name = ""
	} else {
		if box.Name == "" {
			box.Name = item_id
		} else if box.Name != item_id {
			panic(fmt.Sprintf("attempting to load %v in %v box", item_id, box.Name))
		}
	}
	// Write update plane to json.
	n_pp := s.nBoxesPerPlane()
	plane_id := box_id / n_pp
	box_plane := make([]storageBox, n_pp)
	for i := 0; i < n_pp; i++ {
		box_plane[i] = s.Boxes[n_pp*plane_id+i]
	}
	path := fmt.Sprintf("%s/plane.%d", path.Dir(s.Path), plane_id)
	storeJSON(path, box_plane)
}

const ExportVBoxID = -1

type loadOrder struct {
	ID    workID // work id
	BoxID int    `json:"box_id"` // ExportVBoxID = export drop
	// items to transfer. only one item type makes sense for suck operations
	// since items cannot be selectively extrated from containers.
	Items map[itemID]itemLoadCount
	Drop  bool // true = drop, false = suck
}

type itemLoadCount struct {
	// amount of this item before the load operation
	PreCount int `json:"pre_count"`
	// order abs(delta) to transfer
	AbsDelta int `json:"abs_delta"`
}

type storageBox struct {
	Amount int // -1 = hole, not allocatable
	Name   itemID
}

func (s *storageBox) Capacity() int {
	return 2048
}

var areas = map[areaID]interface{}{}

type workRequest struct {
	t      turtle
	rsp_ch chan *string
}

var work_ch = make(chan workRequest, 0)

func decideWork(t turtle) *string {
	req := workRequest{
		t:      t,
		rsp_ch: make(chan *string, 1),
	}
	work_ch <- req
	return <-req.rsp_ch
}

func workMgrGo() {
	for {
		req := <-work_ch
		req.rsp_ch <- mgrDecideWork(req.t)
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
	default:
		log.Printf("decide work: error: do not understand area type: %T", area)
		return nil
	}
}

func mgrDecideStorageWork(t turtle, s *storageArea) *string {
	// We generally do not assign work when an existing job is not completed,
	// except for low priority interruptible jobs that should always be
	// re-evaluated when reported in case another more important job is available.
	if t.CurWork != nil && !t.CurWork.ID.isLowPriority() && !t.CurWork.Complete {
		// Non interruptible work is not complete yet.
		job := ""
		return &job
	}
	pending_area_changes := false

	// Have we completed a box load that we should account for?
	if lo_ptr := s.LoadOrders[t.Label]; lo_ptr != nil {
		lo := *lo_ptr
		if t.CurWork == nil || (t.CurWork.ID != lo.ID && t.CurWork.Complete) {
			// Turtle did not get assigned work? Reassign load order.
			log.Printf("storage work warning: turtle %v: current load order work: %#v,"+
				" was unexpectedly not assigned", t.Label, lo)
			job := makeLoadOrderJob(*s, lo)
			return &job
		}
		if t.CurWork.ID != lo.ID ||
			(lo.Drop && t.CurWork.Type != "drop") ||
			(!lo.Drop && t.CurWork.Type != "suck") {
			log.Printf("storage work error: turtle %v: current work: %#v,"+
				" does not match current load order: %#v", t.Label, t.CurWork, lo)
			return nil
		}
		for item_id, lo_count := range lo.Items {
			// Calculate turtle inventory amount delta.
			cur_count := t.InvCount.Grouped[item_id]
			n_delta := cur_count - lo_count.PreCount
			if (n_delta < 0 && !lo.Drop) || (n_delta > 0 && lo.Drop) {
				log.Printf("storage work error: turtle %v: negative load (%v) %#v", t.Label, n_delta, lo)
				return nil
			}
			if lo.BoxID == ExportVBoxID {
				// Export, assert(lo.Drop).
				log_bad_export := func() {
					log.Printf("storage work error: turtle %v: bad export %#v"+
						", incompatible with %v", t.Label, lo, s.ExportAllocs)
				}
				item_map, ok := s.ExportAllocs[t.Label]
				if !ok {
					log_bad_export()
					return nil
				}
				item_amount, ok := item_map[item_id]
				if !ok || -n_delta > item_amount {
					log_bad_export()
					return nil
				}
				// Update remaining amount.
				remaining := item_amount + n_delta
				if remaining > 0 {
					item_map[item_id] = remaining
				} else {
					delete(item_map, item_id)
					if len(item_map) == 0 {
						delete(s.ExportAllocs, t.Label)
					}
				}
			} else {
				// Normal box load.
				box := s.Boxes[lo.BoxID]
				if box.Amount < n_delta || (box.Name != "" && box.Name != item_id) {
					log.Printf("storage work error: turtle %v: completed load is incompatible"+
						" with box: (%v), box: %#v, load order: %#v", t.Label, n_delta, box, lo)
					return nil
				}
				// Box delta is negative turtle delta.
				box_n_delta := -n_delta
				// Adjust box content.
				s.updateBox(lo.BoxID, item_id, box_n_delta)
			}
		}
		// Load order complete, remove it.
		delete(s.LoadOrders, t.Label)
		// Storing changes is pending.
		pending_area_changes = true
	}

	// Find new job.
	// First we need to divide our theoretical inventory into the subsets:
	// A = {inventory we have and don't want to export}
	// B = {inventory we have and want to export}
	// C = {inventory we don't have and want to export}
	// When we have free slots:
	//  - 1. When C has more than one item: suck them (select closest box).
	//  - 2. When B has more than one item: drop them (export all).
	//  - 3. When A has more than one item: drop it (select closest box).
	//  - 4. Queue for import.
	// When we have zero free slots:
	//  - 1. When A has more than one item: drop it (select closest box).
	//  - 2. When B has more than one item: drop them (export all).

	// Declare A, B and C.
	inv_a := map[itemID]int{}
	inv_b := map[itemID]int{}
	inv_c := map[itemID]int{}

	// Define import and export queue.
	import_q := s.getImportQ()
	export_q := s.getExportQ()

	// General handler for A/C cases.
	tryHandleAC := func(inv_x map[itemID]int, drop bool) *string {
		if len(inv_x) == 0 {
			return nil
		}
		// Find closest free box to drop for any item we want to drop.
		var cand *boxCandidate
		for item_id := range inv_x {
			used := s.closestBox(export_q.origin, item_id, true)
			if used == nil {
				log.Printf("storage work warning: turtle %v: no free box to unload junk %v", t.Label, item_id)
				continue
			}
			if cand == nil || used.dist < cand.dist {
				cand = used
			}
		}
		if cand == nil {
			return nil
		}
		// fmt.Printf("load candidate: %#v\n", cand)
		// Has a load candidate now.
		if !drop {
			// Allocate export of this item to this turtle if have not already.
			if s.ExportAllocs[t.Label][cand.item_id] > 0 {
				pending_area_changes = true
				n_export_me := inv_x[cand.item_id]
				n_export_tot := s.Exporting[cand.item_id]
				// assert(n_export_tot <= n_export_me) - inv_c cannot be defined with larger value
				s.Exporting[cand.item_id] = n_export_tot - n_export_me
				eallocs := s.ExportAllocs[t.Label]
				if eallocs == nil {
					eallocs = map[itemID]int{}
					s.ExportAllocs[t.Label] = eallocs
				}
				eallocs[cand.item_id] = n_export_me
			}
		}
		// Are we at the box load position?
		box_orient := s.getBoxOrient(cand.id)
		box_load_pos := box_orient.loadPos()
		fmt.Printf("box %#v %#v %v %v\n", cand, box_orient, box_load_pos, drop)
		if vec3Equal(t.CurPos, box_load_pos) {
			// Create load order job.
			pending_area_changes = true
			s.WorkIDSeq++
			lo := new(loadOrder)
			lo.ID = workID(s.WorkIDSeq)
			lo.BoxID = cand.id
			lo.Items = map[itemID]itemLoadCount{
				cand.item_id: itemLoadCount{
					PreCount: t.InvCount.Grouped[cand.item_id],
					AbsDelta: inv_x[cand.item_id],
				},
			}
			lo.Drop = drop
			s.LoadOrders[t.Label] = lo
			job := makeLoadOrderJob(*s, *lo)
			return &job
		} else {
			// Create go job.
			job := makeJobGo(workIDTmp, []vec3{box_load_pos})
			return &job
		}
	}

	// Handlers for all cases.
	tryHandleA := func() *string {
		return tryHandleAC(inv_a, true)
	}
	tryHandleB := func() *string {
		if len(inv_b) == 0 {
			return nil
		}
		// Are we at the export position?
		if vec3Equal(t.CurPos, export_q.origin) {
			// Create export drop order job.
			pending_area_changes = true
			s.WorkIDSeq++
			drop_lo := new(loadOrder)
			drop_lo.ID = workID(s.WorkIDSeq)
			drop_lo.BoxID = ExportVBoxID
			for item_id, has_count := range inv_b {
				drop_lo.Items[item_id] = itemLoadCount{
					PreCount: t.InvCount.Grouped[item_id],
					AbsDelta: has_count,
				}
			}
			drop_lo.Drop = true
			s.LoadOrders[t.Label] = drop_lo
			job := makeLoadOrderJob(*s, *drop_lo)
			return &job
		} else {
			// Create queue job. This is a temporary important job and
			// does not need to be interruptible.
			job := makeQueueOrderJob(workIDTmp, export_q)
			return &job
		}
	}
	tryHandleC := func() *string {
		return tryHandleAC(inv_c, false)
	}
	importQueue := func() string {
		// Are we at the import position?
		// Both of these jobs are temporary and unimportant. They should be
		// cancelled if required (e.g. if export is suddenly required).
		// Since they are constantly re-evaluated we must prevent multiple assignment.
		// Comparing order equality is trivial since these jobs are static and
		// only has one nature. Therefore looking at their ID is sufficient.
		if vec3Equal(t.CurPos, import_q.origin) {
			// Create generic suck order.
			if t.CurWork != nil && t.CurWork.ID == workIDInvImpSuck {
				return ""
			}
			return makeJobSuck(workIDInvImpSuck, nil, 0, import_q.face_dir)
		} else {
			// Create queue order.
			if t.CurWork != nil && t.CurWork.ID == workIDInvImpQueue {
				return ""
			}
			return makeQueueOrderJob(workIDInvImpQueue, import_q)
		}
	}

	// Generate A, B and C.
	exporting := s.ExportAllocs[t.Label]
	if len(exporting) == 0 {
		// Nothing allocated for export, use global export.
		exporting = s.Exporting
	}
	for item_id, want_count := range exporting {
		inv_c[item_id] = want_count
	}
	for item_id, has_count := range t.InvCount.Grouped {
		inv_a[item_id] = has_count
		exp_count := inv_c[item_id]
		if exp_count > 0 {
			if exp_count > has_count {
				exp_count = has_count
			}
			inv_a[item_id] -= exp_count
			inv_b[item_id] = exp_count
			inv_c[item_id] -= exp_count
		}
	}

	// Work priority is based on free slots.
	var job *string
	if t.InvCount.FreeSlots > 0 {
		for _, fn := range []func() *string{tryHandleC, tryHandleB, tryHandleA} {
			job = fn()
			if job != nil {
				break
			}
		}
		if job == nil {
			import_job := importQueue()
			job = &import_job
		}
	} else {
		for _, fn := range []func() *string{tryHandleA, tryHandleB} {
			job = fn()
			if job != nil {
				break
			}
		}
		if job == nil {
			// No free slots and nothing to drop? Expected if entire inventory is full.
			log.Printf("storage work error: turtle %v: no free slots and nothing to drop", t.Label)
		}
	}

	// Store pending area changes.
	if pending_area_changes {
		s.store()
	}

	// Return job.
	return job
}

func makeLoadOrderJob(s storageArea, lo loadOrder) string {
	if lo.Drop {
		var load_dir vec3
		if lo.BoxID == ExportVBoxID {
			export_q := s.getExportQ()
			load_dir = export_q.face_dir
		} else {
			box_orient := s.getBoxOrient(lo.BoxID)
			load_dir = box_orient.loadDir
		}
		items := map[itemID]int{}
		for item_id, lo_count := range lo.Items {
			items[item_id] = lo_count.AbsDelta
		}
		return makeJobDrop(lo.ID, items, load_dir)
	} else {
		box_orient := s.getBoxOrient(lo.BoxID)
		for item_id, lo_count := range lo.Items {
			return makeJobSuck(lo.ID, &item_id, lo_count.AbsDelta, box_orient.loadDir)
		}
		panic("expected exactly one item in load order to suck, got zero")
	}
}

func makeQueueOrderJob(id workID, q qCoords) string {
	return makeJobQueue(id, q.origin, q.q_dir, q.o_q0_dir, q.q0_t0_dir)
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
