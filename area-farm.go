package main

import (
	"fmt"
	"time"
)

type farmArea struct {
	Enabled bool
	Path    string `json:"-"`
	ID      areaID
	// sequence counter for work ids
	WorkIDSeq int `json:"work_id_seq"`
	Interval  int // how often to farm plots in minutes
	Pos       vec3
	Seeds     []itemID
	Plots     []*farmPlot
}

func (f farmArea) store() {
	storeJSON(f.Path, f)
}

type farmPlot struct {
	Level     int `json:"-"` // populated on load, index in f.Plots
	Seeds     []itemID
	PlantTime string `json:"plant_time"`
	Assignee  turtleID
	WorkID    workID // `json:"work_id"`
}

func (f farmArea) getBoxCoord(box_id int) vec3 {
	return vec3Add(f.Pos, vec3{-4 + box_id, 2, 9})
}

func (_ farmArea) getBoxLoadDir(box_id int) vec3 {
	return vec3{0, 0, 1}
}

func (f farmArea) getBoxLoadCoord(box_id int) vec3 {
	coord := f.getBoxCoord(box_id)
	dir := f.getBoxLoadDir(box_id)
	return vec3{
		coord[0] + dir[0]*-1,
		coord[1] + dir[1]*-1,
		coord[2] + dir[2]*-1,
	}
}

func (f farmArea) getDecendOrigin() vec3 {
	return vec3Add(f.Pos, vec3{-4, 2, 6})
}

func (f farmArea) getAscendOrigin() vec3 {
	return vec3Add(f.Pos, vec3{-3, 2, 6})
}

func (f farmArea) getQ() qCoords {
	return qCoords{
		face_dir:  vec3{-1, 0, 0},
		origin:    vec3Add(f.Pos, vec3{-1, 2, 6}),
		q_dir:     vec3{0, 0, -1},
		o_q0_dir:  vec3{1, 0, 0},
		q0_t0_dir: vec3{0, 0, 1},
	}
}

func (f farmArea) getElevatorJob(t turtle, dst_level int) *string {
	cur_level := -(t.CurPos[1] - f.Pos[1] - 2) / 3
	fmt.Printf("cur level: %d, dst_level: %d\n", cur_level, dst_level)
	if cur_level < 0 {
		cur_level = 0
	}
	if cur_level == dst_level {
		// Already at level.
		return nil
	}
	var elevator vec3
	if cur_level < dst_level {
		elevator = f.getAscendOrigin()
	} else {
		elevator = f.getDecendOrigin()
	}
	waypoints := []vec3{
		vec3{elevator[0], elevator[1] - cur_level*3, elevator[2]},
		vec3{elevator[0], elevator[1] - dst_level*3, elevator[2]},
	}
	job := makeJobGo(workIDTmp, waypoints)
	return &job
}

func (f farmArea) getWaitJob(t turtle) *string {
	queue := f.getQ()
	if vec3Equal(t.CurPos, queue.origin) {
		idle_job := makeJobIdle(workIDTmp, 10)
		return &idle_job
	}
	elevator_job := f.getElevatorJob(t, 0)
	if elevator_job != nil {
		return elevator_job
	}
	queue_job := makeQueueOrderJob(workIDTmp, queue)
	return &queue_job
}

