package main

import (
	"fmt"
	"log"
	"math"
	"path"
	"time"
)

type boreholeState int

const (
	boreholeUndrilled  = boreholeState(0)
	boreholeInProgress = boreholeState(1)
	boreholeComplete   = boreholeState(2)
)

type mineArea struct {
	Path string `json:"-"`
	ID   areaID
	// sequence counter for work ids
	WorkIDSeq int `json:"work_id_seq"`
	Pos       vec3
	Depth     int
	NextClear int `json:"next_clear"`
	NextMine  int `json:"next_mine"`
	// all mines in progress have the state of their 5 borehole jobs mapped here
	MineProgress map[string][]boreholeState `json:"mine_progress"`
	MineAllocs   map[turtleID]*mineOrder    `json:"mine_allocs"`
}

func (m mineArea) store() {
	storeJSON(m.Path, m)
}

type mineOrderType string

const (
	mineOrderClear = mineOrderType("clear")
	mineOrderDrill = mineOrderType("drill")
)

type mineOrder struct {
	ID workID // work id
	// clear: Clear next mine (with id m.NextClear).
	// drill: Drill a borehole.
	Type mineOrderType
	// when drilling: borehole to drill.
	BoreholeID int `json:"borehole_id"`
	// when clearing: 0 = mine, 1 = build torches
	State int
}

func (m mineArea) getFuelBoxCoord() vec3 {
	return vec3Add(m.Pos, vec3{3, 1, 0})
}

func (m mineArea) getTorchBoxCoord() vec3 {
	return vec3Add(m.Pos, vec3{3, 1, -4})
}

func (m mineArea) getUnloadBoxCoord() vec3 {
	return vec3Add(m.Pos, vec3{5, 1, -2})
}

func (m mineArea) getTorchOffsets() []vec3 {
	return []vec3{
		vec3{2, 0, -2},
		vec3{7, 0, -2},
	}
}

func (m mineArea) getMineCoord(mine_id int) vec3 {
	// The mine d value is the edge mine is part of as counted from center.
	mine_d := int(math.Floor((math.Sqrt(float64(mine_id)) + 1.) / 2.))
	// Determine inner edge, area and edge offset.
	inner_edge := (mine_d*2 - 1)
	if inner_edge < 0 {
		inner_edge = 0
	}
	inner_area := inner_edge * inner_edge
	edge_offs := mine_id - inner_area
	// Determine the segment the mine is part of.
	seg_id := -1
	seg_offs := edge_offs
	for i, seg_len := range []int{
		mine_d + 1,
		2 * mine_d,
		2 * mine_d,
		2 * mine_d,
		2*mine_d - (mine_d + 1),
	} {
		if seg_offs < seg_len {
			seg_id = i
			break
		}
		seg_offs -= seg_len
	}
	// Calculate mine simple coordinate based on segment.
	var smine_coord vec3
	switch seg_id {
	case 0:
		smine_coord = vec3{-seg_offs, 0, -mine_d}
	case 1:
		smine_coord = vec3{-mine_d, 0, -mine_d + seg_offs + 1}
	case 2:
		smine_coord = vec3{-mine_d + seg_offs + 1, 0, mine_d}
	case 3:
		smine_coord = vec3{mine_d, 0, mine_d - seg_offs - 1}
	case 4:
		smine_coord = vec3{mine_d - seg_offs - 1, 0, -mine_d}
	}
	// Convert simple coordinate to real coordinate and return.
	xlen := 10
	zlen := 5
	return vec3Add(m.Pos, vec3{
		smine_coord[0] * xlen,
		0,
		smine_coord[2] * zlen,
	})
}

type boxLoadOrient struct {
	coord vec3
	dir   vec3
}

func getMineBoxLoadOrient(boxCoord vec3) boxLoadOrient {
	return boxLoadOrient{
		vec3Add(boxCoord, vec3{0, -1, 0}),
		vec3{0, 1, 0},
	}
}

type mineAlloc struct {
	MineID int `json:"mine_id"`
	turtleID
}

var boreholeOffsets = []vec3{
	{0, -1, 0},
	{1, -1, -2},
	{2, -1, -4},
	{3, -1, -1},
	{4, -1, -3},
	{5, -1, 0},
	{6, -1, -2},
	{7, -1, -4},
	{8, -1, -1},
	{9, -1, -3},
}

