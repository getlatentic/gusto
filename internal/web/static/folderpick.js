// The window for choosing the folder a chat works in, laid out as a Finder's open panel is: the projects down the side, the path along
// the top, and the folders and files of the place in a list with their dates and sizes. It shows the person's cloud computer, which is
// where the agent works. A folder is chosen with Choose, made with New Folder, and in the desktop app one of the person's own computer
// can be kept the same as an agent's instead.
(function () {
  var root = document.getElementById("folder-pick");
  if (!root) { return; }
  var at = root.querySelector('input[name="at"]');
  var keyInput = root.querySelector('input[name="folder"]');
  var nameInput = root.querySelector('input[name="folder_name"]');
  var chip = root.querySelector(".chip");
  var label = chip.querySelector(".label");
  var clear = root.querySelector(".clear");
  var none = label.textContent;

  var icons = {
    folder: '<svg viewBox="0 0 16 16"><path d="M2.5 4.5a1 1 0 0 1 1-1h2.9l1.2 1.5h5a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1h-9.1a1 1 0 0 1-1-1z"/></svg>',
    file: '<svg viewBox="0 0 16 16"><path d="M4 2.5h5l3 3v8a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-10a1 1 0 0 1 1-1z"/><path d="M9 2.5v3h3"/></svg>',
    back: '<svg viewBox="0 0 16 16"><path d="M9.5 3.5 5 8l4.5 4.5"/></svg>',
    up: '<svg viewBox="0 0 16 16"><path d="M3.5 9.5 8 5l4.5 4.5"/></svg>',
    close: '<svg viewBox="0 0 16 16"><path d="m4.5 4.5 7 7M11.5 4.5l-7 7"/></svg>',
    go: '<svg viewBox="0 0 16 16"><path d="m6.5 3.5 4.5 4.5-4.5 4.5"/></svg>'
  };
  function icon(name) { var span = document.createElement("span"); span.className = "ic"; span.innerHTML = icons[name]; return span; }
  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) { node.className = className; }
    if (text !== undefined) { node.textContent = text; }
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
  function when(seconds) {
    if (!seconds) { return ""; }
    var then = new Date(seconds * 1000), now = new Date();
    if (then.toDateString() === now.toDateString()) { return "Today " + then.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }); }
    return then.toLocaleDateString([], { day: "numeric", month: "short", year: then.getFullYear() === now.getFullYear() ? undefined : "numeric" });
  }
  function bytes(n) {
    if (n < 1024) { return n + " B"; }
    if (n < 1048576) { return Math.round(n / 1024) + " KB"; }
    return (n / 1048576).toFixed(1) + " MB";
  }

  var dialog, state;

  function open() {
    state = { here: null, selected: "", back: [], top: [], filter: "", making: false, problem: "" };
    var backdrop = el("div", "dialog-backdrop");
    dialog = el("div", "dialog");
    dialog.setAttribute("role", "dialog");
    dialog.setAttribute("aria-modal", "true");
    dialog.setAttribute("aria-label", "Choose a folder");
    backdrop.appendChild(dialog);
    backdrop.addEventListener("mousedown", function (event) { if (event.target === backdrop) { close(); } });
    document.body.appendChild(backdrop);
    dialog.backdrop = backdrop;
    document.addEventListener("keydown", keys);
    chip.setAttribute("aria-expanded", "true");
    load(at.value || "", false);
    fetchPlaces();
  }
  function close() {
    document.removeEventListener("keydown", keys);
    if (dialog && dialog.backdrop) { dialog.backdrop.remove(); }
    dialog = null;
    chip.setAttribute("aria-expanded", "false");
    chip.focus();
  }

  function fetchPlaces() {
    get("").then(function (top) { state.top = top.folders; if (dialog) { draw(); } });
  }
  function get(place) {
    return fetch("/chat/folders?at=" + encodeURIComponent(place), { credentials: "same-origin" })
      .then(function (answer) { return answer.ok ? answer.json() : Promise.reject(); });
  }
  function load(place, remember) {
    get(place).then(function (shown) {
      if (remember && state.here) { state.back.push(state.here.at); }
      state.here = shown; state.selected = ""; state.filter = ""; state.making = false; state.problem = "";
      draw();
    }).catch(function () { state.problem = "The folders could not be read."; draw(); });
  }
  function parent() {
    var crumbs = state.here.crumbs;
    return crumbs.length > 1 ? crumbs[crumbs.length - 2].at : "";
  }

  function draw() {
    dialog.textContent = "";
    var head = el("div", "d-head");
    head.appendChild(el("h2", "", "Choose a folder"));
    head.appendChild(button("d-close", close, [icon("close")], "Close"));
    dialog.appendChild(head);
    if (!state.here) { dialog.appendChild(el("div", "d-loading", state.problem || "Looking…")); return; }
    dialog.appendChild(toolbar());
    var body = el("div", "d-body");
    body.appendChild(places());
    body.appendChild(list());
    dialog.appendChild(body);
    dialog.appendChild(footer());
    var inputBox = dialog.querySelector(".making input");
    if (inputBox) { inputBox.focus(); }
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
      path.appendChild(last ? el("span", "here", crumb.name) : button("crumb", function () { load(crumb.at, true); }, [crumb.name]));
    });
    bar.appendChild(path);
    var filter = el("input", "d-filter");
    filter.type = "search"; filter.placeholder = "Filter"; filter.value = state.filter; filter.setAttribute("aria-label", "Filter this folder");
    filter.addEventListener("input", function () { state.filter = filter.value; drawList(); });
    bar.appendChild(filter);
    return bar;
  }

  function places() {
    var side = el("nav", "d-places");
    side.setAttribute("aria-label", "Projects");
    side.appendChild(el("div", "d-label", "Projects"));
    side.appendChild(button("place" + (!state.here.at ? " current" : ""), function () { load("", true); }, [icon("folder"), el("span", "t", "Your cloud computer")]));
    state.top.slice(0, 14).forEach(function (project) {
      var current = state.here.at === project.at || state.here.at.indexOf(project.at + "/") === 0;
      side.appendChild(button("place" + (current ? " current" : ""), function () { load(project.at, true); }, [icon("folder"), el("span", "t", project.name)], project.name));
    });
    return side;
  }

  function list() {
    var box = el("div", "d-list");
    box.id = "d-list";
    box.tabIndex = 0;
    var head = el("div", "row heads");
    head.appendChild(el("span", "", "Name")); head.appendChild(el("span", "", "Modified")); head.appendChild(el("span", "num", "Size"));
    box.appendChild(head);
    var rows = el("div", "rows");
    rows.id = "d-rows";
    box.appendChild(rows);
    setTimeout(drawList, 0);
    return box;
  }
  function drawList() {
    var rows = document.getElementById("d-rows");
    if (!rows) { return; }
    rows.textContent = "";
    var needle = state.filter.trim().toLowerCase();
    if (state.making) { rows.appendChild(makingRow()); }
    var shown = 0;
    state.here.folders.filter(function (f) { return f.name.toLowerCase().indexOf(needle) >= 0; }).forEach(function (folder) {
      shown++;
      var row = el("div", "row folder" + (state.selected === folder.at ? " selected" : ""));
      row.tabIndex = -1;
      row.dataset.at = folder.at;
      var name = el("span", "name"); name.appendChild(icon("folder")); name.appendChild(el("span", "t", folder.name));
      row.appendChild(name); row.appendChild(el("span", "dim", when(folder.modified))); row.appendChild(el("span", "go", ""));
      row.lastChild.appendChild(icon("go"));
      row.addEventListener("click", function () { state.selected = folder.at; mark(); });
      row.addEventListener("dblclick", function () { load(folder.at, true); });
      rows.appendChild(row);
    });
    state.here.files.filter(function (f) { return f.name.toLowerCase().indexOf(needle) >= 0; }).forEach(function (file) {
      shown++;
      var row = el("div", "row file");
      var name = el("span", "name"); name.appendChild(icon("file")); name.appendChild(el("span", "t", file.name));
      row.appendChild(name); row.appendChild(el("span", "dim", when(file.modified))); row.appendChild(el("span", "dim num", bytes(file.size)));
      rows.appendChild(row);
    });
    if (!shown && !state.making) {
      rows.appendChild(el("div", "d-empty", needle ? "Nothing here matches." : state.here.at ? "This folder is empty." : "No projects yet. Choose New Folder to start one."));
    }
  }
  function mark() {
    document.querySelectorAll("#d-rows .row.folder").forEach(function (row) { row.classList.toggle("selected", row.dataset.at === state.selected); });
    var summary = dialog.querySelector(".d-summary");
    if (summary) { summary.textContent = summaryText(); }
    dialog.querySelector(".d-choose").disabled = !target();
  }
  function target() { return state.selected || state.here.at; }
  function summaryText() {
    var place = target();
    if (!place) { return "Open a project, or make one"; }
    var chosen = state.here.folders.filter(function (f) { return f.at === place; })[0];
    return "Work in  " + (chosen ? chosen.name : state.here.crumbs[state.here.crumbs.length - 1].name);
  }

  function makingRow() {
    var row = el("div", "row making");
    var name = el("span", "name"); name.appendChild(icon("folder"));
    var input = el("input", "d-new");
    input.type = "text"; input.maxLength = 80; input.placeholder = state.here.at ? "New folder" : "New project";
    input.setAttribute("aria-label", "Name of the new folder");
    input.addEventListener("keydown", function (event) {
      event.stopPropagation();
      if (event.key === "Enter") { make(input.value); }
      if (event.key === "Escape") { state.making = false; state.problem = ""; draw(); }
    });
    name.appendChild(input);
    row.appendChild(name);
    if (state.problem) { row.appendChild(el("span", "problem", state.problem)); }
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
        get(state.here.at).then(function (shown) { state.here = shown; state.making = false; state.problem = ""; state.selected = made.at; draw(); fetchPlaces(); });
      })
      .catch(function (why) { state.problem = String(why || "That folder could not be made."); draw(); });
  }

  function footer() {
    var foot = el("div", "d-foot");
    var left = el("div", "d-left");
    left.appendChild(button("d-quiet", function () { state.making = true; state.problem = ""; draw(); }, [state.here.at ? "New Folder" : "New Project"]));
    if (window.latenticDesktop) {
      left.appendChild(button("d-quiet", function () { close(); location.href = "/app/folder"; }, ["Sync from this computer…"]));
    }
    foot.appendChild(left);
    foot.appendChild(el("div", "d-summary", summaryText()));
    var right = el("div", "d-right");
    right.appendChild(button("d-cancel", close, ["Cancel"]));
    var choose = button("d-choose", function () { pick(); }, ["Choose"]);
    choose.disabled = !target();
    right.appendChild(choose);
    foot.appendChild(right);
    return foot;
  }

  function pick() {
    var place = target();
    if (!place) { return; }
    get(place).then(function (shown) {
      at.value = shown.at;
      keyInput.value = nameInput.value = "";
      label.textContent = shown.crumbs[shown.crumbs.length - 1].name;
      clear.hidden = false;
      close();
      root.dispatchEvent(new CustomEvent("chosen", { detail: { harness: shown.harness } }));
    });
  }

  function keys(event) {
    if (event.key === "Escape") { event.preventDefault(); close(); return; }
    if (!state.here || event.target.matches("input")) { return; }
    var folders = state.here.folders.map(function (f) { return f.at; });
    var i = folders.indexOf(state.selected);
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      i = event.key === "ArrowDown" ? Math.min(i + 1, folders.length - 1) : Math.max(i - 1, 0);
      state.selected = folders[i] || ""; mark();
    } else if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) { event.preventDefault(); pick(); }
    else if (event.key === "Enter" && state.selected) { event.preventDefault(); load(state.selected, true); }
    else if (event.key === "Backspace" && state.here.at) { event.preventDefault(); load(parent(), true); }
  }

  function release() {
    at.value = keyInput.value = nameInput.value = "";
    label.textContent = none;
    clear.hidden = true;
    root.dispatchEvent(new CustomEvent("chosen", { detail: { harness: "" } }));
  }

  // A folder of the person's own computer, chosen by the desktop app, which tells the page which.
  window.latenticFolder = function (picked) {
    at.value = "";
    keyInput.value = picked.key;
    nameInput.value = picked.name;
    label.textContent = picked.name + " (this computer)";
    clear.hidden = false;
    root.dispatchEvent(new CustomEvent("chosen", { detail: { harness: "" } }));
  };

  // A chat opened from a project in the side list starts in that project's folder.
  if (root.dataset.at) {
    at.value = root.dataset.at;
    label.textContent = root.dataset.label;
    clear.hidden = false;
    // Told once the page has finished loading, when whatever follows the agent has had its say.
    var told = function () { root.dispatchEvent(new CustomEvent("chosen", { detail: { harness: root.dataset.harness } })); };
    if (document.readyState === "loading") { document.addEventListener("DOMContentLoaded", told); } else { told(); }
  }

  chip.addEventListener("click", open);
  clear.addEventListener("click", release);
})();
