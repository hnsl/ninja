package main

import(
    "fmt"
    "strings"
    "strconv"
)

func luaSerialVec3(v vec3) string {
    return fmt.Sprintf("{%d, %d, %d}", v[0], v[1], v[2])
}

var tplJobGo = `new_job = {
    id = %d,
    type = "go",
    instructions = {
        waypoint_stack = {%s},
    },
},
`

func makeJobGo(id int, waypoints []vec3) string {
    wp_parts := make([]string, len(waypoints))
    for i, wp := range(waypoints) {
        wp_parts[len(waypoints) - i - 1] = luaSerialVec3(wp) + ","
    }
    wp_srl := strings.Join(wp_parts, "")
    return fmt.Sprintf(tplJobGo, id, wp_srl)
}

var tplJobSuck = `new_job = {
    id = %d,
    type = "suck",
    instructions = {
        item = %s,
        amount = %d,
        dir = %s,
    },
},
`

func makeJobSuck(id int, iname *itemName, amount int, dir vec3) string {
    item_srl := "nil"
    if iname != nil {
        item_srl = strconv.Quote(string(*iname))
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

func makeJobDrop(id int, items map[itemName]int, dir vec3) string {
    items_parts := make([]string, len(items))
    i := 0
    for iname, count := range(items) {
        items_parts[i] = fmt.Sprintf("[%s] = %d,", strconv.Quote(string(iname)), count)
        i++
    }
    items_srl := strings.Join(items_parts, "")
    dir_srl := luaSerialVec3(dir)
    return fmt.Sprintf(tplJobDrop, id, items_srl, dir_srl)
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

func makeJobQueue(id int, origin, q_dir, o_q0_dir, q0_t0_dir vec3) string {
    return fmt.Sprintf(tplJobQueue, id,
        luaSerialVec3(origin),
        luaSerialVec3(q_dir),
        luaSerialVec3(o_q0_dir),
        luaSerialVec3(q0_t0_dir),
    )
}
