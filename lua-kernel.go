package main

var lua_src_kernel = `
version = 1

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

-- Role parser.
function parse_role()
    local label = (is_server ~= nil and 'server' or os.getComputerLabel())
    debug("kernel: label [" .. label .. "]")
    local lparts = split(label, "_")
    role = {}
    if lparts[1] == "server" then
        return {
            id = role_server,
            parent = nil,
        }
    elseif lparts[1] == "miner" then
        return {
            id = role_miner,
            mine_id = lparts[2],
            turtle_id = lparts[3],
            parent = ("mine." .. lparts[2]),
        }
    else
        error("invalid role: " .. label)
    end
end

-- F

debug("kernel: booting, parsing role")
role = parse_role()
debug("kernel: " .. fmt(role))

--EOF
`
