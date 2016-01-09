var state, item_map;

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

function nPerPlane(details) {
    return details.XLen * 2 + details.YLen * 2;
}

function sortInventoryTotals(item_totals) {
    var totals = [];
    for (var item_id in item_totals) {
        totals.push({item_id: item_id, count: item_totals[item_id]});
    }
    totals.sort(function(a, b) {
        return a.count < b.count? 1: (a.count > b.count? -1:
            (a.item_id.localeCompare(b.item_id)));
    });
    return totals;
}

function getItemDisplayName(item_id) {
    var display_name = item_map[item_id];
    if (!display_name) {
        display_name = item_map[item_id.replace(/\/\d+$/, "/0")];
    }
    return display_name;
}

function refreshInventoryUI() {
    // Recount item totals.
    var inv_id = "storage.0";
    var details = state[inv_id + "/details"];
    var item_totals = {};
    for (var key in state) {
        var match = key.match(/^(storage\.\d+)\/plane\.(\d+)$/);
        if (!match || match[1] !== inv_id) {
            continue;
        }
        //var plane_id = parseInt(match[2], 10);
        //var offset = nPerPlane(details) * plane_id;
        var plane = state[key];
        for (var i = 0; i < plane.length; i++) {
            var slot = plane[i];
            if (slot.Amount > 0) {
                if (!item_totals[slot.Name]) {
                    item_totals[slot.Name] = 0;
                }
                item_totals[slot.Name] += slot.Amount;
            }
        }
    }
    var sorted_totals = sortInventoryTotals(item_totals);
    for (var i = 0; i < sorted_totals.length; i++) {
        var stack = sorted_totals[i];
        stack.elem_id = "inv_stack_" + stack.item_id;
        stack.elem = document.getElementById(stack.elem_id);
    }
    var igrid = $("#inventory_grid");
    $(igrid).empty();
    for (var i = 0; i < sorted_totals.length; i++) {
        var stack = sorted_totals[i];
        var elem = stack.elem;
        if (!elem) {
            elem = document.createElement("div");
            var display_name = getItemDisplayName(stack.item_id);
            $(elem).attr({
                id: stack.elem_id,
                class: "stack",
                style: "background-image: url('items/icons/" + display_name + ".png');",
            }).data("stack", stack).html(
                '<span class="stack-name"></span>'
                + '<span class="stack-count"></span>'
            ).find(".stack-name").text(display_name);
        }
        $(elem).find(".stack-count").text(stack.count);
        $(igrid).append(elem);
    }
}

function refreshUI() {
    refreshInventoryUI()

}

function reconnect() {
    console.log("reconnecting to sync...");
    var ws = new WebSocket(wsRelToAbs("sync"), ["ninja"]);

    ws.onopen = function(ev) {
        state = {}
        console.log(ev)
    }

    ws.onmessage = function(ev) {
        var kvdata = JSON.parse(ev.data);
        for (var key in kvdata) {
            state[key] = kvdata[key];
        }
        refreshUI();
        ws.send("ok")
    }

    ws.onerror = function(ev) {
        console.error(ev);
        setTimeout(function() {
            reconnect();
        }, 2000);
    }

    ws.onclose = function(ev) {
        console.log(ev)
    }
}

$.get("items/item-map.json", function(data) {
    console.log(data);
    item_map = data;
    reconnect();
});

$("#inventory_grid").on("mousedown", "> .stack", function(ev) {
    ev.preventDefault();
    var elem = ev.currentTarget;
    var export_now_fn = function(count) {
        if (!count || isNaN(count) || count < 0 || count > 1e6) {
            return;
        }
        var stack = $(elem).data("stack");
        $.post("export", JSON.stringify({
            name: stack.item_id,
            count: count,
        }), function(ret) {
            console.log("export rsp:", ret);
        }, "json");
    };
    if (ev.ctrlKey || ev.metaKey) {
        $("#export_count").data("submit", false).val("");
        $("#export_prompt").show();
        $("#export_count").focus().one("blur", function(ev) {
            var submit = $(this).data("submit");
            $("#export_prompt").hide();
            if (submit) {
                var count = parseInt($(this).val(), 10);
                export_now_fn(count);
            }
        });
    } else {
        var count = ev.shiftKey? 64: 1;
        export_now_fn(count);
    }
    return false;
});

$("#export_count").on("keypress", function(ev) {
    var elem = ev.currentTarget;
    if (ev.which == 13) {
        $(elem).data("submit", true);
        $(elem).trigger("blur");
        return false;
    }
});
