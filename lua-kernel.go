package main; var lua_src_kernel = `
version = 5

local base_url = "http://skogen.twitverse.com:4456/72ceda8b"
local state_root = "/state"

local orient_block_name = "ExtraUtilities:color_stonebrick"

local refuel_item_name = "minecraft:coal"
local refuel_item_fpi = 80

-- Debugging.
function fmt(x)
    if is_server then
        return JSON:encode(x)
    else
    return textutils.serializeJSON(x)
    end
end

function debug(x)
    print(type(x) == "string" and x or fmt(x))
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

function orcomp()
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
        local comp, err = orcomp()
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
    local rcode = h.getResponseCode()
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

(function()
    if is_server then
        debug("kernel: server run complete")
        return
    end

    -- Upgrade kernel automatically if required.
    upgradeKernel()

    -- Initialize state.
    debug("initializing turtle state")
    local new_kernel = nil
    local cur_action = nil
    local pos = fs_state_get("pos", nil)
    local work_q = fs_state_get("work_q", {})
    local fatal_err = nil
    local refuel_err = false

    local refuel_min = 100
    local refuel_max = 32 * 80

    local fatalError = (function(err)
        local full_err = "fatal error: " .. fmt(err)
        debug(full_err)
        fatal_err = full_err
    end)

    -- Fuel management.
    local actionFuelMng = (function()
        if refuel_err then
            return false
        end
        local refuel_lvl = turtle.getFuelLevel()
        if refuel_lvl > refuel_min then
            return false
        end
        debug("refueling required (" .. fmt(refuel_lvl) .. "/" .. fmt(refuel_min) .. ")")
        while true do
            local required = refuel_max - refuel_lvl
            err = tryRefuel(required)
            if err then
                debug("refueling failed: " .. fmt(err))
                refuel_err = true
                break
            end
            refuel_lvl = turtle.getFuelLevel()
            if refuel_lvl >= refuel_max then
                break
            end
        end
        debug("refueling complete")
        return false
    end)

    local actionMngOrient = (function()
        if pos ~= nil then
            return false
        end
        debug("orientation required")
        local coord, err = orientate()
        if err ~= nil then
            fatalError("orientation failed, error: " .. fmt(err))
            return true
        end
        pos = coord
        fs_state_put("pos", pos)
        debug("orientation complete: " .. fmt(pos))
        return false
    end)

    local actionJobExec = (function()
        debug("todo: exec job")
        return true
    end)

    -- Reporting.
    function try_report()
        debug("reporting: sending")
        local data = textutils.serializeJSON({
            version = version,
            label = os.getComputerLabel(),
            cur_action = cur_action,
            pos = pos,
            work_q = work_q,
            fatal_err = fatal_err,
            refuel_err = refuel_err,
            fuel_lvl = turtle.getFuelLevel(),
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
        for i,action in pairs(actionSequence) do
            if fatal_error then
                debug("fatal error in brain: enabling permanent apathy")
                return
            end
            cur_action = action.name
            if action.fn() then
                return
            end
        end
    end)
    parallel.waitForAny((function()
        -- Report every 10 seconds.
        while true do
            report()
            sleep(10)
        end
    end), (function()
        -- Brain tick with a one second rate limit in case of hysterical panic.
        while true do
            brainTick()
            sleep(1)
        end
    end))
end)()
`
