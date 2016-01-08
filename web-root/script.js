function pathJoin(parts, sep){
   var separator = sep || '/';
   var replace   = new RegExp(separator+'{1,}', 'g');
   return parts.join(separator).replace(replace, separator);
}

function wsRelToAbs(rel) {
    var loc = window.location;
    var uri_base = (loc.protocol === "https:"? "wss:": "ws:") +
        "//" + loc.host + loc.pathname;
    return pathJoin([uri_base, rel])
}

var ws = new WebSocket(wsRelToAbs("sync"), ["ninja"]);

ws.onopen = function(ev) {
    console.log(ev)
}

ws.onmessage = function(ev) {
    console.log(JSON.parse(ev.data))
    ws.send("ok")
}

ws.onerror = function(ev) {
    console.log(ev)
}

ws.onclose = function(ev) {
    console.log(ev)
}
