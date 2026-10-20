// The explorer of the person's cloud computer: the folders and files of a place in a list with their dates and sizes. Drawn into a window
// that chooses a folder (projects down the side, path along the top), or into the Files page, where the page heading is the path, a folder
// opens on one click, a file opens in the viewer and the folder can be the one to chat in.
(function () {
  var icons = {
    folder: '<svg viewBox="0 0 24 24"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg>',
    file: '<svg viewBox="0 0 24 24"><path d="M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z"/><path d="M14 2v5a1 1 0 0 0 1 1h5"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/></svg>',
    back: '<svg viewBox="0 0 24 24"><path d="m15 18-6-6 6-6"/></svg>',
    up: '<svg viewBox="0 0 24 24"><path d="m18 15-6-6-6 6"/></svg>',
    image: '<svg viewBox="0 0 24 24"><rect width="18" height="18" x="3" y="3" rx="2" ry="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21"/></svg>',
    code: '<svg viewBox="0 0 24 24"><path d="M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z"/><path d="M14 2v5a1 1 0 0 0 1 1h5"/><path d="M10 12.5 8 15l2 2.5"/><path d="m14 12.5 2 2.5-2 2.5"/></svg>',
    table: '<svg viewBox="0 0 24 24"><path d="M3 9h18"/><path d="M9 3v18"/><rect x="3" y="3" width="18" height="18" rx="2"/></svg>',
    plus: '<svg viewBox="0 0 24 24"><path d="M5 12h14"/><path d="M12 5v14"/></svg>',
    go: '<svg viewBox="0 0 24 24"><path d="m9 18 6-6-6-6"/></svg>',
    cloud: '<svg viewBox="0 0 24 24"><path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"/></svg>'
  };
  // What the explorer is dressed in. A class a script reads by name (row, folder, file, d-summary, d-choose) stays in the list beside the utilities.
  var look = {
    icon: "inline-grid flex-none place-items-center [&_svg]:size-full [&_svg]:stroke-current [&_svg]:stroke-[1.5] [&_svg]:[stroke-linecap:round] [&_svg]:[stroke-linejoin:round]",
    bare: "[&_svg]:fill-none",
    crumb: "inline-flex min-h-6 cursor-pointer items-center border-0 bg-transparent p-0 font-[inherit] text-muted hover:text-ink",
    input: "rounded-sm border border-line-strong bg-surface px-3 text-ink placeholder:text-faint",
    button: "inline-flex flex-none cursor-pointer items-center justify-center gap-2 rounded-sm border border-line-strong bg-surface whitespace-nowrap text-ink no-underline transition-colors hover:bg-wash",
    primary: "inline-flex flex-none cursor-pointer items-center justify-center gap-2 rounded-sm border-0 bg-accent whitespace-nowrap text-on-accent no-underline transition-colors hover:bg-accent-hover disabled:cursor-default disabled:opacity-40 aria-disabled:pointer-events-none aria-disabled:opacity-40",
    place: "flex w-full cursor-pointer items-center gap-2 rounded-sm border-0 bg-transparent p-2 text-left text-sm text-muted hover:bg-hover hover:text-ink aria-[current]:bg-surface aria-[current]:text-ink aria-[current]:shadow-1",
    tool: "grid size-8 cursor-pointer place-items-center rounded-sm border-0 bg-transparent p-0 text-muted hover:bg-hover hover:text-ink disabled:cursor-default disabled:opacity-35"
  };
  function icon(name, size, fill) {
    var span = document.createElement("span");
    span.className = look.icon + " " + (size || "size-4") + " " + (fill || look.bare);
    span.setAttribute("aria-hidden", "true");
    span.innerHTML = icons[name];
    return span;
  }
  // A title is given where the text may be cut short by the column it is in.
  function el(tag, className, text, title) {
    var node = document.createElement(tag);
    if (className) { node.className = className; }
    if (text !== undefined) { node.textContent = text; }
    if (title) { node.title = title; }
    return node;
  }
  function button(className, onclick, children, title) {
    var node = el("button", className);
    node.type = "button";
    if (title) { node.title = title; node.setAttribute("aria-label", title); }
    (children || []).forEach(function (child) { node.appendChild(typeof child === "string" ? document.createTextNode(child) : child); });
    node.addEventListener("click", onclick);
    return node;
  }
  var kinds = {
    image: /\.(png|jpe?g|gif|webp|svg|avif)$/i,
    table: /\.(csv|tsv|xlsx?)$/i,
    code: /\.(js|mjs|ts|tsx|jsx|go|py|rb|rs|java|c|h|cpp|css|html?|json|ya?ml|toml|sh|sql|php)$/i
  };
  function iconFor(name, size) {
    for (var kind in kinds) { if (kinds[kind].test(name)) { return icon(kind, size, "text-subtle " + look.bare); } }
    return icon("file", size, "text-subtle " + look.bare);
  }
  function when(seconds) {
    if (!seconds) { return ""; }
    var then = new Date(seconds * 1000), now = new Date();
    if (then.toDateString() === now.toDateString()) { return then.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" }); }
    return then.toLocaleDateString([], { day: "numeric", month: "short", year: then.getFullYear() === now.getFullYear() ? undefined : "numeric" });
  }
  function bytes(n) {
    if (n < 1024) { return n + " B"; }
    if (n < 1048576) { return Math.round(n / 1024) + " KB"; }
    return (n / 1048576).toFixed(1) + " MB";
  }
  function get(place) {
    return fetch("/chat/folders?at=" + encodeURIComponent(place), { credentials: "same-origin" })
      .then(function (answer) { return answer.ok ? answer.json() : Promise.reject(); });
  }

  // options: mode ("choose" or "browse"), start (the place to open at), onChoose(shown), onMove(at), extras (buttons to add to the foot),
  // and the name of the agent being used, for a folder made at the top.
  window.Explorer = function (container, options) {
    var state = { here: null, selected: "", back: [], top: [], filter: "", making: false, problem: "", refocus: false, focusNew: false };
    var browsing = options.mode === "browse";

    function load(place, remember) {
      get(place).then(function (shown) {
        if (remember && state.here) { state.back.push(state.here.at); }
        state.here = shown; state.selected = ""; state.filter = ""; state.making = false; state.problem = "";
        if (options.onMove) { options.onMove(shown.at); }
        draw();
      }).catch(function () { state.problem = "The folders could not be read."; draw(); });
    }
    function parent() {
      var crumbs = state.here.crumbs;
      return crumbs.length > 1 ? crumbs[crumbs.length - 2].at : "";
    }
    function places() { get("").then(function (top) { state.top = top.folders; draw(); }).catch(function () {}); }
    function target() { return state.selected || state.here.at; }
    function nameOf(place) {
      var chosen = state.here.folders.filter(function (f) { return f.at === place; })[0];
      return chosen ? chosen.name : state.here.crumbs[state.here.crumbs.length - 1].name;
    }
    function summaryText() {
      var place = target();
      if (!place) { return browsing ? "" : "Open a project, or make one"; }
      return (browsing ? "" : "Work in  ") + nameOf(place);
    }
    function viewer(file) {
      var parts = state.here.at.split("/");
      var dir = parts.slice(1).concat(file.name).join("/");
      return "/instances/" + encodeURIComponent(parts[0]) + "/file?path=" + encodeURIComponent(dir) + "&from=" + encodeURIComponent("/files?at=" + encodeURIComponent(state.here.at));
    }

    // Drawing again throws away the element the keyboard was on, so the list takes it back once it is drawn.
    function draw() {
      state.refocus = state.refocus || container.contains(document.activeElement);
      container.textContent = "";
      if (!state.here) { container.appendChild(el("div", "p-8 text-center text-subtle [grid-row:1/-1]", state.problem || "Looking…")); return; }
      if (browsing) {
        container.appendChild(pageHead());
        container.appendChild(list());
      } else {
        container.appendChild(toolbar());
        var body = el("div", "grid min-h-0 grid-cols-[12.5rem_minmax(0,1fr)] @max-[40rem]:grid-cols-[minmax(0,1fr)]");
        body.appendChild(placesList());
        body.appendChild(list());
        container.appendChild(body);
        container.appendChild(footer());
      }
    }

    function toolbar() {
      var bar = el("div", "flex items-center gap-1 border-b border-line px-4 pb-3");
      var back = button(look.tool, function () { load(state.back.pop() || "", false); }, [icon("back")], "Back");
      back.disabled = !state.back.length;
      var up = button(look.tool, function () { load(parent(), true); }, [icon("up")], "Up one folder");
      up.disabled = !state.here.at;
      bar.appendChild(back);
      bar.appendChild(up);
      var path = el("div", "ml-2 flex h-8 min-w-0 flex-1 items-center gap-1 overflow-hidden rounded-sm bg-wash px-3 text-xs whitespace-nowrap");
      state.here.crumbs.forEach(function (crumb, i) {
        if (i > 0) { path.appendChild(el("span", "inline-flex items-center text-subtle", "›")); }
        var last = i === state.here.crumbs.length - 1;
        path.appendChild(last ? el("span", "inline-flex min-h-6 items-center overflow-hidden font-semibold text-ellipsis text-ink", crumb.name, crumb.name) : button(look.crumb + " text-xs hover:underline", function () { load(crumb.at, true); }, [crumb.name]));
      });
      bar.appendChild(path);
      bar.appendChild(filterBox("ml-2 h-8 w-36 text-xs @max-[40rem]:hidden"));
      return bar;
    }

    function filterBox(size) {
      var filter = el("input", look.input + " " + size);
      filter.type = "search"; filter.placeholder = "Filter"; filter.value = state.filter; filter.setAttribute("aria-label", "Filter this folder");
      filter.addEventListener("input", function () { state.filter = filter.value; drawList(); });
      return filter;
    }

    // The Files page has no window around it: the way down is the heading itself, and what can be done here sits to its right.
    function pageHead() {
      var head = el("div", "mb-6 flex min-h-10 flex-wrap items-center gap-x-4 gap-y-3");
      var trail = el("h1", "flex min-w-0 flex-[1_1_14rem] items-center gap-2");
      state.here.crumbs.forEach(function (crumb, i) {
        var name = i === 0 ? "Files" : crumb.name;
        if (i > 0) { trail.appendChild(el("span", "font-normal text-line-strong", "/")); }
        trail.appendChild(i === state.here.crumbs.length - 1 ? el("span", "min-w-0 truncate", name, name) : button(look.crumb + " text-[length:inherit] font-[inherit]", function () { load(crumb.at, true); }, [name]));
      });
      head.appendChild(trail);
      var tools = el("div", "flex min-w-0 flex-[1_1_22rem] items-center justify-end gap-2");
      tools.appendChild(filterBox("h-9 min-w-24 flex-[1_1_8rem] text-sm"));
      tools.appendChild(button(look.button + " h-9 px-4 text-sm font-medium", function () { state.making = true; state.focusNew = true; state.problem = ""; draw(); }, [icon("plus", "size-[1.125rem]"), state.here.at ? "New folder" : "New project"]));
      if (state.here.at) {
        var chat = el("a", look.primary + " d-choose h-9 px-4 text-sm font-semibold", "Chat here");
        chat.href = "/chat?at=" + encodeURIComponent(state.here.at);
        tools.appendChild(chat);
      }
      head.appendChild(tools);
      return head;
    }

    function placesList() {
      var side = el("nav", "flex min-h-0 flex-col gap-px overflow-y-auto border-r border-line bg-wash p-3 @max-[40rem]:hidden");
      side.setAttribute("aria-label", "Projects");
      var top = button(look.place + " font-semibold text-ink", function () { load("", true); }, [icon("cloud"), el("span", "truncate", "Your cloud workspace")]);
      if (!state.here.at) { top.setAttribute("aria-current", "true"); }
      side.appendChild(top);
      state.top.slice(0, 14).forEach(function (project) {
        var current = state.here.at === project.at || state.here.at.indexOf(project.at + "/") === 0;
        var place = button(look.place + " pl-8", function () { load(project.at, true); }, [icon("folder"), el("span", "truncate", project.name)], project.name);
        if (current) { place.setAttribute("aria-current", "true"); }
        side.appendChild(place);
      });
      return side;
    }

    function list() {
      var box = el("div", browsing ? "overflow-hidden rounded-md bg-surface shadow-1 dark:border dark:border-line" : "flex min-h-0 min-w-0 flex-col");
      var head = el("div", rowGrid() + " " + (browsing ? "min-h-10 border-b border-line bg-wash px-5" : "min-h-8 border-b border-line px-6") + " text-xs font-medium text-muted");
      head.appendChild(el("span", "", "Name")); head.appendChild(el("span", "@max-[40rem]:hidden", "Modified")); head.appendChild(el("span", "text-right", "Size"));
      box.appendChild(head);
      var rows = el("div", browsing ? "[&>*+*]:border-t [&>*+*]:border-line" : "min-h-0 flex-1 overflow-y-auto px-2 py-1");
      rows.id = "d-rows";
      if (!browsing) { rows.setAttribute("role", "listbox"); rows.setAttribute("aria-label", "Folders"); }
      box.appendChild(rows);
      setTimeout(drawList, 0);
      return box;
    }
    function rowGrid() {
      return "grid items-center " + (browsing ? "gap-4 grid-cols-[minmax(0,1fr)_8rem_5rem]" : "gap-3 grid-cols-[minmax(0,1fr)_8.5rem_4.5rem]") + " @max-[40rem]:grid-cols-[minmax(0,1fr)_4.5rem]";
    }
    function rowLook() { return rowGrid() + (browsing ? " min-h-12 px-5 text-base" : " min-h-9 rounded-sm px-4 text-sm") + " focus-visible:-outline-offset-2"; }
    function nameLook() { return "flex min-w-0 items-center gap-3" + (browsing ? " font-medium" : ""); }
    function drawList() {
      var rows = container.querySelector("#d-rows");
      if (!rows) { return; }
      rows.textContent = "";
      var needle = state.filter.trim().toLowerCase();
      if (state.making) { rows.appendChild(makingRow()); }
      var shown = 0;
      state.here.folders.filter(function (f) { return f.name.toLowerCase().indexOf(needle) >= 0; }).forEach(function (folder) {
        shown++;
        var row = el("div", "row folder " + rowLook() + " cursor-pointer text-ink hover:bg-wash aria-selected:bg-wash aria-selected:outline-1 aria-selected:-outline-offset-1 aria-selected:outline-subtle" + (state.selected === folder.at ? " selected" : ""));
        row.dataset.at = folder.at;
        row.tabIndex = -1;
        row.setAttribute("role", browsing ? "link" : "option");
        if (browsing) { row.tabIndex = 0; } else { row.setAttribute("aria-selected", String(state.selected === folder.at)); }
        row.addEventListener("keydown", function (event) { if (browsing && event.key === "Enter") { load(folder.at, true); } });
        var name = el("span", nameLook()); name.appendChild(icon("folder", browsing ? "size-5" : "size-4", "text-muted [&_svg]:fill-subtle/35")); name.appendChild(el("span", "truncate", folder.name, folder.name));
        row.appendChild(name); row.appendChild(el("span", "text-xs whitespace-nowrap text-muted @max-[40rem]:hidden", when(folder.modified))); row.appendChild(el("span", "flex justify-end text-subtle"));
        row.lastChild.appendChild(icon("go"));
        row.addEventListener("click", function () { if (browsing) { load(folder.at, true); } else { state.selected = folder.at; mark(); } });
        row.addEventListener("dblclick", function () { load(folder.at, true); });
        rows.appendChild(row);
      });
      state.here.files.filter(function (f) { return f.name.toLowerCase().indexOf(needle) >= 0; }).forEach(function (file) {
        shown++;
        var row = el(browsing ? "a" : "div", "row file " + rowLook() + (browsing ? " cursor-pointer text-ink no-underline hover:bg-wash" : " text-subtle"));
        if (browsing) { row.href = viewer(file); } else { row.setAttribute("role", "option"); row.setAttribute("aria-disabled", "true"); }
        var name = el("span", nameLook()); name.appendChild(iconFor(file.name, browsing ? "size-5" : "size-4")); name.appendChild(el("span", "truncate", file.name, file.name));
        row.appendChild(name); row.appendChild(el("span", "text-xs whitespace-nowrap text-muted @max-[40rem]:hidden", when(file.modified))); row.appendChild(el("span", "text-right text-xs whitespace-nowrap text-muted", bytes(file.size)));
        rows.appendChild(row);
      });
      if (!shown && !state.making) {
        rows.appendChild(el("div", "px-4 py-12 text-center text-sm text-subtle", needle ? "Nothing here matches." : state.here.at ? "This folder is empty." : "No projects yet. Choose New Folder to start one."));
      }
      roving();
      var fresh = rows.querySelector(".making input");
      if (fresh && state.focusNew) { state.focusNew = false; fresh.focus(); }
      if (state.refocus) {
        state.refocus = false;
        var back = rows.querySelector('.folder[tabindex="0"], a.row') || container.querySelector("input, button:not(:disabled)");
        if (back && !container.contains(document.activeElement)) { back.focus(); }
      }
    }
    // In the window that chooses, the keyboard comes into the list on the folder chosen, or the first.
    function roving() {
      if (browsing) { return; }
      var folders = container.querySelectorAll("#d-rows .row.folder");
      var chosen = container.querySelector('#d-rows .row.folder[aria-selected="true"]') || folders[0];
      folders.forEach(function (row) { row.tabIndex = row === chosen ? 0 : -1; });
    }
    function mark(focus) {
      container.querySelectorAll("#d-rows .row.folder").forEach(function (row) {
        var chosen = row.dataset.at === state.selected;
        row.classList.toggle("selected", chosen);
        row.setAttribute("aria-selected", String(chosen));
        if (chosen && focus) { row.focus(); }
      });
      roving();
      var summary = container.querySelector(".d-summary");
      if (summary) { summary.textContent = summaryText(); }
      var go = container.querySelector(".d-choose");
      if (go) { go.disabled = !target(); }
    }

    function makingRow() {
      var row = el("div", "row making grid grid-cols-[minmax(0,1fr)] items-center gap-1 text-sm " + (browsing ? "min-h-12 px-5" : "min-h-9 px-4"));
      var name = el("span", nameLook()); name.appendChild(icon("folder", browsing ? "size-5" : "size-4", "text-muted [&_svg]:fill-subtle/35"));
      var input = el("input", "h-7 w-full rounded-xs border border-subtle bg-surface px-2 text-sm text-ink ring-3 ring-hover");
      input.type = "text"; input.maxLength = 80; input.placeholder = state.here.at ? "New folder" : "New project";
      input.setAttribute("aria-label", "Name of the new folder");
      if (state.problem) { input.setAttribute("aria-invalid", "true"); input.setAttribute("aria-describedby", "d-problem"); }
      input.addEventListener("keydown", function (event) {
        event.stopPropagation();
        if (event.key === "Enter") { make(input.value); }
        if (event.key === "Escape") { state.making = false; state.problem = ""; draw(); }
      });
      name.appendChild(input);
      row.appendChild(name);
      if (state.problem) { var problem = el("span", "mb-1 ml-8 text-xs text-bad", state.problem); problem.id = "d-problem"; row.appendChild(problem); }
      return row;
    }
    function make(name) {
      name = name.trim();
      if (!name) { return; }
      var harness = document.getElementById("harness-pick");
      var body = new URLSearchParams({ at: state.here.at, name: name, harness: harness && harness.picker ? harness.picker.value : "" });
      fetch("/chat/folders", { method: "POST", body: body, credentials: "same-origin" })
        .then(function (answer) { return answer.ok ? answer.json() : answer.text().then(function (why) { return Promise.reject(why.trim()); }); })
        .then(function (made) {
          get(state.here.at).then(function (shown) { state.here = shown; state.making = false; state.problem = ""; state.selected = made.at; draw(); places(); });
        })
        .catch(function (why) { state.problem = String(why || "That folder could not be made."); state.focusNew = true; draw(); });
    }

    function footer() {
      var foot = el("div", "grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-4 border-t border-line px-4 py-3 @max-[40rem]:grid-cols-[minmax(0,1fr)_auto]");
      var quiet = look.button + " h-[2.125rem] px-4 text-xs font-semibold";
      var left = el("div", "flex items-center gap-2");
      left.appendChild(button(quiet, function () { state.making = true; state.focusNew = true; state.problem = ""; draw(); }, [state.here.at ? "New Folder" : "New Project"]));
      (options.extras || []).forEach(function (extra) { left.appendChild(button(quiet, extra.run, [extra.label])); });
      foot.appendChild(left);
      foot.appendChild(el("div", "d-summary overflow-hidden text-right text-xs text-ellipsis whitespace-nowrap text-muted @max-[40rem]:hidden", summaryText()));
      var right = el("div", "flex items-center gap-2");
      if (browsing) {
        var chat = el("a", look.primary + " d-choose h-[2.125rem] px-4 text-xs font-semibold", "Chat here");
        chat.href = target() ? "/chat?at=" + encodeURIComponent(target()) : "/chat";
        if (!target()) { chat.setAttribute("aria-disabled", "true"); }
        right.appendChild(chat);
      } else {
        right.appendChild(button(quiet, options.onCancel, ["Cancel"]));
        var choose = button(look.primary + " d-choose h-[2.125rem] px-4 text-xs font-semibold", function () { options.onChoose(target()); }, ["Choose"]);
        choose.disabled = !target();
        right.appendChild(choose);
      }
      foot.appendChild(right);
      return foot;
    }

    // The keys of the list, given by the window it is in; a key on a field or a button is that control's own.
    this.keys = function (event) {
      if (!state.here || event.target.closest("input, button, a")) { return; }
      var folders = state.here.folders.map(function (f) { return f.at; });
      var i = folders.indexOf(state.selected);
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        i = event.key === "ArrowDown" ? Math.min(i + 1, folders.length - 1) : Math.max(i - 1, 0);
        state.selected = folders[i] || ""; mark(true);
      } else if (event.key === "Enter" && (event.metaKey || event.ctrlKey) && !browsing) { event.preventDefault(); options.onChoose(target()); }
      else if (event.key === "Enter" && state.selected) { event.preventDefault(); load(state.selected, true); }
      else if (event.key === "Backspace" && state.here.at) { event.preventDefault(); load(parent(), true); }
    };

    draw();
    load(options.start || "", false);
    places();
  };
})();
