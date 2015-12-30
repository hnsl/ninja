package main

var lua_src_kernel = `
version = 1

local base_url = "http://skogen.twitverse.com:4456/72ceda8b"
local state_root = "/state"

local role_server = 0 -- Server vm, "server"
local role_miner = 1  -- Miner computer, "miner_$MINEID_$TURTLEID"

-- Debugging.
function fmt(x)
    if is_server == nil then
        return textutils.serializeJSON(x)
    else
        return JSON:encode(x)
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
    fs.move(tmp_path)
end

function fs_state_get(name, default)
    local path = state_root .. "/" .. name
    if not path.exists(path) then
        fs_state_put(name, default)
        return default
    end
    local h = fs.open(path, "r")
    local data = h.readAll()
    h.close()
    return textutils.unserialize(data)
end

-- Identity parser.
function parse_identity()
    local label = (is_server ~= nil and 'server' or os.getComputerLabel())
    debug("kernel: label [" .. label .. "]")
    local lparts = split(label, "_")
    if lparts[1] == "server" then
        return {
            role = role_server,
            parent = nil,
        }
    elseif lparts[1] == "miner" then
        return {
            role = role_miner,
            mine_id = lparts[2],
            turtle_id = lparts[3],
            parent = ("mine." .. lparts[2]),
        }
    else
        error("invalid role: " .. label)
    end
end

-- Parse role.

debug("kernel: booting, parsing role")
identity = parse_identity()
debug("kernel: " .. fmt(role))

-- Robot specific functions.

function orcomp()
    local comp = 0
    for i = 0, 3, 1 do
        local ok, bdata
        if (i % 2) == 0 then
            if not turtle.detectUp() then
                return nil, "missing up block"
            end
            ok, bdata = turtle.inspectUp()
        else
            if not turtle.detectDown() then
                return nil, "missing down block"
            end
            ok, bdata = turtle.inspectDown()
        end
        if not ok then
            return nil, ("inspect failed: " .. fmt(bdata))
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
    debug("main orientation sequence start")
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
    debug("main orientation sequence complete: " .. fmt(coord))
    return coord, nil
end

function main_miner()
    debug("miner: loading turtle state")
    -- State.
    local new_kernel = nil
    local pos = fs_state_get("pos", nil)
    local work_q = fs_state_get("work_q", {})
    -- Reporting.
    local try_report = (function()
        debug("miner: sending report")
        local data = textutils.serializeJSON({
            version = version,
            identity = identity,
            pos = pos,
            work_q = work_q,
            fuel_lvl = turtle.getFuelLevel(),
        })
        local h = http.post(base_url + "/report", data)
        local rcode = ret.getResponseCode()
        if rcode ~= 200 then
            debug("miner: report failed, bad status code [" .. tostring(rcode) .. "]")
            h.close()
            return false
        end
        local raw_rsp = h.readAll()
        h.close()
        -- Unserialize response table.
        local rsp = textutils.unserialize(raw_rsp)
        if type(rsp) == "table" then
            debug("miner: report failed, non-table response")
            return false
        end




        debug("miner: report submitted ok")
        return true
    end)
    local report = (function()
        -- Try reporting until reporting is successful.
        while true do
            if try_report() then
                break
            end
            sleep(2)
        end
    end)
	parallel.waitForAny((function()
        -- Report every 10 seconds.
        while true do
            report()
            sleep(10)
        end
    end), (function()
        while true do
            -- Priority 1: Ensure we have sufficient fuel to operate.

            -- Priority 2: Ensure we have completed orientation.

            -- Priority 3:

        end
    end))

end

-- Run identity role main.

if identity.id == role_server then
    debug("kernel: is server, done")
elseif identity.id == role_miner then
    main_miner()
end

`
