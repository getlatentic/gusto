// The explorer for choosing the folder a chat works in. It shows the person's cloud computer: their agents' folders at the top, and
// inside one its folders and files. "Work here" chooses the folder shown, a new folder can be made in it, and a person in the desktop
// app can instead have a folder of their own computer kept the same as an agent's.
(function () {
  var root = document.getElementById("folder-pick");
  if (!root) { return; }
  var at = root.querySelector('input[name="at"]');
  var keyInput = root.querySelector('input[name="folder"]');
  var nameInput = root.querySelector('input[name="folder_name"]');
  var chip = root.querySelector(".chip");
  var label = chip.querySelector(".label");
  var clear = root.querySelector(".clear");
  var panel = root.querySelector(".explorer");
  var none = label.textContent;
  var here = null;

  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) { node.className = className; }
    if (text !== undefined) { node.textContent = text; }
    return node;
  }
  function button(text, className, onclick) {
    var node = el("button", className, text);
    node.type = "button";
    node.addEventListener("click", onclick);
    return node;
  }
  function harnessNow() {
    var picker = document.getElementById("harness-pick");
    return picker && picker.picker ? picker.picker.value : "";
  }

  function open() { panel.hidden = false; chip.setAttribute("aria-expanded", "true"); go(at.value || ""); }
  function close() { panel.hidden = true; chip.setAttribute("aria-expanded", "false"); }

  function go(place) {
    fetch("/chat/folders?at=" + encodeURIComponent(place), { credentials: "same-origin" })
      .then(function (answer) { return answer.ok ? answer.json() : Promise.reject(); })
      .then(function (shown) { here = shown; draw(shown); })
      .catch(function () { panel.textContent = "The folders could not be read."; });
  }

  function draw(shown) {
    panel.textContent = "";
    var trail = el("div", "trail");
    shown.crumbs.forEach(function (crumb, i) {
      if (i > 0) { trail.appendChild(el("span", "sep", "/")); }
      var last = i === shown.crumbs.length - 1;
      trail.appendChild(last ? el("span", "here", crumb.name) : button(crumb.name, "crumb", function () { go(crumb.at); }));
    });
    panel.appendChild(trail);

    var list = el("div", "entries");
    shown.folders.forEach(function (folder) {
      var row = button("", "folder-row", function () { go(folder.at); });
      row.appendChild(el("span", "name", folder.name));
      row.appendChild(el("span", "go", "›"));
      list.appendChild(row);
    });
    shown.files.forEach(function (file) { list.appendChild(el("div", "file-row", file)); });
    if (!shown.folders.length && !shown.files.length) {
      list.appendChild(el("div", "empty", shown.at ? "This folder is empty." : "No folders yet. Make one to start."));
    }
    panel.appendChild(list);

    var foot = el("div", "foot");
    foot.appendChild(button("New folder", "quiet", function () { makeFolder(shown); }));
    if (window.latenticDesktop) {
      foot.appendChild(button("Sync one from this computer…", "quiet", function () { close(); location.href = "/app/folder"; }));
    }
    var use = button("Work here", "use", function () { choose(shown); });
    use.disabled = !shown.at;
    foot.appendChild(use);
    panel.appendChild(foot);
  }

  function makeFolder(shown) {
    var name = window.prompt(shown.at ? "Name of the new folder" : "Name of the new folder, which is a new place for an agent to work");
    if (!name) { return; }
    var body = new URLSearchParams({ at: shown.at, name: name.trim(), harness: harnessNow() });
    fetch("/chat/folders", { method: "POST", body: body, credentials: "same-origin" })
      .then(function (answer) { return answer.ok ? answer.json() : answer.text().then(function (why) { return Promise.reject(why); }); })
      .then(function (made) { go(made.at); })
      .catch(function (why) { window.alert(String(why || "That folder could not be made.")); });
  }

  function choose(shown) {
    at.value = shown.at;
    keyInput.value = nameInput.value = "";
    label.textContent = shown.crumbs[shown.crumbs.length - 1].name;
    clear.hidden = false;
    close();
    root.dispatchEvent(new CustomEvent("chosen", { detail: { harness: shown.harness } }));
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

  chip.addEventListener("click", function () { panel.hidden ? open() : close(); });
  clear.addEventListener("click", release);
  document.addEventListener("click", function (event) { if (!root.contains(event.target)) { close(); } });
  root.addEventListener("keydown", function (event) { if (event.key === "Escape") { close(); chip.focus(); } });
})();
