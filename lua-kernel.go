package main; var lua_src_kernel = `
version = 79

local base_url = "http://skogen.twitverse.com:4456/72ceda8b"
local state_root = "/state"

local orient_block_name = "ExtraUtilities:color_stonebrick"

local refuel_item_id = "minecraft:coal/0"
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

-- inverts a direction (orhogonal unit vector)
function vec3_inv_dir(a)
    local out = {0, 0, 0}
    local d = vec_dim(a)
    out[d] = -a[d]
    return out
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

local commonNames = {
    ["minecraft:stone"] = true,
    ["minecraft:cobblestone"] = true,
    ["minecraft:dirt"] = true,
    ["minecraft:gravel"] = true,
    ["chisel:marble"] = true,
    ["chisel:diorite"] = true,
    ["chisel:andesite"] = true,
    ["chisel:granite"] = true,
    ["minecraft:torch/0"] = true,
    ["ComputerCraft:CC-Turtle"] = true,
    ["ComputerCraft:CC-TurtleExpanded"] = true,
    ["ComputerCraft:CC-TurtleAdvanced"] = true,
}

local commonItems = {}

-- Returns true if details from inspect is a common block and not interesting.
function commonBlock(details)
    if commonNames[details.name] or commonItems[details.id] then
        return true
    end
    return false
end


local fallingNames = {
    ["minecraft:dirt"] = true,
    ["minecraft:sand"] = true,
}

-- Returns true if details from inspect is a falling block.
function fallingBlock(details)
    if commonNames[details.name] then
        return true
    end
    return false
end

-- Turtle robot logic.

function getItemDetail(slotNum)
    local out = turtle.getItemDetail(slotNum)
    if out == nil then
        return nil
    end
    -- Damage = metadata for blocks. Both damage and metadata must be
    -- equiviallent for items, otherwise items are considered different
    -- and cannot ever be stacked.
    out.id = out.name .. "/" .. tostring(out.damage)
    return out
end

function attack(dir)
    local ok, lret = look(dir)
    if not ok then
        return false
    end
    if lret == look_fwd then
        return turtle.attack()
    elseif lret == look_up then
        return turtle.attackUp()
    elseif lret == look_down then
        return turtle.attackDown()
    end
end

function dig(dir)
    local ok, lret = look(dir)
    if not ok then
        return false
    end
    if lret == look_fwd then
        return turtle.dig()
    elseif lret == look_up then
        return turtle.digUp()
    elseif lret == look_down then
        return turtle.digDown()
    end
end

function place(dir)
    local ok, lret = look(dir)
    if not ok then
        return false
    end
    if lret == look_fwd then
        return turtle.place()
    elseif lret == look_up then
        return turtle.placeUp()
    elseif lret == look_down then
        return turtle.placeDown()
    end
end

function detect(dir)
    local ok, lret = look(dir)
    if not ok then
        return false, nil
    end
    if lret == look_fwd then
        return true, turtle.detect()
    elseif lret == look_up then
        return true, turtle.detectUp()
    elseif lret == look_down then
        return true, turtle.detectDown()
    end
end

function inspect(dir)
    local ok, lret = look(dir)
    if not ok then
        return false, nil
    end
    local success, data
    if lret == look_fwd then
        success, data = turtle.inspect()
    elseif lret == look_up then
        success, data = turtle.inspectUp()
    elseif lret == look_down then
        success, data = turtle.inspectDown()
    end
    if success then
        -- Generate virtual id, see getItemDetail().
        data.id = data.name .. "/" .. tostring(data.metadata)
    end
    return success, data
end

function mine(dir, clear)
    while true do
        local detect_ok, block = detect(dir)
        if not detect_ok then
            return false, ("detect " .. fmt(dir) .. " failed")
        end
        if not block then
            return true, nil
        end
        local dig_ok = dig(dir)
        if not dig_ok then
            return false, ("dig " .. fmt(dir) .. " failed")
        end
        if not clear or vec_equal(dir, {0, -1, 0}) then
            return true, nil
        end
        -- Wait 1 second for any block to fall down.
        os.sleep(1)
    end
end

-- Step towards waypoint with no collision handling.
-- Useful when movement must be extremely deterministic and can be assumed
-- to be collision free such as when following waypoints when constructing,
-- mining, farming etc.
function stepTowardsWaypoint(wp)
    local orient = curOrient()
    for i = 1, 4 do
        if i > 3 then
            -- We have reached waypoint.
            return true, nil
        elseif orient.pos[i] ~= wp[i] then
            -- Move in this dimension.
            local dir = {0, 0, 0}
            dir[i] = (orient.pos[i] < wp[i] and 1) or -1
            local move_ok = move(dir)
            if not move_ok then
                return false, ("stepTowardsWaypoint: move " .. fmt(dir) .. " failed")
            end
            return false, nil
        end
    end
end

function dynamicMine()
    for i = 1, 6 do
        local orient = curOrient()
        local dir
        if i <= 4 then
            dir = vec3_rot90_y(orient.rot, false)
        elseif i == 5 then
            dir = {0, -1, 0}
        elseif i == 6 then
            dir = {0, 1, 0}
        end
        -- Inspect in this direction.
        local detect_ok, block = detect(dir)
        if not detect_ok then
            workError("mine: detect " .. fmt(dir) .. " failed")
            return
        end
        if block then
            local inspect_ok, details = inspect(dir)
            if not inspect_ok then
                workError("mine: inspect " .. fmt(dir) .. " failed")
                return
            end
            if not commonBlock(details) then
                -- Found uncommon block, dig it.
                local dig_ok = dig(dir)
                if not dig_ok then
                    workError("mine: dig " .. fmt(dir) .. " failed")
                    return
                end
            end
        end
    end
end

function orientate()
    -- Use hard coded peers.
    local peers = {
        ["10.1"] = {
            pos = {50, 118, 827},
            rot = {-1, 0, 0},
        },
        ["10.2"] = {
            pos = {80, 61, 806},
            rot = {0, 0, 1},
        },
        ["10.3"] = {
            pos = {57, 122, 802},
            rot = {-1, 0, 0},
        },
    }
    local down_ok, det_down = turtle.inspectDown()
    if not down_ok then
        return false, nil
    end
    if det_down.name ~= orient_block_name then
        return false, nil
    end
    local up_ok, det_up = turtle.inspectUp()
    if not up_ok then
        return false, nil
    end
    if det_up.name ~= orient_block_name then
        return false, nil
    end
    local index = (tostring(det_down.metadata) .. "." .. tostring(det_up.metadata))
    local orient = peers[index]
    if orient == nil then
        return false, nil
    end
    return true, orient
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
        local detail = getItemDetail(cur_slot)
        if detail ~= nil and detail.id == refuel_item_id then
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

function flashKernel(new_kernel)
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

function upgradeKernel()
    debug("checking for kernel upgrade")
    local h = http.get(base_url .. "/version", data)
    local rcode = (h ~= nil and h.getResponseCode()) or 0
    if rcode ~= 200 then
        debug("upgrade: failed+skipped, bad status code [" .. fmt(rcode) .. "]")
        if h ~= nil then
            h.close()
        end
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
    flashKernel(new_kernel)
end

-- Returns inventory count.
function inventoryCount()
    local inv_count = {
        free_slots = 0,
        grouped = {},
    }
    for i = 1, 16 do
        local detail = getItemDetail(i)
        if detail == nil then
            inv_count.free_slots = inv_count.free_slots + 1
        else
            local count = inv_count.grouped[detail.id] or 0
            inv_count.grouped[detail.id] = count + detail.count
        end
    end
    return inv_count
end

-- Packs the inventory at position pos by moving it to other positions.
-- pos: position to move inventory from.
-- high: highest position to consider moving to.
-- defrag: when true defrags inventory by moving remaining to lowest empty.
function inventoryPack(pos, high, defrag)
    local dt_from = getItemDetail(pos)
    if dt_from == nil then
        return
    end
	local ok = turtle.select(pos)
    if not ok then
        debug("inventoryPack: turtle.select() failed")
        return
    end
    -- Find free slots to move inventory to.
    local low_free = 0
    for i = 1, high do
        for _ = 1, 1 do
            if i == pos then
                break
            end
            local dt_to = getItemDetail(i)
            if dt_to == nil then
                if low_free == 0 then
                    low_free = i
                end
                break
            end
            if dt_to.id ~= dt_from.id then
                break
            end
            local spc = turtle.getItemSpace(i)
            if spc == 0 then
                break
            end
            -- Found slot to move to, transfer.
            turtle.transferTo(i)
            -- Update details at pos and return if completed packing.
            dt_from = getItemDetail(pos)
            if dt_from == nil then
                return
            end
        end
    end
    -- Defragment if requested.
    if defrag and low_free ~= 0 and low_free < pos then
        turtle.transferTo(low_free)
    end
end

-- For slowing down operation slightly and communicating often.
-- Common for low priority waits that can be interrupted.
function lowPrioWait()
    if timeSinceReport() > 6 then
        trigger_report()
    end
    os.sleep(2)
end

function executeWorkIdle(work)
    local instr = work.instructions
    os.sleep(instr.time)
    work.complete = true
    saveCurWork()
end

function executeWorkGo(work)
    local wp_stack = work.instructions.waypoint_stack
    if #wp_stack == 0 then
        -- Complete.
        work.complete = true
    else
        -- Move to next coordinate.
        local wp = wp_stack[#wp_stack]
        moveTo(wp, 0)
        -- Update work if still current.
        -- This is cancellation and work update safe.
        table.remove(wp_stack)
    end
    saveCurWork()
end

function executeWorkSuck(work)
    -- Look in the direction we want to suck.
    local instr = work.instructions
    local ok, lret = look(instr.dir)
    if not ok then
        workError("executeWorkSuck: turning failed")
        return
    end
    local suck_done
    local suck_total = 0
    while true do
        -- Check if specific suck is complete.
        if instr.item_id ~= nil and instr.amount <= 0 then
            suck_done = true
            break
        end
        -- Find available occupied slot.
        local dst_slot = 0
        local pre_amount
        if instr.item_id ~= nil then
            for i = 1, 16 do
                local detail = getItemDetail(i)
                if detail ~= nil and detail.id == instr.item_id then
                    local spc = turtle.getItemSpace(i)
                    if spc > 0 then
                        dst_slot = i
                        pre_amount = detail.count
                        break
                    end
                end
            end
        end
        if dst_slot == 0 then
            -- Suck to empty slot.
            for i = 1, 16 do
                if turtle.getItemCount(i) == 0 then
                    dst_slot = i
                    pre_amount = 0
                    break
                end
            end
            if dst_slot == 0 then
                -- No free slot to suck to, we definitely want to consider
                -- us done here both in the general and specific case.
                -- The controller should notice our out of space condition
                -- and react accordingly.
                debug("suck: complete: no free slot remains")
                suck_done = true
                break
            end
        end
        -- Select destination slot to suck to.
    	local select_ok = turtle.select(dst_slot)
        if not select_ok then
            workError("executeWorkSuck: turtle.select() failed")
            return
        end
        -- Suck now.
    	local suck_ok
        if instr.item_id == nil then
            if lret == look_fwd then
                suck_ok = turtle.suck()
            elseif lret == look_up then
                suck_ok = turtle.suckUp()
            elseif lret == look_down then
                suck_ok = turtle.suckDown()
            end
        else
            n_suck_here = math.min(instr.amount, 64)
            if lret == look_fwd then
                suck_ok = turtle.suck(n_suck_here)
            elseif lret == look_up then
                suck_ok = turtle.suckUp(n_suck_here)
            elseif lret == look_down then
                suck_ok = turtle.suckDown(n_suck_here)
            end
        end
        if not suck_ok then
            -- No more items. We are done only if general suck and got more
            -- than one item or specific suck and got zero items.
            debug("suck: no more items")
            suck_done = (instr.item_id == nil and suck_total > 0) or
                (instr.item_id ~= nil and suck_total == 0)
            break
        end
        -- Got at least one item.
        -- Sanity check, count the number of sucked items and adjust amount.
        local detail = getItemDetail(dst_slot)
        if detail == nil then
            fatalError("inventory error, nil slot #" .. fmt(dst_slot) .. " after successful suck")
            return
        end
        if instr.item_id ~= nil and detail.id ~= instr.item_id then
            fatalError("inventory error, suck produced the wrong item: got: " ..
                fmt(detail.id) .. ", expected: " .. fmt(instr.item_id))
            return
        end
        local this_total = detail.count - pre_amount
        suck_total = suck_total + this_total
        instr.amount = instr.amount - this_total
        -- We need to pack after sucking if generic suck.
        -- Packing not required for specific suck since partial stacks are
        -- chosen over free slots.
        if instr.item_id == nil then
            inventoryPack(dst_slot, 16, false)
        end
    end
    if suck_done then
        -- Job complete. Cases:
        -- 1. Specific suck and amount reached. (normal)
        -- 2. Out of free slots. (normal)
        -- 3. General suck and out of items in container after
        --    at least one successfully sucked item. (normal)
        -- 4. Specific suck and out of items in container before
        --    the full amount of requested items to suck was reached.
        --    This can be caused by an out of sync between the controllers
        --    understanding of how many items where in the box or by a suck
        --    miscount that the controller should correct for by counting
        --    items in inventory. In any case it's the controllers problem.
        work.complete = true
        saveCurWork()
    else
        -- Job not complete. Cases:
        -- 1. General suck and out of items in container before even one item
        --    was sucked. This is a normal blocking condition, waiting for the
        --    container to get more than zero item before continuing. (normal)
        if suck_total > 0 then
            saveCurWork()
        end
        lowPrioWait()
    end
end

function executeWorkDrop(work)
    debug("dropping items")
    -- Look in the direction we want to drop.
    local instr = work.instructions
    local ok, lret = look(instr.dir)
    if not ok then
        workError("executeWorkDrop: turning failed")
        return
    end
    -- Clone items to drop.
    debug("dropping1: " .. fmt(instr))
    local new_items = {}
    for item_id, amount in pairs(instr.items) do
        if amount ~= nil and amount > 0 then
            new_items[item_id] = amount
        end
    end
    debug("dropping2: " .. fmt(new_items))
    -- Go through all slots.
    local out_of_space = false
    local n_total_dropped = 0
    for i = 1, 16 do
        for _ = 1, 1 do
            local detail = getItemDetail(i)
            if detail == nil then
                break
            end
            local n_to_drop = new_items[detail.id]
            if n_to_drop == nil or n_to_drop <= 0 then
                break
            end
            debug("dropping " .. fmt(n_to_drop) ..  " "  .. fmt(detail.id) .. " in slot " .. fmt(i))
            -- Select destination slot to drop from.
            local select_ok = turtle.select(i)
            if not select_ok then
                workError("executeWorkDrop: turtle.select() failed")
                return
            end
            -- Drop now.
            n_drop_here = math.min(n_to_drop, detail.count)
            if lret == look_fwd then
                turtle.drop(n_drop_here)
            elseif lret == look_up then
                turtle.dropUp(n_drop_here)
            elseif lret == look_down then
                turtle.dropDown(n_drop_here)
            end
            -- Count the number of dropped items.
            local post_count = turtle.getItemCount(i)
            local n_dropped = detail.count - post_count
            if n_dropped < n_to_drop then
                -- Container is partially out of space.
                out_of_space = true
            end
            n_total_dropped = n_total_dropped + n_dropped
            local remaining = n_to_drop - n_dropped
            if remaining > 0 then
                new_items[detail.id] = remaining
            else
                new_items[detail.id] = nil
            end
            -- When one item or more remains after dropping we have a partial
            -- stack that must be packed.
            if post_count > 0 then
                inventoryPack(i, 16, false)
            end
        end
    end
    -- Check if we are done.
    if #new_items == 0 or not out_of_space then
        if #new_items > 0 then
            -- This is definitely worth logging to a better place.
            -- It's not fatal as a race condition between side effects of work and storing updated work
            -- can cause it. In this condition the controller should detect the problem and react appropriately.
            workError("executeWorkDrop: asked to drop non-existing items: " .. fmt(new_items))
        end
        -- Job complete.
        work.items = nil
        work.complete = true
        saveCurWork()
    else
        -- Not done. All items have not been dropped yet because container
        -- is out of space. Wait for container to free up.
        workError("executeWorkDrop: container is out of space")
        if n_total_dropped > 0 then
            work.items = new_items
            saveCurWork()
        end
        lowPrioWait()
    end
end

function executeWorkRefuel(work)
    local instr = work.instructions
    local remaining = instr.count
    for i = 1, 16 do
        if remaining <= 0 then
            break
        end
        -- Check if found fuel item in this slot.
        local detail = getItemDetail(i)
        if detail ~= nil and detail.id == instr.item then
            -- Fuel found, select slot.
            local success = turtle.select(i)
            if not success then
                workError("executeWorkRefuel: selecting slot " .. fmt(i) .. " failed")
                return
            end
            -- Refuel the amount we want.
            local n_refuel = detail.count
            if n_refuel > remaining then
                n_refuel = remaining
            end
            local success = turtle.refuel(n_refuel)
            if not success then
                workError("executeWorkRefuel: refuel() failed")
                return
            end
            remaining = remaining - n_refuel
        end
    end
    -- Job complete.
    work.complete = true
    saveCurWork()
end

function executeWorkQueue(work)
    local instr = work.instructions
    if instr.state == nil then
        -- First attempt to reach queue t0.
        debug("queue: walking to @ " .. fmt(instr.origin))
        local t0_pos = vec_add(vec_add(instr.origin, instr.o_q0_dir), instr.q0_t0_dir)
        moveTo(t0_pos, 0)
        instr.state = 1
        saveCurWork()
        debug("queue: reached queue t lane, scanning for q slot")
        return
    elseif instr.state == 1 then
        debug("queue: moving in t lane")
        while true do
            -- Attempt to go from t to q first.
            local t2q_ok = move(vec3_inv_dir(instr.q0_t0_dir))
            if t2q_ok then
                debug("queue: reached queue q lane, queuing for q0")
                instr.state = 2
                saveCurWork()
                return
            end
            -- Face in reverse queue direction to walk down t.
            local tfwd_ok = move(vec3_inv_dir(instr.q_dir))
            if not tfwd_ok then
                debug("stuck in queue t lane (reached end?)")
                lowPrioWait()
                return
            end
        end
    elseif instr.state == 2 then
        debug("queue: moving in q lane")
        local q0_pos = vec_add(instr.origin, instr.o_q0_dir)
        while true do
            -- Have we reached q0?
            local orient = curOrient()
            if vec_equal(q0_pos, orient.pos) then
                debug("queue: reached queue q0, queueing for o")
                instr.state = 3
                saveCurWork()
                return
            end
            -- Attempt to move forward in queue.
            local qfwd_ok = move(instr.q_dir)
            if not qfwd_ok then
                debug("waiting in queue q lane")
                lowPrioWait()
                return
            end
        end
    elseif instr.state == 3 then
        debug("queue: moving from q0 to o")
        local o_ok = move(vec3_inv_dir(instr.o_q0_dir))
        if o_ok then
            debug("queue: reached o, queuing complete")
            work.complete = true
            saveCurWork()
            return
        else
            lowPrioWait()
            return
        end
    else
        fatalError("queue: unknown state " .. fmt(instr.state))
    end
end

function executeWorkMine(work)
    local instr = work.instructions
    local wp_stack = instr.waypoint_stack
    if #wp_stack == 0 then
        -- Complete.
        work.complete = true
    else
        -- Move to next coordinate.
        local wp = wp_stack[#wp_stack]
        local there = false
        while not there do
            -- Mine to next position.
            local orient = curOrient()
            for i = 1, 4 do
                if i > 3 then
                    -- We have reached waypoint.
                    there = true
                    break
                elseif orient.pos[i] ~= wp[i] then
                    -- Move in this dimension.
                    local dir = {0, 0, 0}
                    dir[i] = (orient.pos[i] < wp[i] and 1) or -1
                    -- Mine block in path.
                    local mine_ok, err = mine(dir, instr.clear)
                    if not mine_ok then
                        workError("mine: mine failed: " .. err)
                        return
                    end
                    local move_ok = move(dir)
                    if not move_ok then
                        workError("mine: move " .. fmt(dir) .. " failed")
                        return
                    end
                    -- Mine in extra directions.
                    for i,dir in ipairs(instr.extra_dirs) do
                        local mine_ok, err = mine(dir, instr.clear)
                        if not mine_ok then
                            workError("mine: mine failed: " .. err)
                            return
                        end
                    end
                    -- Handle dynamic mining.
                    if instr.dynamic then
                        dynamicMine()
                    end
                    break
                end
            end
        end
        -- Update work if still current.
        -- This is cancellation and work update safe.
        table.remove(wp_stack)
    end
    saveCurWork()
end

function executeWorkConstruct(work)
    local instr = work.instructions

    local wp_stack = instr.waypoint_stack
    if #wp_stack == 0 then
        -- Complete.
        work.complete = true
    else
        -- Move to next coordinate.
        local wp = wp_stack[#wp_stack]
        local there = false
        while not there do
            -- Place what we construct if there is room.
            -- This check is required to prevent race (place -> reboot -> place).
            local detect_ok, block = detect(instr.dir)
            if not detect_ok then
                workError("construct: detect " .. fmt(instr.dir) .. " failed")
                return
            end
            if not block then
                -- Select block and place it.
                for i = 1, 17 do
                    if i > 16 then
                        workError("construct: out of " .. fmt(instr.item))
                        return
                    end
                    local detail = getItemDetail(i)
                    if detail ~= nil and detail.id == instr.item then
                        local select_ok = turtle.select(i)
                        if not select_ok then
                            workError("construct: selecting slot " .. fmt(i) .. " failed")
                            return
                        end
                        local place_ok = place(instr.dir)
                        if not place_ok then
                            workError("construct: place " .. fmt(instr.dir) .. " failed")
                            return
                        end
                    end
                end
            end
            -- Move to next mine position.
            local err
            there, err = stepTowardsWaypoint(wp)
            if err ~= nil then
                workError("construct: step error: " .. err)
                return
            end
        end
        -- Update work if still current.
        -- This is cancellation and work update safe.
        table.remove(wp_stack)
    end
    saveCurWork()
end

function executeWorkFarm(work)
    local harvest_limit = 7 -- Harvest crops when they reach this level.
    local instr = work.instructions

    local wp_stack = instr.waypoint_stack
    if #wp_stack == 0 then
        -- Complete.
        work.complete = true
    else
        -- Move to next coordinate.
        local wp = wp_stack[#wp_stack]
        local there = false
        local down = {0, -1, 0}
        while not there do
            -- First determine the seed we use here.
            local orient = curOrient()
            local seed_item_id = instr.items[(orient.pos[instr.mod_dim] % #instr.items) + 1]
            -- Inspect down and determine if we should harvest and/or plant.
            local do_plant
            local success, detail = inspect(down)
            if success then
                if detail.metadata < harvest_limit then
                    -- Skip, not yet mature or other block.
                    do_plant = false
                else
                    -- Harvest!
                    turtle.digDown()
                    do_plant = true
                end
            else
                -- Ensure dirt is tilled for planting.
                turtle.digDown()
                do_plant = true
            end
            if do_plant then
                -- Select block and place it.
                for i = 1, 17 do
                    if i > 16 then
                        workError("farm: out of " .. fmt(seed_item_id))
                        return
                    end
                    local detail = getItemDetail(i)
                    if detail ~= nil and detail.id == seed_item_id then
                        local select_ok = turtle.select(i)
                        if not select_ok then
                            workError("farm: selecting slot " .. fmt(i) .. " failed")
                            return
                        end
                        local place_ok = place(down)
                        if not place_ok then
                            workError("farm: place " .. fmt(down) .. " failed")
                            return
                        end
                    end
                end
            end
            -- Move to next farm position.
            local err
            there, err = stepTowardsWaypoint(wp)
            if err ~= nil then
                workError("farm: step error: " .. err)
                return
            end
        end
        -- Update work if still current.
        -- This is cancellation and work update safe.
        table.remove(wp_stack)
    end
    saveCurWork()
end

function executeWork(work)
    if work.type == "idle" then
        executeWorkIdle(work)
    elseif work.type == "go" then
        executeWorkGo(work)
    elseif work.type == "suck" then
        executeWorkSuck(work)
    elseif work.type == "drop" then
        executeWorkDrop(work)
    elseif work.type == "refuel" then
        executeWorkRefuel(work)
    elseif work.type == "queue" then
        executeWorkQueue(work)
    elseif work.type == "mine" then
        executeWorkMine(work)
    elseif work.type == "construct" then
        executeWorkConstruct(work)
    elseif work.type == "farm" then
        executeWorkFarm(work)
    else
        fatalError("unknown work type: " .. fmt(work.type))
    end
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
    local new_work = nil
    local cur_action = nil
    local cur_dst = nil
    local cur_best_dist = nil -- best distance so far, reset on new move
    local cur_pivot = nil -- pivot configuration, reset on completed pivot or new move
    local cur_frustration = 0 -- frustration, reset on new move, determines pivot aggressiveness
    local last_mdir = {1, 0, 0} -- last move direction, required as looking up or down is not possible
    local cur_pos = fs_state_get("cur_pos", nil)
    local cur_rot = fs_state_get("cur_rot", nil)
    local cur_work = fs_state_get("cur_work", nil)
    local fatal_err = nil
    local refuel_err = nil
    local work_err = nil
    local last_report_time = 0

    -- Autonomous refueling refuels as little as possible
    -- as it's only used as a last resort.
    local refuel_min = 1
    local refuel_max = 100

    -- Maximum number of brain ticks per second.
    local max_fps = 2

    function fatalError(err)
        local full_err = "fatal error: " .. fmt(err)
        debug(full_err)
        fatal_err = full_err
    end

    function workError(err)
        local full_err = "work error: " .. fmt(err)
        debug(full_err)
        work_err = full_err
    end

    function curOrient()
        return {pos = cur_pos, rot = cur_rot}
    end

    function timeSinceReport()
        return os.clock() - last_report_time
    end

    -- When updates to a reference to what is presumably the current work
    -- has been made, this function is called to save those updates if that
    -- work is still the current work. If it's not the current work this
    -- function has no effect.
    function saveCurWork()
        fs_state_put("cur_work", cur_work)
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
            -- We're stuck.
            if cur_dist <= 1 then
                debug("step failed, destination occupied")
                -- Have a 20% chance to move in inverse blocked direction to resolve deadlock.
                if math.random(1, 5) == 1 then
                    debug("attempting to resolve 'swap-d1' deadlock")
                    for i,blocked_dir in ipairs(blocked_dirs) do
                        local try_dir = {
                            blocked_dir[1] * -1,
                            blocked_dir[2] * -1,
                            blocked_dir[3] * -1,
                        }
                        local move_ok = move(try_dir)
                        if move_ok then
                            return false
                        end
                    end
                end
                os.sleep(2)
                return false
            end
            -- Pivot, Pick a random blocked dir.
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

    local actionKernelUpdate = (function()
        if new_kernel ~= nil then
            debug("got new kernel, flashing it")
            flashKernel(new_kernel)
        end
        return false
    end)

    -- Fuel management.
    local actionFuelMng = (function()
        manageFuel()
        -- Ignore fuel management failure. It is still possible that we can
        -- continue, for example if we are next to fuel deposit.
        return false
    end)

    local actionMngOrient = (function()
        if cur_pos ~= nil and cur_rot ~= nil then
            return false
        end
        debug("orientation required")
        local ok, orient = orientate()
        if not ok then
            fatalError("orientation failed")
            return true
        end
        -- Set oriented position.
        cur_pos = orient.pos
        fs_state_put("cur_pos", cur_pos)
        -- Rotation after orientation is complete is +z.
        cur_rot = orient.rot
        fs_state_put("cur_rot", cur_rot)
        debug("orientation complete: " .. fmt(cur_pos) .. " " .. fmt(cur_rot))
        return true
    end)

    local actionJobExec = (function()
        -- Load new work now if available.
        if new_work ~= nil then
            debug("loading new work")
            cur_work = new_work
            new_work = nil
            work_err = nil
            saveCurWork()
        end
        if cur_work == nil or cur_work.complete then
            debug("no work available, waiting for report ok")
            os.pullEvent("user.report-ok")
            return true
        end
        -- Has work, execute it. When work can be partially completed
        -- (and/or cancelled) this function returns and should be safe/correct
        -- to call multiple times since all work progress is stored in the
        -- job object we pass to it.
        executeWork(cur_work)
        if cur_work.complete then
            -- Work complete, trigger report to get new work.
            trigger_report()
        end
        return true
    end)

    -- Reporting.
    function try_report()
        debug("reporting: sending")
        last_report_time = os.clock()
        local work = nil
        if cur_work ~= nil then
            work = {
                id = cur_work.id,
                type = cur_work.type,
                complete = (cur_work.complete == true),
            }
        end
        local data = textutils.serializeJSON({
            new_kernel = (new_kernel ~= nil),
            version = version,
            label = os.getComputerLabel(),
            cur_action = cur_action,
            cur_dst = cur_dst,
            cur_best_dist = cur_best_dist,
            cur_pivot = cur_pivot,
            cur_frustration = cur_frustration,
            cur_pos = cur_pos,
            cur_rot = cur_rot,
            cur_work = work,
            fatal_err = fatal_err,
            refuel_err = refuel_err,
            work_err = work_err,
            fuel_lvl = turtle.getFuelLevel(),
            inv_count = inventoryCount(),
        })
        local h = http.post(base_url .. "/report", data)
        local rcode = (h ~= nil and h.getResponseCode() or 0)
        if rcode ~= 200 then
            debug("reporting: failed, bad status code [" .. tostring(rcode) .. "]")
            if h ~= nil then
                h.close()
            end
            return false
        end
        local raw_rsp = h.readAll()
        h.close()
        -- Unserialize response table.
        local rsp = textutils.unserialize(raw_rsp)
        if type(rsp) ~= "table" then
            debug("reporting: failed, non-table response")
            return false
        end
        if rsp.new_job ~= nil then
            -- We where assigned new work.
            new_work = rsp.new_job
        end
        if rsp.new_kernel ~= nil then
            -- We where assigned new kernel.
            new_kernel = rsp.new_kernel
        end
        debug("reporting: completed ok")
        os.queueEvent("user.report-ok")
        return true
    end

    function trigger_report()
        last_report_time = os.clock()
        os.queueEvent("user.report")
    end

    -- Main brain.
    local brainTick = (function()
        -- Actions in order of priority
        actionSequence = {
            -- Priority 1: Ensure kernel is up to date.
            {fn = actionKernelUpdate, name = "kernel"},
            -- Priority 2: Ensure we have sufficient fuel to operate.
            {fn = actionFuelMng, name = "fuel"},
            -- Priority 3: Ensure we have completed orientation.
            {fn = actionMngOrient, name = "orientation"},
            -- Priority 4: Execute jobs.
            {fn = actionJobExec, name = "job"},
        }
        for i,action in ipairs(actionSequence) do
            if fatal_err then
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
        -- Report whenever reporting is requested.
        while true do
            os.pullEvent("user.report")
            report()
        end
    end), (function()
        -- Report automatically every 30 seconds.
        while true do
            trigger_report()
            sleep(30)
        end
    end), (function()
        -- Brain tick with a one second rate limit in case of hysterical panic.
        local max_spf = 1 / max_fps
        while true do
            local t0 = os.clock()
            if not brainTick() then
                debug("fatal error in brain: enabling permanent apathy")
                return
            end
            -- Apply maximum brain tick fps limit.
            local dt = os.clock() - t0
            local wait = max_spf - dt
            if wait > 0 then
                os.sleep(wait)
            end
        end
    end))
end)()
`
