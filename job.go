package main

import(
    "fmt"
    "strings"
    "strconv"
)

func luaSerialVec3(v vec3) string {
    return fmt.Sprintf("{%d, %d, %d}", v[0], v[1], v[2])
}

func luaSerialVec3Arr(vecs []vec3, invert bool) string {
    parts := make([]string, len(vecs))
    for i, vec := range(vecs) {
        dst := i
        if invert {
            dst = len(vecs) - i - 1
        }
        parts[dst] = luaSerialVec3(vec) + ","
    }
    return strings.Join(parts, "")
}

var tplJobIdle = `new_job = {
    id = %d,
    type = "idle",
    instructions = {
        time = %d,
    },
},
`

func makeJobIdle(id workID, seconds int) string {
    return fmt.Sprintf(tplJobIdle, id, seconds)
}

var tplJobGo = `new_job = {
    id = %d,
    type = "go",
    instructions = {
        waypoint_stack = {%s},
    },
},
`

func makeJobGo(id workID, waypoints []vec3) string {
    wp_srl := luaSerialVec3Arr(waypoints, true)
    return fmt.Sprintf(tplJobGo, id, wp_srl)
}

var tplJobSuck = `new_job = {
    id = %d,
    type = "suck",
    instructions = {
        item_id = %s,
        amount = %d,
        dir = %s,
    },
},
`

func makeJobSuck(id workID, item_id *itemID, amount int, dir vec3) string {
    item_srl := "nil"
    if item_id != nil {
        item_srl = strconv.Quote(string(*item_id))
    }
    dir_srl := luaSerialVec3(dir)
    return fmt.Sprintf(tplJobSuck, id, item_srl, amount, dir_srl)
}

var tplJobDrop = `new_job = {
    id = %d,
    type = "drop",
    instructions = {
        items = {%s},
        dir = %s,
    },
},
`

func makeJobDrop(id workID, items map[itemID]int, dir vec3) string {
    items_parts := make([]string, len(items))
    i := 0
    for item_id, count := range(items) {
        items_parts[i] = fmt.Sprintf("[%s] = %d,", strconv.Quote(string(item_id)), count)
        i++
    }
    items_srl := strings.Join(items_parts, "")
    dir_srl := luaSerialVec3(dir)
    return fmt.Sprintf(tplJobDrop, id, items_srl, dir_srl)
}

var tplJobRefuel = `new_job = {
    id = %d,
    type = "refuel",
    instructions = {
        item = %s,
        count = %d
    },
},`

func makeJobRefuel(id workID, item_id itemID, count int) string {
    item_srl := strconv.Quote(string(item_id))
    return fmt.Sprintf(tplJobRefuel, id, item_srl, count)
}

var tplJobQueue = `new_job = {
    id = %d,
    type = "queue",
    instructions = {
        origin = %s,
        q_dir = %s,
        o_q0_dir = %s,
        q0_t0_dir = %s,
    },
},
`

func makeJobQueue(id workID, origin, q_dir, o_q0_dir, q0_t0_dir vec3) string {
    return fmt.Sprintf(tplJobQueue, id,
        luaSerialVec3(origin),
        luaSerialVec3(q_dir),
        luaSerialVec3(o_q0_dir),
        luaSerialVec3(q0_t0_dir),
    )
}

var tplJobMine = `new_job = {
    id = %d,
    type = "mine",
    instructions = {
        waypoint_stack = {%s},
        extra_dirs = {%s},
        dynamic = %t,
        clear = %t,
    },
},`

// Creates a mine job.
// A static mine job means just drill forward to the next waypoint.
// Extra dirs can be added to mine in other directions than forward
// after each step taken.
// A dynamic mine job means to also look around for intresting blocks that will
// be selectively mined.
func makeJobMine(id workID, waypoints []vec3, extra_dirs []vec3, dynamic bool, clear bool) string {
    wp_srl := luaSerialVec3Arr(waypoints, true)
    extra_dirs_srl := luaSerialVec3Arr(extra_dirs, false)
    return fmt.Sprintf(tplJobMine, id, wp_srl, extra_dirs_srl, dynamic, clear)
}

var tplJobConstruct = `new_job = {
    id = %d,
    type = "construct",
    instructions = {
        item = %s,
        waypoint_stack = {%s},
        dir = %s,
    },
},`

// Creates a construct job.
// Before starting and after each step taken the specified item will be placed
// in the specified direction while walking towards dir.
func makeJobConstruct(id workID, item_id itemID, waypoints []vec3, dir vec3) string {
    item_srl := strconv.Quote(string(item_id))
    wp_srl := luaSerialVec3Arr(waypoints, true)
    dir_srl := luaSerialVec3(dir)
    return fmt.Sprintf(tplJobConstruct, id, item_srl, wp_srl, dir_srl)
}
