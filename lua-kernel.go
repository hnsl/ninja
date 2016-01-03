package main; var lua_src_kernel = `
version = 30

local base_url = "http://skogen.twitverse.com:4456/72ceda8b"
local state_root = "/state"

local orient_block_name = "ExtraUtilities:color_stonebrick"

local refuel_item_name = "minecraft:coal"
local refuel_item_fpi = 80

-- Look() return values.
local look_up = 1
local look_down = -1
local look_fwd = 0

-- Debugging.
function fmt(x)
    if x == nil then
        return "null"
    elseif type(x) == "string" then
        return x
    end
    if is_server then
        return JSON:encode(x)
    else
    return textutils.serializeJSON(x)
    end
end

function debug(x)
    print(fmt(x))
end

-- String splitting.
function split(str, pat)
   local t = {}  -- NOTE: use {n = 0} in Lua-5.0
   local fpat = "(.-)" .. pat
   local last_end = 1
   local s, e, cap = str:find(fpat, 1)
   while s do
      if s ~= 1 or cap ~= "" then
         table.insert(t, cap)
      end
      last_end = e+1
      s, e, cap = str:find(fpat, last_end)
   end
   if last_end <= #str then
      cap = str:sub(last_end)
      table.insert(t, cap)
   end
   return t
end

-- Vector functions.

function vec_equal(a, b)
    if #a ~= #b then
        return false
    end
    for i, v in ipairs(a) do
        if b[i] ~= v then
            return false
        end
    end
    return true
end

function vec_add(a, b)
    local out = {}
    for i, v in ipairs(a) do
        out[i] = v + b[i]
    end
    return out
end

function vec_sub(a, b)
    local out = {}
    for i, v in ipairs(a) do
        out[i] = v - b[i]
    end
    return out
end

-- Calculates L1 (manhattan) distance between two vectors.
function vec_l1dist(a, b)
    local dist = 0
    for i, v in ipairs(a) do
        dist = dist + math.abs(v - b[i])
    end
    return dist
end

-- Returns one of the dimensions this vector has a non-zero component in.
function vec_dim(a)
    for i, v in ipairs(a) do
        if v ~= 0 then
            return i
        end
    end
    return nil
end


-- Takes neighbour 2d vector and returns neighbour id (1-8).
function vec2_neighbour_v2id(v)
    if v[1] == 0 then
        if v[2] == 1 then
            return 3
        else
            return 7
        end
    elseif v[1] == 1 then
        if v[2] == 1 then
            return 2
        elseif v[2] == 0 then
            return 1
        else
            return 8
        end
    else
        if v[2] == 1 then
            return 4
        elseif v[2] == 0 then
            return 5
        else
            return 6
        end
    end
end

-- Takes neighbour id (1-8) and returns neighbour 2d vector.
function vec2_neighbour_id2v(nid)
    local d1, d2
    local p1 = (nid % 8) + 1
    if p1 == 4 or p1 == 8 then
        d1 = 0
    elseif p1 < 4 then
        d1 = 1
    else
        d1 = -1
    end
    if nid == 1 or nid == 5 then
        d2 = 0
    elseif nid < 5 then
        d2 = 1
    else
        d2 = -1
    end
    return {d1, d2}
end

-- 90 degrees right or left rotation around y axis.
function vec3_rot90_y(a, left)
    -- cos(a) is always 0 for 90 degree rotations and can be ignored
    local sin_a
    if left then
        -- a = 1/2 pi
        sin_a = 1
    else
        -- a = -1/2 pi
        sin_a = -1
    end
    return {
        sin_a * a[3],  -- new_x = cos(a) * x + 0 * y + sin(a) * z = sin(a) * z
        a[2],          -- new_y = 0 * x + 1 * y + 0 * z = y
        -sin_a * a[1], -- new_z = -sin(a) * x + 0 * y + cos(a) * z = -sin(a) * x
    }
end

-- 180 degree rotation around y axis.
function vec3_rot180_y(a)
    -- cos(a) is -1 for 180 degree rotations
    local cos_a = -1
    -- sin(a) is 0 for 180 degree rotations and can be ignored
    return {
        -1 * a[1], -- new_x = cos(a) * x + 0 * y + sin(a) * z = -x
        a[2],      -- new_y = 0 * x + 1 * y + 0 * z = y
        -1 * a[3], -- new_z = -sin(a) * x + 0 * y + cos(a) * z = -z
    }
end

-- Returns the dimensions of a plane id.
function plane_dims(plane_id)
    if plane_id == 1 then
        -- x/y
        return {1, 2}
    elseif plane_id == 2 then
        -- x/z
        return {1, 3}
    elseif plane_id == 3 then
        -- y/z
        return {2, 3}
    end
    return nil
end

-- File I/O
function fs_state_put(name, data)
    -- Make sure state root exists.
    fs.makeDir(state_root)
    -- Replace data in a manner that is as atomic as possible.
    local path = state_root .. "/" .. name
    local tmp_path = path .. ".tmp"
    local h = fs.open(tmp_path, "w")
    h.write(textutils.serialize(data))
    h.close()
    fs.delete(path)
    fs.move(tmp_path, path)
end

function fs_state_get(name, default)
    local path = state_root .. "/" .. name
    if not fs.exists(path) then
        fs_state_put(name, default)
        return default
    end
    local h = fs.open(path, "r")
    local data = h.readAll()
    h.close()
    return textutils.unserialize(data)
end

-- Turtle robot logic.

function orientate_component()
    local comp = 0
    for i = 0, 3, 1 do
        local success, bdata
        if (i % 2) == 0 then
            if not turtle.detectUp() then
                return nil, "missing up block"
            end
            success, bdata = turtle.inspectUp()
        else
            if not turtle.detectDown() then
                return nil, "missing down block"
            end
            success, bdata = turtle.inspectDown()
        end
        if not success then
            return nil, ("inspect failed: " .. fmt(bdata))
        end
        if bdata.name ~= orient_block_name then
            return nil, ("invalid block: " .. fmt(bdata.name))
        end
        local v4b = bdata.metadata
        comp = comp + v4b * (2 ^ (4 * i))
        if (i % 2) ~= 0 then
            if not turtle.forward() then
                return nil, "failed moving forward"
            end
        end
    end
    return comp, nil
end

function orientate()
    local coord = {}
    for i = 1, 3, 1 do
        debug("orienting component [" .. i .. "]")
        local comp, err = orientate_component()
        if err then
            return nil, err
        end
        debug("component [" .. i .. "]: [" .. comp .. "]")
        coord[i] = comp
    end
    return coord, nil
end

function report()
    -- Try reporting until reporting is successful.
    while true do
        if try_report() then
            break
        end
        sleep(2)
    end
end

function tryRefuel(required)
    local first_slot = turtle.getSelectedSlot()
    local cur_slot = first_slot
    while true do
        -- Check if found fuel item in this slot.
        local detail = turtle.getItemDetail(cur_slot)
        if detail ~= nil and detail.name == refuel_item_name then
            -- Fuel found, select slot.
            if cur_slot ~= first_slot then
                local success = turtle.select(cur_slot)
                if not success then
                    return "selecting slot " .. fmt(cur_slot) .. " failed"
                end
            end
            -- Refuel the amount we want.
            local max = math.ceil(required / refuel_item_fpi)
            debug("refueling " .. fmt(max) .. "@" .. fmt(cur_slot))
            local success = turtle.refuel(max)
            if not success then
                return "refuel() failed"
            end
            -- Refueling ok.
            return nil
        end
        cur_slot = (cur_slot % 16) + 1
        if cur_slot == first_slot then
            return "no fuel found in inventory"
        end
    end
end

function upgradeKernel()
    debug("checking for kernel upgrade")
    local h = http.get(base_url .. "/version", data)
    local rcode = (h ~= nil and h.getResponseCode()) or 0
    if rcode ~= 200 then
        debug("upgrade: failed+skipped, bad status code [" .. fmt(rcode) .. "]")
        h.close()
        return false
    end
    local new_version = tonumber(h.readAll())
    h.close()
    debug("upgrade: current version [" .. fmt(new_version) .. "], new version: [" .. fmt(new_version) .. "]")
    if new_version <= version then
        return
    end
    debug("upgrade: downloading new version")
    local h = http.get(base_url .. "/kernel", data)
    local rcode = h.getResponseCode()
    if rcode ~= 200 then
        debug("upgrade: failed+skipped, bad status code [" .. fmt(rcode) .. "]")
        h.close()
        return false
    end
    local new_kernel = h.readAll()
    h.close()
    debug("upgrade: flashing new version")
    local path = "/startup"
    local tmp_path = path .. ".tmp"
    local h = fs.open(tmp_path, "w")
    h.write(new_kernel)
    h.close()
    fs.delete(path)
    fs.move(tmp_path, path)
    debug("upgrade: booting new version now")
    os.sleep(1)
    os.reboot()
end

-- Returns inventory count.
function inventoryCount()
    local inv_count = {}
    for i = 1, 16 do
        local detail = turtle.getItemDetail(i)
        if detail ~= nil then
            local count = inv_count[detail.name] or 0
            inv_count[detail.name] = count + detail.count
        end
    end
    return inv_count
end

(function()
    if is_server then
        debug("kernel: server run complete")
        return
    end

    -- Upgrade kernel automatically if required.
    upgradeKernel()

    -- Seed random function.
    debug("seeding random function")
    math.randomseed(os.time())

    -- Initialize state.
    debug("initializing turtle state")
    local new_kernel = nil
    local cur_action = nil
    local cur_dst = nil
    local cur_best_dist = nil -- best distance so far, reset on new move
    local cur_pivot = nil -- pivot configuration, reset on completed pivot or new move
    local cur_frustration = 0 -- frustration, reset on new move, determines pivot aggressiveness
    local last_mdir = {1, 0, 0} -- last move direction, required as looking up or down is not possible
    local cur_pos = fs_state_get("cur_pos", nil)
    local cur_rot = fs_state_get("cur_rot", nil)
    local work_q = fs_state_get("work_q", {})
    local fatal_err = nil
    local refuel_err = nil

    local refuel_min = 100
    local refuel_max = 32 * 80

    function fatalError(err)
        local full_err = "fatal error: " .. fmt(err)
        debug(full_err)
        fatal_err = full_err
    end

    -- Moving and rotating.
    function turn(left)
        local turn_ok
        if left then
            turn_ok = turtle.turnLeft()
        else
            turn_ok = turtle.turnRight()
        end
        if not turn_ok then
            debug("turn: unexpected turn failure")
            return false
        end
        -- Update current rotation.
        cur_rot = vec3_rot90_y(cur_rot, left)
        fs_state_put("cur_rot", cur_rot)
        -- Turning is racy so we need to sleep a little to slow rate.
        os.sleep(0.5)
        return true
    end

    function look(dir)
        -- Handle trivial y movement first.
        if vec_equal(dir, {0, 1, 0}) then
            return true, look_up
        elseif vec_equal(dir, {0, -1, 0}) then
            return true, look_down
        end
        -- Trivial forward case.
        if vec_equal(cur_rot, dir) then
            return true, look_fwd
        end
        -- Turn cases.
        if vec_equal(vec3_rot180_y(cur_rot), dir) then
            -- Backward case.
            for i = 1, 2 do
                local turn_ok = turn(false)
                if not turn_ok then
                    return false, nil
                end
            end
        elseif vec_equal(vec3_rot90_y(cur_rot, true), dir) then
            -- Left case.
            local turn_ok = turn(true)
            if not turn_ok then
                return false, nil
            end
        elseif vec_equal(vec3_rot90_y(cur_rot, false), dir) then
            -- Right case.
            local turn_ok = turn(false)
            if not turn_ok then
                return false, nil
            end
        else
            debug("look: error: got non-orthogonal unit vector: " .. fmt(dir))
            return false, nil
        end
        return true, look_fwd
    end

    function move(dir)
        -- We expect dir to be an orthogonal unit vector.
        local can_move = true
        local move_ok = false
        debug("move: dir " .. fmt(dir))
        -- Handle trivial y movement first.
        if vec_equal(dir, {0, 1, 0}) then
            can_move = not turtle.detectUp()
            if can_move then
                move_ok = turtle.up()
            end
        elseif vec_equal(dir, {0, -1, 0}) then
            can_move = not turtle.detectDown()
            if can_move then
                move_ok = turtle.down()
            end
        else
            -- Handle x/z movement.
            if not vec_equal(cur_rot, dir) then
                -- Need to turn.
                local ok, lret = look(dir)
                if not ok or lret ~= look_fwd then
                    debug("move error: turning failed")
                    return false
                end
            end
            can_move = not turtle.detect()
            if can_move then
                move_ok = turtle.forward()
            end
        end
        if move_ok then
            -- Move successful, update position.
            last_mdir = dir
            cur_pos = vec_add(cur_pos, dir)
            fs_state_put("cur_pos", cur_pos)
            -- Moving is racy so we need to sleep a little to slow rate.
            os.sleep(0.5)
            return true
        else
            if can_move then
                debug("move error: unexpected move failure")
            end
            return false
        end
    end

    -- Tries to move one step closer to destination.
    -- Returns true if reached destination within close range.
    function step(dst, close)
        -- Calculate distance and see if arrived ok.
        local cur_dist = vec_l1dist(cur_pos, dst)
        debug("moving one step: " .. fmt(cur_pos) .. ", " .. fmt(dst) .. ", dist: " .. fmt(cur_dist))
        if cur_dist <= close then
            return true
        end
        if cur_best_dist == nil or cur_dist < cur_best_dist then
            -- Best distance so far, reset pivot and frustration.
            cur_best_dist = cur_dist
            cur_pivot = nil
            cur_frustration = 0
        end
        if turtle.getFuelLevel() <= 0 then
            debug("step will fail: out of fuel!")
            os.sleep(5)
            return false
        end
        if cur_pivot == nil then
            -- Try first moving in current move direction if it brings us closer.
            local fwd_dist = vec_l1dist(vec_add(cur_pos, last_mdir), dst)
            if fwd_dist < cur_dist then
                local move_ok = move(last_mdir)
                if move_ok then
                    return false
                end
            end
            -- We need to turn, pick another direction.
            local blocked_dirs = {}
            for i = 1, 3 do
                -- Start with y dimension because it require no rotation.
                local dim = (i % 3) + 1
                local ddiff = dst[dim] - cur_pos[dim]
                if ddiff ~= 0 then
                    local try_dir = {0, 0, 0}
                    -- Trying dimension that gives us lower distance.
                    try_dir[dim] = (ddiff > 0 and 1) or -1
                    local move_ok = move(try_dir)
                    if move_ok then
                        -- Move ok, reset pivot.
                        cur_pivot = nil
                        return false
                    end
                    table.insert(blocked_dirs, try_dir)
                end
            end
            -- We're stuck. Pick a random blocked dir.
            local blocked_dir = blocked_dirs[math.random(1, #blocked_dirs)]
            debug("move: stuck - pivoting")
            -- Dim0 is the blocked dimension, pick another random dimension.
            -- Together these two dimensions form a plane we are rotating in.
            local dim1 = vec_dim(blocked_dir)
            local dim2 = ((dim1 + math.random(3, 4)) % 3) + 1
            local rdir = math.random(0, 1) == 1
            cur_pivot = {
                dim1 = dim1,
                dim2 = dim2,
                -- The blocked coordinate we are pivoting around.
                bcoord = vec_add(cur_pos, blocked_dir),
                -- Pick a random rotation direction.
                rdir = rdir,
                -- Energy is based on frustration.
                energy = 4 + 2 ^ cur_frustration,
            }
            debug("move: pivot parameters: " .. fmt(cur_pivot))
            -- We are now slightly more frustrated.
            cur_frustration = math.min(cur_frustration + 1, 6)
            debug("move: increasing frustration to " .. fmt(cur_frustration))
        end
        -- We're stuck. Pivoting.
        -- Calculate our neighbour 2d vector to blocked coordinate in the chosen plane.
        -- Not beeing consistent with dimensional ordering here is not important
        -- as the rotation direction is random. It only needs to be deterministic
        -- after cur_pivot has been defined to ensure we are always rotating in
        -- the same direction during a single pivotation session.
        local neigh_vec2 = {
            cur_pos[cur_pivot.dim1] - cur_pivot.bcoord[cur_pivot.dim1],
            cur_pos[cur_pivot.dim2] - cur_pivot.bcoord[cur_pivot.dim2],
        }
        local neigh_id = vec2_neighbour_v2id(neigh_vec2)
        local neigh_id_next
        if cur_pivot.rdir then
            neigh_id_next = (neigh_id % 8) + 1
        else
            neigh_id_next = ((neigh_id + 8) % 8) + 1
        end
        local neigh_vec2_next = vec2_neighbour_id2v(neigh_id_next)
        local neigh_vec3 = {cur_pivot.bcoord[1], cur_pivot.bcoord[2], cur_pivot.bcoord[3]}
        neigh_vec3[cur_pivot.dim1] = neigh_vec3[cur_pivot.dim1] + neigh_vec2_next[1]
        neigh_vec3[cur_pivot.dim2] = neigh_vec3[cur_pivot.dim2] + neigh_vec2_next[2]
        local pivot_dir = vec_sub(neigh_vec3, cur_pos)
        debug("move: pivot dir: " .. fmt(pivot_dir) .. " (" .. fmt(neigh_vec3) .. ", " .. fmt(neigh_vec2_next) .. ")")
        local move_ok = move(pivot_dir)
        if not move_ok then
            -- We where blocked, update blocked coordinate to pivot around it.
            cur_pivot.bcoord = neigh_vec3
            debug("move: pivoting around new coordinate " .. fmt(neigh_vec3))
        end
        -- Reduce energy.
        cur_pivot.energy = cur_pivot.energy - 1
        if cur_pivot.energy == 0 then
            -- Tired. Stop pivoting.
            -- We don't want to try too hard in one direction since some
            -- chosen planes can be much better.
            cur_pivot = nil
        end
        return false
    end

    function moveTo(dst, close)
        -- Naive traveller that assumes that we can't get permanently stuck
        -- walking to a certain coord via an arbitrary manhattan path.
        -- To make things simpler we never assume that moving can permanently
        -- fail in a way that forces us to return an error code. The only way
        -- to resolve that problem is manual anyway. There is no sensible thing
        -- an error handler could possibly do.
        forgetMove = (function()
            cur_best_dist = nil
            cur_pivot = nil
            cur_frustration = 0
        end)
        if cur_dst ~= dst then
            cur_dst = dst
            forgetMove()
        end
        while true do
            -- Refuel periodically if required.
            if not manageFuel() then
                debug("move: warning - refuel failed")
            end
            -- Try move one step now.
            if step(dst, close) then
                -- Moving sufficiently close to desination was successful.
                cur_dst = nil
                forgetMove()
                return
            end
        end
    end

    function manageFuel()
        if refuel_err ~= nil then
            if os.clock() - refuel_err < 30 then
                return false
            end
        end
        local refuel_lvl = turtle.getFuelLevel()
        if refuel_lvl > refuel_min then
            return true
        end
        debug("refueling required (" .. fmt(refuel_lvl) .. "/" .. fmt(refuel_min) .. ")")
        while true do
            local required = refuel_max - refuel_lvl
            local err = tryRefuel(required)
            if err then
                debug("refueling failed: " .. fmt(err))
                refuel_err = os.clock()
                return false
            end
            local refuel_lvl = turtle.getFuelLevel()
            if refuel_lvl >= refuel_max then
                debug("refueling complete")
                refuel_err = nil
                return true
            end
        end
    end

    -- Fuel management.
    local actionFuelMng = (function()
        manageFuel()
        -- Ignore fuel management failure. It is still possible that we can
        -- continue, for example if we are next to fuel deposit.
        return false
    end)

    local actionMngOrient = (function()
        if cur_pos ~= nil then
            return false
        end
        debug("orientation required")
        local coord, err = orientate()
        if err ~= nil then
            fatalError("orientation failed, error: " .. fmt(err))
            return true
        end
        -- Set oriented position.
        cur_pos = coord
        fs_state_put("cur_pos", cur_pos)
        -- Rotation after orientation is complete is +z.
        cur_rot = {0, 0, 1}
        fs_state_put("cur_rot", cur_rot)
        debug("orientation complete: " .. fmt(cur_pos) .. " " .. fmt(cur_rot))
        return false
    end)

    local actionJobExec = (function()
        --debug("todo: exec job")
        debug("moving")
        moveTo({78, 65, 808}, 0)
        debug("move complete")
        return true
    end)

    -- Reporting.
    function try_report()
        debug("reporting: sending")
        local data = textutils.serializeJSON({
            version = version,
            label = os.getComputerLabel(),
            cur_action = cur_action,
            cur_dst = cur_dst,
            cur_best_dist = cur_best_dist,
            cur_pivot = cur_pivot,
            cur_frustration = cur_frustration,
            cur_pos = cur_pos,
            cur_rot = cur_rot,
            work_q = work_q,
            fatal_err = fatal_err,
            refuel_err = refuel_err,
            fuel_lvl = turtle.getFuelLevel(),
            inv_count = inventoryCount(),
        })
        local h = http.post(base_url .. "/report", data)
        local rcode = h.getResponseCode()
        if rcode ~= 200 then
            debug("reporting: failed, bad status code [" .. tostring(rcode) .. "]")
            h.close()
            return false
        end
        local raw_rsp = h.readAll()
        h.close()
        -- Unserialize response table.
        local rsp = textutils.unserialize(raw_rsp)
        if type(rsp) == "table" then
            debug("reporting: failed, non-table response")
            return false
        end
        debug("reporting: submitted ok")
        return true
    end
    local brainTick = (function()
        -- Actions in order of priority
        actionSequence = {
            -- Priority 1: Ensure we have sufficient fuel to operate.
            {fn = actionFuelMng, name = "fuel"},
            -- Priority 2: Ensure we have completed orientation.
            {fn = actionMngOrient, name = "orientation"},
            -- Priority 3: Execute jobs.
            {fn = actionJobExec, name = "job"},
        }
        for i,action in ipairs(actionSequence) do
            if fatal_error then
                return false
            end
            cur_action = action.name
            if action.fn() then
                break
            end
        end
        return true
    end)
    parallel.waitForAny((function()
        -- Report every 30 seconds.
        while true do
            report()
            sleep(30)
        end
    end), (function()
        -- Brain tick with a one second rate limit in case of hysterical panic.
        while true do
            if not brainTick() then
                debug("fatal error in brain: enabling permanent apathy")
                return
            end
            sleep(1)
        end
    end))
end)()
`
