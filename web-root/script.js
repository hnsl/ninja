var state, item_map;

var inv_id = "storage.0";

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

function sortItemStacks(stacks) {
    var totals = [];
    for (var item_id in stacks) {
        totals.push({item_id: item_id, count: stacks[item_id]});
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
    var details = state[inv_id + "/details"];
    var inventory_totals = {};
    var exports_totals = {};
    var exporting_totals = {};
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
                if (!inventory_totals[slot.Name]) {
                    inventory_totals[slot.Name] = 0;
                }
                inventory_totals[slot.Name] += slot.Amount;
            }
        }
    }
    for (var item_id in details.Exporting) {
        var amount = details.Exporting[item_id];
        if (!exports_totals[item_id]) {
            exports_totals[item_id] = 0;
        }
        exports_totals[item_id] += amount;
    }
    for (var turtle_id in details.export_allocs) {
        var turtle_allocs = details.export_allocs[turtle_id];
        for (var item_id in turtle_allocs) {
            var amount = turtle_allocs[item_id];
            if (!exporting_totals[item_id]) {
                exporting_totals[item_id] = 0;
            }
            exporting_totals[item_id] += amount;
        }
    }
    var sorted_inventory = sortItemStacks(inventory_totals);
    var sorted_exports = sortItemStacks(exports_totals);
    var sorted_exporting = sortItemStacks(exporting_totals);
    var stack_element_map_fn = function(id_prefix, sorted_stacks) {
        for (var i = 0; i < sorted_stacks.length; i++) {
            var stack = sorted_stacks[i];
            stack.elem_id = id_prefix + stack.item_id;
            stack.elem = document.getElementById(stack.elem_id);
        }
    }
    stack_element_map_fn("inv_stack_", sorted_inventory);
    stack_element_map_fn("exp1_stack_", sorted_exports);
    stack_element_map_fn("exp2_stack_", sorted_exporting);
    var populate_grid_fn = function(grid, sorted_stacks) {
        $(grid).empty();
        for (var i = 0; i < sorted_stacks.length; i++) {
            var stack = sorted_stacks[i];
            var elem = stack.elem;
            if (!elem) {
                elem = document.createElement("div");
                var display_name = getItemDisplayName(stack.item_id);
                $(elem).attr({
                    id: stack.elem_id,
                    class: "stack",
                    style: "background-image: url('items/icons/" + display_name + ".png');",
                    "data-item_id": stack.item_id,
                }).html(
                    '<span class="stack-name"></span>'
                    + '<span class="stack-count"></span>'
                ).find(".stack-name").text(display_name);
            }
            $(elem).find(".stack-count").text(stack.count);
            $(grid).append(elem);
        }
    };
    populate_grid_fn($("#inventory_grid"), sorted_inventory);
    populate_grid_fn($("#exports_grid"), sorted_exports);
    populate_grid_fn($("#exporting_grid"), sorted_exporting);
}

function refreshTurtleUI() {
    // console.log(state);
    var turtles = [];
    for (var key in state) {
        var match = key.match(/^turtles\//);
        if (!match) {
            continue;
        }
        var turtle = state[key];
        turtles.push(turtle);
    }
    turtles.sort(function(a, b) {
        return a.label.localeCompare(b.label);
    });
    for (var i = 0; i < turtles.length; i++) {
        var turtle = turtles[i];
        turtle.elem_id = "turtle_" + turtle.label;
        turtle.elem = document.getElementById(turtle.elem_id);
    }
    var turtle_list = $("#turtle_table tbody");
    $(turtle_list).empty();
    for (var i = 0; i < turtles.length; i++) {
        var turtle = turtles[i];
        var elem = turtle.elem;
        if (!elem) {
            elem = document.createElement("tr");
            $(elem).attr({
                id: turtle.elem_id,
                class: "turtle-row",
                "data-turtle_id": turtle.label,
            }).html(
                '<td class="turtle-id"></td>'
                + '<td class="turtle-version"></td>'
                + '<td class="turtle-fuel"></td>'
                + '<td class="turtle-inventory"></td>'
                + '<td class="turtle-activity"></td>'
            ).find(".turtle-id").text(turtle.label);
        }
        $(turtle_list).append(elem);
        $(elem).find(".turtle-fuel").text(turtle.fuel_lvl);
        $(elem).find(".turtle-version").text("v" + turtle.version + (turtle.new_kernel? "*": ""));
        var activity = turtle.cur_action;
        if (turtle.cur_work) {
            activity += "/" + turtle.cur_work.type + "/" + turtle.cur_work.id;
            if (turtle.cur_work.complete) {
                activity += " (complete)";
            }
        }
        if (turtle.cur_frustration > 0) {
            activity += " (" + turtle.cur_frustration + " frustrated)";
        }
        activity += " @ " + JSON.stringify(turtle.cur_pos);
        if (turtle.cur_dst) {
            activity += " -> " + JSON.stringify(turtle.cur_dst);
        }
        $(elem).find(".turtle-activity").text(activity);
        var inventory = turtle.inv_count.free_slots + "/16";
        $(elem).find(".turtle-inventory").text(inventory);
    }
}

function refreshUI() {
    refreshInventoryUI();
    refreshTurtleUI();
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
    }

    ws.onclose = function(ev) {
        console.log(ev)
        setTimeout(function() {
            reconnect();
        }, 2000);
    }
}

$.get("items/item-map.json", function(data) {
    item_map = data;
    reconnect();
});

$("#inventory_grid, #exports_grid").on("mousedown", "> .stack", function(ev) {
    ev.preventDefault();
    var elem = ev.currentTarget;
    var export_now_fn = function(count) {
        if (!count || isNaN(count) || count < -1e6 || count > 1e6) {
            return;
        }
        if ($(elem).parents("#exports_grid").length > 0) {
            count = -count;
        }
        $.post("export", JSON.stringify({
            area_id: inv_id,
            item_id: $(elem).data("item_id"),
            count: count,
        }), function(ret) {
            console.log("export rsp:", ret);
        }, "json");
    };
    if (ev.ctrlKey || ev.metaKey) {
        $("#export_count").data("submit", false).val("");
        $("#amount_prompt").show();
        $("#export_count").focus().one("blur", function(ev) {
            var submit = $(this).data("submit");
            $("#amount_prompt").hide();
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

$("#wrapper > .page").hide();

$("#nav-hdr").on("click", "> a", function(ev) {
    ev.preventDefault();
    var elem = ev.currentTarget;
    var page = $(elem).data("nav");
    $("#wrapper > .page").hide();
    $("#wrapper > .page_" + page).show();
    $("#nav-hdr > a").removeClass("active");
    $(elem).addClass("active");
});

$("#nav-hdr > a:first-child").click();
