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
  function icon(name) { var span = document.createElement("span"); span.className = "ic"; span.setAttribute("aria-hidden", "true"); span.innerHTML = icons[name]; return span; }
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
  function iconFor(name) {
    for (var kind in kinds) { if (kinds[kind].test(name)) { return icon(kind); } }
    return icon("file");
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
      if (!state.here) { container.appendChild(el("div", "d-loading", state.problem || "Looking…")); return; }
      if (browsing) {
        container.appendChild(pageHead());
        container.appendChild(list());
      } else {
        container.appendChild(toolbar());
        var body = el("div", "d-body");
        body.appendChild(placesList());
        body.appendChild(list());
        container.appendChild(body);
        container.appendChild(footer());
      }
    }

    function toolbar() {
      var bar = el("div", "d-bar");
      var back = button("d-tool", function () { load(state.back.pop() || "", false); }, [icon("back")], "Back");
      back.disabled = !state.back.length;
      var up = button("d-tool", function () { load(parent(), true); }, [icon("up")], "Up one folder");
      up.disabled = !state.here.at;
      bar.appendChild(back);
      bar.appendChild(up);
      var path = el("div", "d-path");
      state.here.crumbs.forEach(function (crumb, i) {
        if (i > 0) { path.appendChild(el("span", "sep", "›")); }
        var last = i === state.here.crumbs.length - 1;
        path.appendChild(last ? el("span", "here", crumb.name, crumb.name) : button("crumb", function () { load(crumb.at, true); }, [crumb.name]));
      });
      bar.appendChild(path);
      var filter = el("input", "d-filter");
      filter.type = "search"; filter.placeholder = "Filter"; filter.value = state.filter; filter.setAttribute("aria-label", "Filter this folder");
      filter.addEventListener("input", function () { state.filter = filter.value; drawList(); });
      bar.appendChild(filter);
      return bar;
    }

    // The Files page has no window around it: the way down is the heading itself, and what can be done here sits to its right.
    function pageHead() {
      var head = el("div", "f-head");
      var trail = el("h1", "f-trail");
      state.here.crumbs.forEach(function (crumb, i) {
        var name = i === 0 ? "Files" : crumb.name;
        if (i > 0) { trail.appendChild(el("span", "sep", "/")); }
        trail.appendChild(i === state.here.crumbs.length - 1 ? el("span", "here", name, name) : button("crumb", function () { load(crumb.at, true); }, [name]));
      });
      head.appendChild(trail);
      var tools = el("div", "f-tools");
      var filter = el("input", "d-filter");
      filter.type = "search"; filter.placeholder = "Filter"; filter.value = state.filter; filter.setAttribute("aria-label", "Filter this folder");
      filter.addEventListener("input", function () { state.filter = filter.value; drawList(); });
      tools.appendChild(filter);
      tools.appendChild(button("d-quiet", function () { state.making = true; state.focusNew = true; state.problem = ""; draw(); }, [icon("plus"), state.here.at ? "New folder" : "New project"]));
      if (state.here.at) {
        var chat = el("a", "d-choose", "Chat here");
        chat.href = "/chat?at=" + encodeURIComponent(state.here.at);
        tools.appendChild(chat);
      }
      head.appendChild(tools);
      return head;
    }

    function placesList() {
      var side = el("nav", "d-places");
      side.setAttribute("aria-label", "Projects");
      var top = button("place root" + (!state.here.at ? " current" : ""), function () { load("", true); }, [icon("cloud"), el("span", "t", "Your cloud computer")]);
      if (!state.here.at) { top.setAttribute("aria-current", "true"); }
      side.appendChild(top);
      state.top.slice(0, 14).forEach(function (project) {
        var current = state.here.at === project.at || state.here.at.indexOf(project.at + "/") === 0;
        var place = button("place child" + (current ? " current" : ""), function () { load(project.at, true); }, [icon("folder"), el("span", "t", project.name)], project.name);
        if (current) { place.setAttribute("aria-current", "true"); }
        side.appendChild(place);
      });
      return side;
    }

    function list() {
      var box = el("div", "d-list");
      var head = el("div", "row heads");
      head.appendChild(el("span", "", "Name")); head.appendChild(el("span", "", "Modified")); head.appendChild(el("span", "num", "Size"));
      box.appendChild(head);
      var rows = el("div", "rows");
      rows.id = "d-rows";
      if (!browsing) { rows.setAttribute("role", "listbox"); rows.setAttribute("aria-label", "Folders"); }
      box.appendChild(rows);
      setTimeout(drawList, 0);
      return box;
    }
    function drawList() {
      var rows = container.querySelector("#d-rows");
      if (!rows) { return; }
      rows.textContent = "";
      var needle = state.filter.trim().toLowerCase();
      if (state.making) { rows.appendChild(makingRow()); }
      var shown = 0;
      state.here.folders.filter(function (f) { return f.name.toLowerCase().indexOf(needle) >= 0; }).forEach(function (folder) {
        shown++;
        var row = el("div", "row folder" + (state.selected === folder.at ? " selected" : ""));
        row.dataset.at = folder.at;
        row.tabIndex = -1;
        row.setAttribute("role", browsing ? "link" : "option");
        if (browsing) { row.tabIndex = 0; } else { row.setAttribute("aria-selected", String(state.selected === folder.at)); }
        row.addEventListener("keydown", function (event) { if (browsing && event.key === "Enter") { load(folder.at, true); } });
        var name = el("span", "name"); name.appendChild(icon("folder")); name.appendChild(el("span", "t", folder.name, folder.name));
        row.appendChild(name); row.appendChild(el("span", "dim", when(folder.modified))); row.appendChild(el("span", "go"));
        row.lastChild.appendChild(icon("go"));
        row.addEventListener("click", function () { if (browsing) { load(folder.at, true); } else { state.selected = folder.at; mark(); } });
        row.addEventListener("dblclick", function () { load(folder.at, true); });
        rows.appendChild(row);
      });
      state.here.files.filter(function (f) { return f.name.toLowerCase().indexOf(needle) >= 0; }).forEach(function (file) {
        shown++;
        var row = el(browsing ? "a" : "div", "row file" + (browsing ? " openable" : ""));
        if (browsing) { row.href = viewer(file); } else { row.setAttribute("role", "option"); row.setAttribute("aria-disabled", "true"); }
        var name = el("span", "name"); name.appendChild(iconFor(file.name)); name.appendChild(el("span", "t", file.name, file.name));
        row.appendChild(name); row.appendChild(el("span", "dim", when(file.modified))); row.appendChild(el("span", "dim num", bytes(file.size)));
        rows.appendChild(row);
      });
      if (!shown && !state.making) {
        rows.appendChild(el("div", "d-empty", needle ? "Nothing here matches." : state.here.at ? "This folder is empty." : "No projects yet. Choose New Folder to start one."));
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
      var row = el("div", "row making");
      var name = el("span", "name"); name.appendChild(icon("folder"));
      var input = el("input", "d-new");
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
      if (state.problem) { var problem = el("span", "problem", state.problem); problem.id = "d-problem"; row.appendChild(problem); }
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
      var foot = el("div", "d-foot");
      var left = el("div", "d-left");
      left.appendChild(button("d-quiet", function () { state.making = true; state.focusNew = true; state.problem = ""; draw(); }, [state.here.at ? "New Folder" : "New Project"]));
      (options.extras || []).forEach(function (extra) { left.appendChild(button("d-quiet", extra.run, [extra.label])); });
      foot.appendChild(left);
      foot.appendChild(el("div", "d-summary", summaryText()));
      var right = el("div", "d-right");
      if (browsing) {
        var chat = el("a", "d-choose", "Chat here");
        chat.href = target() ? "/chat?at=" + encodeURIComponent(target()) : "/chat";
        if (!target()) { chat.setAttribute("aria-disabled", "true"); }
        right.appendChild(chat);
      } else {
        right.appendChild(button("d-cancel", options.onCancel, ["Cancel"]));
        var choose = button("d-choose", function () { options.onChoose(target()); }, ["Choose"]);
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