// Returns the offset of the global borehole id in a local mine.
func getBoreholeOffsInMine(borehole_id int) int {
	return borehole_id % (len(boreholeOffsets) / 2)
}

// Returns the global mine id of the global borehole id.
func getBoreholeMineID(borehole_id int) int {
	return borehole_id / (len(boreholeOffsets) / 2)
}

func (m mineArea) getBoreholeWaypoints(borehole_id int) []vec3 {
	mine_borehole_offs := getBoreholeOffsInMine(borehole_id)
	mine_id := getBoreholeMineID(borehole_id)
	mine_coord := m.getMineCoord(mine_id)
	down := vec3Add(mine_coord, boreholeOffsets[mine_borehole_offs*2])
	up := vec3Add(mine_coord, boreholeOffsets[mine_borehole_offs*2+1])
	return []vec3{
		down,
		vec3Add(down, vec3{0, -m.Depth, 0}),
		vec3Add(up, vec3{0, -m.Depth, 0}),
		up,
	}
}

func mgrDecideMineWork(t turtle, m *mineArea) *string {
	// We do not assign work when an existing job is not completed.
	if t.CurWork != nil && !t.CurWork.ID.isLowPriority() && !t.CurWork.Complete {
		// Non interruptible work is not complete yet.
		job := ""
		return &job
	}
	pending_area_changes := false

	// Have we completed a mining operation that we should account for?
	if order := m.MineAllocs[t.Label]; order != nil {
		if t.CurWork == nil || t.CurWork.ID != order.ID {
			// Turtle completed intermediary step in mining operation
			// or a race caused work to not be assigned (e.g. chunk unload).
			job := makeMineOrderJob(t, *m, *order)
			return &job
		}
		if order.State > 0 {
			// Explicit intermediary step in mining operation completed.
			// Assign new order with next state.
			m.WorkIDSeq++
			order.ID = workID(m.WorkIDSeq)
			order.State--
			m.store()
			job := makeMineOrderJob(t, *m, *order)
			return &job
		}
		switch order.Type {
		case mineOrderClear:
			// Completed clearing a mine section.
			m.NextClear++
		case mineOrderDrill:
			// Note that borehole is complete for mine.
			mine_borehole_offs := getBoreholeOffsInMine(order.BoreholeID)
			mine_id := getBoreholeMineID(order.BoreholeID)
			mine_progress := m.MineProgress[itoa(mine_id)]
			if mine_progress[mine_borehole_offs] != boreholeInProgress {
				log.Printf("mine work error: turtle %v: current bore order: %#v,"+
					" does not match mine progress: %#v", t.Label, order, m.MineProgress)
				return nil
			}
			mine_progress[mine_borehole_offs] = boreholeComplete
			mine_complete := true
			for _, state := range mine_progress {
				if state != boreholeComplete {
					mine_complete = false
					break
				}
			}
			if mine_complete {
				log.Printf("mine work: completed mine #%v", mine_id)
				delete(m.MineProgress, itoa(mine_id))
			}
			// Write borehole statistics.
			stats := map[string]interface{}{
				"time":               time.Now().Format("2006-01-02 15:04:05"),
				"borehole_id":        order.BoreholeID,
				"mine_id":            mine_id,
				"mine_borehole_offs": mine_borehole_offs,
				"turtle":             t.Label,
				"items":              t.InvCount.Grouped,
			}
			storeJSONSync(path.Dir(m.Path)+"/stats/"+itoa(order.BoreholeID), stats, false)
		}
		// Mine allocation complete, remove it.
		delete(m.MineAllocs, t.Label)
		// Storing changes is pending.
		pending_area_changes = true
	}

	// Find new job.
	// Work is selected in the following priority:
	// 1. Refuel when out of fuel.
	// 2. Unload all items.
	// 3. Clear ahead if not sufficiently ahead and no other turtle is clearing.
	//    (multi job work allocation)
	// 4. Allocate and drill an exiting borehole. When out of boreholes we
	//    increment NextMine if behind NextClear and generate new bore holes.
	// 5. Idle. (We could have a designated idle queue for this if this
	//    condition is common which is unlikely.)

	// Handler for refueling.
	tryRefuel := func() *string {
		if t.FuelLvl > 500 {
			return nil
		}
		// When we have refuel item in inventory we use them.
		item_id := itemID("minecraft:coal/0")
		fuel_per_item := 80
		fuel_to_lvl := 5000
		n_has := t.InvCount.Grouped[item_id]
		// Note: Assuming one stack of fuel.
		if n_has == 0 && t.InvCount.FreeSlots == 0 {
			return nil
		}
		n_need := fuel_to_lvl / fuel_per_item
		if n_has >= n_need {
			// Refuel now.
			job := makeJobRefuel(workIDTmp, item_id, n_need)
			return &job
		}
		// Go to fuel box.
		box_orient := getMineBoxLoadOrient(m.getFuelBoxCoord())
		if vec3Equal(t.CurPos, box_orient.coord) {
			// Create suck job.
			amount := n_need - n_has
			job := makeJobSuck(workIDTmp, &item_id, amount, box_orient.dir)
			return &job
		} else {
			// Create go job.
			job := makeJobGo(workIDTmp, []vec3{box_orient.coord})
			return &job
		}
	}

	// Handler for unloading.
	tryUnload := func() *string {
		if len(t.InvCount.Grouped) == 0 {
			return nil
		}
		// Go to unload box.
		box_orient := getMineBoxLoadOrient(m.getUnloadBoxCoord())
		if vec3Equal(t.CurPos, box_orient.coord) {
			// Create drop job.
			job := makeJobDrop(workIDTmp, t.InvCount.Grouped, box_orient.dir)
			return &job
		} else {
			// Create go job.
			job := makeJobGo(workIDTmp, []vec3{box_orient.coord})
			return &job
		}
	}

	// Handler for clearing.
	tryClear := func() *string {
		ideal_clear_ahead := int(8) // How far NextClear should be kept from NextMine.
		clear_ahead := m.NextClear - m.NextMine
		if clear_ahead >= ideal_clear_ahead {
			return nil
		}
		// Only one turtle may clear at a time.
		for _, order := range m.MineAllocs {
			if order.Type == mineOrderClear {
				return nil
			}
		}
		// Generate clear order.
		pending_area_changes = true
		m.WorkIDSeq++
		order := new(mineOrder)
		order.ID = workID(m.WorkIDSeq)
		order.Type = mineOrderClear
		order.State = 2
		m.MineAllocs[t.Label] = order
		job := makeMineOrderJob(t, *m, *order)
		return &job
	}

	// Handler for drilling.
	tryDrill := func() *string {
		create_drill_order := func(mine_id int, mine_borehole_offs int) *string {
			pending_area_changes = true
			m.WorkIDSeq++
			order := new(mineOrder)
			order.ID = workID(m.WorkIDSeq)
			order.Type = mineOrderDrill
			order.BoreholeID = mine_id*5 + mine_borehole_offs
			m.MineAllocs[t.Label] = order
			mine_progress := m.MineProgress[itoa(mine_id)]
			mine_progress[mine_borehole_offs] = boreholeInProgress
			job := makeMineOrderJob(t, *m, *order)
			return &job
		}
		// Find an undrilled borehole.
		for mine_id_str, mine_progress := range m.MineProgress {
			mine_id := atoi(mine_id_str)
			for mine_borehole_offs, borehole := range mine_progress {
				if borehole == boreholeUndrilled {
					// Generate drill order.
					return create_drill_order(mine_id, mine_borehole_offs)
				}
			}
		}
		// Try to open a new mine.
		if m.NextMine < m.NextClear {
			pending_area_changes = true
			mine_id := m.NextMine
			mine_borehole_offs := 0
			mine_progress := [5]boreholeState{}
			for i := range mine_progress {
				mine_progress[i] = boreholeUndrilled
			}
			m.MineProgress[itoa(mine_id)] = mine_progress[:]
			m.NextMine++
			return create_drill_order(mine_id, mine_borehole_offs)
		}
		return nil
	}

	var job *string
	for _, fn := range []func() *string{tryRefuel, tryUnload, tryClear, tryDrill} {
		job = fn()
		if job != nil {
			break
		}
	}
	if job == nil {
		idle_job := makeJobIdle(workIDTmp, 10)
		job = &idle_job
	}

	// Store pending area changes.
	if pending_area_changes {
		m.store()
	}

	// Return job.
	return job
}

