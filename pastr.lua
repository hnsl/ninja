args = {...}
local h = fs.open(args[1], "w")
while true do
term.clear()
term.setCursorPos(1, 1)
local line = read()
if line == "--EOF" then
h.close()
return
end
h.writeLine(line)
end