func mgrDecideFarmWork(t turtle, f *farmArea) (*string, error) {
	// We do not assign work when an existing job is not completed.
	if t.CurWork != nil && !t.CurWork.ID.isLowPriority() && !t.CurWork.Complete {
		// Non interruptible work is not complete yet.
		job := ""
		return &job, nil
	}

	if !f.Enabled {
		job := f.getWaitJob(t)
		return job, nil
	}
	pending_area_changes := false

	// Find already assigned work.
	for _, plot := range f.Plots {
		if plot.Assignee == t.Label {
			if t.CurWork == nil || t.CurWork.ID != plot.WorkID {
				// Turtle completed intermediary step in farming operation
				// or a race caused work to not be assigned (e.g. chunk unload).
				job, err := makeFarmOrderJob(t, *f, *plot)
				return job, err
			}
			// Note that harvest and plantation is complete for plot.
			plot.PlantTime = time.Now().UTC().Format(time.RFC3339)
			plot.Assignee = ""
			plot.WorkID = workID(0)
			// Storing changes is pending.
			pending_area_changes = true
		}
	}

	// Handler for refueling.
	tryRefuel := func() (*string, error) {
		if t.FuelLvl > 500 {
			return nil, nil
		}
		// When we have refuel item in inventory we use them.
		item_id := itemID("minecraft:coal/0")
		fuel_per_item := 80
		fuel_to_lvl := 5000
		n_has := t.InvCount.Grouped[item_id]
		// Note: Assuming one stack of fuel.
		if n_has == 0 && t.InvCount.FreeSlots == 0 {
			return nil, nil
		}
		n_need := fuel_to_lvl / fuel_per_item
		if n_has >= n_need {
			// Refuel now.
			job := makeJobRefuel(workIDTmp, item_id, n_need)
			return &job, nil
		}
		// Go to level 0 first.
		elevator_job := f.getElevatorJob(t, 0)
		if elevator_job != nil {
			return elevator_job, nil
		}
		// Go to fuel box.
		load_dir := f.getBoxLoadDir(1)
		load_pos := f.getBoxLoadCoord(1)
		if vec3Equal(t.CurPos, load_pos) {
			// Create suck job.
			amount := n_need - n_has
			job := makeJobSuck(workIDTmp, &item_id, amount, load_dir)
			return &job, nil
		} else {
			// Create go job.
			job := makeJobGo(workIDTmp, []vec3{load_pos})
			return &job, nil
		}
	}

	// Handler for loading and unloading.
	tryReload := func() (*string, error) {
		// Calculate required seed amounts.
		seed_req_amounts := map[itemID]int{}
		plot_edge := 9
		plot_area := plot_edge*plot_edge - 1
		for _, plot := range f.Plots {
			for _, plot_seed := range plot.Seeds {
				required_amount := (plot_edge + plot_area + len(plot.Seeds) - 1) / len(plot.Seeds)
				if seed_req_amounts[plot_seed] < required_amount {
					seed_req_amounts[plot_seed] = required_amount
				}
			}
		}
		// Determine item balance.
		balance := map[itemID]int{}
		for item_id, req_amount := range seed_req_amounts {
			balance[item_id] = -req_amount
		}
		for item_id, has_amount := range t.InvCount.Grouped {
			balance[item_id] = balance[item_id] + has_amount
			if balance[item_id] == 0 {
				delete(balance, item_id)
			}
		}
		for item_id, balance := range balance {
			// Go to level 0 first.
			elevator_job := f.getElevatorJob(t, 0)
			if elevator_job != nil {
				return elevator_job, nil
			}
			// Determine box ID.
			box_id := 0
			if balance < 0 {
				for i, box_seed := range f.Seeds {
					if box_seed != item_id {
						continue
					}
					box_id = 2 + i
				}
			}
			load_dir := f.getBoxLoadDir(box_id)
			load_pos := f.getBoxLoadCoord(box_id)
			if vec3Equal(t.CurPos, load_pos) {
				if balance > 0 {
					// Create drop job.
					job := makeJobDrop(workIDTmp, map[itemID]int{item_id: balance}, load_dir)
					return &job, nil
				} else {
					// Create suck job.
					job := makeJobSuck(workIDTmp, &item_id, -balance, load_dir)
					return &job, nil
				}
			} else {
				// Create go job.
				job := makeJobGo(workIDTmp, []vec3{load_pos})
				return &job, nil
			}
		}
		// Done if everything is balanced.
		return nil, nil
	}

	// Handler for farming.
	tryFarm := func() (*string, error) {
		// Go through plots and find plot to farm.
		replant_duration := time.Minute * time.Duration(f.Interval)
		for _, plot := range f.Plots {
			// Use zero value for plant time (1970) on parse error.
			plant_time, _ := time.Parse(time.RFC3339, plot.PlantTime)
			since := time.Since(plant_time)
			if since >= replant_duration {
				pending_area_changes = true
				f.WorkIDSeq++
				plot.Assignee = t.Label
				plot.WorkID = workID(f.WorkIDSeq)
				job, err := makeFarmOrderJob(t, *f, *plot)
				return job, err
			}
		}
		return nil, nil
	}

	var job *string
	for _, fn := range []func() (*string, error){tryRefuel, tryReload, tryFarm} {
		var err error
		job, err = fn()
		if err != nil {
			return nil, err
		}
		if job != nil {
			break
		}
	}
	if job == nil {
		// Nothing to do, go wait.
		job = f.getWaitJob(t)
	}

	// Store pending area changes.
	if pending_area_changes {
		f.store()
	}

	// Return job.
	return job, nil
}

func makeFarmOrderJob(t turtle, f farmArea, plot farmPlot) (*string, error) {
	// Go to plot level.
	elevator_job := f.getElevatorJob(t, plot.Level)
	if elevator_job != nil {
		return elevator_job, nil
	}
	// Go to farming start position.
	y_offset := 2 - (3 * plot.Level)
	start_pos := vec3Add(f.Pos, vec3{-4, y_offset, 4})
	if !vec3Equal(t.CurPos, start_pos) {
		job := makeJobGo(workIDTmp, []vec3{start_pos})
		return &job, nil
	}
	// Define farming walk waypoints.
	waypoints := []vec3{
		// Top rows.
		vec3{-4, 0, 4},
		vec3{-4, 0, -4},
		vec3{-3, 0, -4},
		vec3{-3, 0, 4},
		vec3{-2, 0, 4},
		vec3{-2, 0, -4},
		vec3{-1, 0, -4},
		vec3{-1, 0, 4},
		// Middle row (corner case).
		vec3{0, 0, 4},
		vec3{0, 0, 1},
		vec3{1, 0, 1},
		vec3{1, 0, -1},
		vec3{0, 0, -1},
		vec3{0, 0, -4},
		// Bottom rows.
		vec3{1, 0, -4},
		vec3{1, 0, 4},
		vec3{2, 0, 4},
		vec3{2, 0, -4},
		vec3{3, 0, -4},
		vec3{3, 0, 4},
		vec3{4, 0, 4},
		vec3{4, 0, -4},
	}
	// Adjust waypoints level and generate absolute coordinates.
	for i, wp := range waypoints {
		wp[1] = y_offset
		waypoints[i] = vec3Add(f.Pos, wp)
	}
	job := makeJobFarm(plot.WorkID, waypoints, plot.Seeds, 1)
	return &job, nil
}