func makeMineOrderJob(t turtle, m mineArea, order mineOrder) string {
	switch order.Type {
	case mineOrderClear:
		torch_item_id := itemID("Railcraft:lantern.stone/9")
		mine_coord := m.getMineCoord(m.NextClear)
		switch {
		case order.State == 2:
			// Ensure we carry 2 torches.
			n_has := t.InvCount.Grouped[torch_item_id]
			n_need := 2
			if n_has < n_need {
				// Go to fuel box.
				box_orient := getMineBoxLoadOrient(m.getTorchBoxCoord())
				if !vec3Equal(t.CurPos, box_orient.coord) {
					return makeJobGo(workIDTmp, []vec3{box_orient.coord})
				}
				// Suck torches.
				amount := n_need - n_has
				return makeJobSuck(workIDTmp, &torch_item_id, amount, box_orient.dir)
			}
			// Clear mine segment.
			return makeClearMineOrderJob(t, m, order, mine_coord)
		case order.State < 2:
			// Place torch.
			coord := vec3Add(mine_coord, m.getTorchOffsets()[order.State])
			place_pos := vec3Add(coord, vec3{0, 1, 0})
			place_dir := vec3{0, -1, 0}
			// Go to construct position.
			if !vec3Equal(t.CurPos, place_pos) {
				return makeJobGo(workIDTmp, []vec3{place_pos})
			}
			// Construct torch now.
			return makeJobConstruct(order.ID, torch_item_id, []vec3{place_pos}, place_dir)
		default:
			panic(fmt.Sprintf("invalid order state %v", order.State))
		}
	case mineOrderDrill:
		// Go to drill position.
		waypoints := m.getBoreholeWaypoints(order.BoreholeID)
		start_pos := vec3Add(waypoints[0], vec3{0, 1, 0})
		if !vec3Equal(t.CurPos, start_pos) {
			return makeJobGo(workIDTmp, []vec3{start_pos})
		}
		// Drill.
		dynamic := true
		clear := false
		return makeJobMine(order.ID, waypoints, []vec3{}, dynamic, clear)
	default:
		panic(fmt.Sprintf("unknown order type %v", order.Type))
	}
}

