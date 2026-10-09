// The window for choosing the folder a chat works in: the explorer, in a dialog, with a way to choose. In the desktop app it also offers a
// folder of the person's own computer, which the app asks the system for and tells the page about.
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
  var backdrop, explorer;

  function open() {
    backdrop = document.createElement("div");
    backdrop.className = "dialog-backdrop";
    var dialog = document.createElement("div");
    dialog.className = "dialog";
    dialog.setAttribute("role", "dialog");
    dialog.setAttribute("aria-modal", "true");
    dialog.setAttribute("aria-label", "Choose a folder");
    var head = document.createElement("div");
    head.className = "d-head";
    var title = document.createElement("h2");
    title.textContent = "Choose a folder";
    var shut = document.createElement("button");
    shut.type = "button"; shut.className = "d-close"; shut.setAttribute("aria-label", "Close");
    shut.innerHTML = '<span class="ic"><svg viewBox="0 0 16 16"><path d="m4.5 4.5 7 7M11.5 4.5l-7 7"/></svg></span>';
    shut.addEventListener("click", close);
    head.appendChild(title); head.appendChild(shut);
    var body = document.createElement("div");
    body.className = "ex";
    dialog.appendChild(head); dialog.appendChild(body);
    backdrop.appendChild(dialog);
    backdrop.addEventListener("mousedown", function (event) { if (event.target === backdrop) { close(); } });
    document.body.appendChild(backdrop);
    chip.setAttribute("aria-expanded", "true");
    var extras = window.latenticDesktop ? [{ label: "Sync from this computer…", run: function () { close(); location.href = "/app/folder"; } }] : [];
    explorer = new window.Explorer(body, { mode: "choose", start: at.value || "", extras: extras, onCancel: close, onChoose: pick });
    document.addEventListener("keydown", keys);
  }
  function close() {
    document.removeEventListener("keydown", keys);
    if (backdrop) { backdrop.remove(); }
    backdrop = null;
    chip.setAttribute("aria-expanded", "false");
    chip.focus();
  }
  function keys(event) {
    if (event.key === "Escape") { event.preventDefault(); close(); return; }
    explorer.keys(event);
  }

  function pick(place) {
    if (!place) { return; }
    fetch("/chat/folders?at=" + encodeURIComponent(place), { credentials: "same-origin" })
      .then(function (answer) { return answer.json(); })
      .then(function (shown) {
        at.value = shown.at;
        keyInput.value = nameInput.value = "";
        label.textContent = shown.crumbs[shown.crumbs.length - 1].name;
        clear.hidden = false;
        close();
        root.dispatchEvent(new CustomEvent("chosen", { detail: { harness: shown.harness } }));
      });
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
