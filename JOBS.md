# Example jobs

    {
        id = 22,
        type = "go",
        complete = false,
        instructions = {
            waypoint_stack = {
                {50, 120, 64},
            },
        },
    }

    {
        id = 23,
        type = "suck",
        complete = false,
        instructions = {
            type: "minecraft:torch", -- can be nil (suck all)
            amount: 1, -- ignored if type is nil
            dir: {0, -1, 0},
        },
    }

    {
        id = 24,
        type = "drop",
        complete = false,
        instructions = {
            to_drop = {
                [minecraft:coal] = 42,
            },
            dir: {1, 0, 0},
        },
    }

    {
        id = 25,
        type = "queue",
        complete = false,
        instructions = {
            origin: {41, 100, 351},
            q_dir: {0, 0, -1},
            o_q0_dir: {-1, 0, 0},
            q0_t0_dir: {-1, 0, 0},
        },
    }

    {
        id = 26,
        type = "mine",
        complete = false,
        instructions = {
            waypoint_stack = {
                {46, 64, 116},
                {46, 10, 116},
                {50, 10, 120},
            },
        },
    }

    {
        id = 27,
        type = "construct",
        complete = false,
        instructions = {
            material = "minecraft:torch",
            start = {50, 120, 65},
            place_dir = {0, -1, 0}, -- per step, place in this direction
            waypoints = {
                {50, 120, 65},
            },
        },
    }