func makeClearMineOrderJob(t turtle, m mineArea, order mineOrder, mine_coord vec3) string {
	// Determine attack direction.
	var attack_dir vec3
	x_pos := mine_coord[0] >= m.Pos[0]
	z_pos := mine_coord[2] >= m.Pos[2]
	switch {
	case !x_pos && !z_pos:
		attack_dir = vec3{-1, 0, 0}
	case !x_pos && z_pos:
		attack_dir = vec3{0, 0, 1}
	case x_pos && z_pos:
		attack_dir = vec3{1, 0, 0}
	case x_pos && !z_pos:
		attack_dir = vec3{0, 0, -1}
	}
	// Determine attack position.
	attack_offs := vec3{}
	if attack_dir[0] == -1 {
		attack_offs[0] = 10
	} else {
		attack_offs[0] = -attack_dir[0]
	}
	if attack_dir[2] == 1 {
		attack_offs[2] = -5
	} else {
		attack_offs[2] = -attack_dir[2]
	}
	attack_pos := vec3Add(mine_coord, attack_offs)
	// First go to attack position.
	// This is an intermediary step (workIDTmp).
	if !vec3Equal(t.CurPos, attack_pos) {
		// Create go job.
		return makeJobGo(workIDTmp, []vec3{attack_pos})
	}
	// We are now next to initial position.
	// Determine mine waypoints.
	init_pos := vec3Add(attack_pos, attack_dir)
	waypoints := []vec3{init_pos}
	cur_pos := init_pos
	z_dir := init_pos[2] < mine_coord[2]
	for z := 0; z < 5; z++ {
		if z > 0 {
			if z_dir {
				// Go z+ to next z lane.
				cur_pos[2]++
			} else {
				// Go z- to next z lane.
				cur_pos[2]--
			}
			waypoints = append(waypoints, cur_pos)
		}
		if cur_pos[0] > mine_coord[0] {
			// Go x- in z lane.
			cur_pos[0] = mine_coord[0]
		} else {
			// Go x+ in z lane.
			cur_pos[0] = mine_coord[0] + 9
		}
		waypoints = append(waypoints, cur_pos)
	}
	extra_dirs := []vec3{vec3{0, 1, 0}}
	dynamic := false
	clear := true
	return makeJobMine(order.ID, waypoints, extra_dirs, dynamic, clear)
}
