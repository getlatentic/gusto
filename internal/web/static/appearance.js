// The appearance panel: the theme and the accent, kept on this device and applied by the script in the page head before anything is drawn.
(function () {
  var panel = document.getElementById("look");
  var open = document.querySelector("[data-look]");
  if (!panel || !open || !window.gustoLook) { return; }
  function show() {
    var size = document.getElementById("look-size");
    if (size) { size.textContent = window.gustoLook.zoom() + "%"; }
    panel.querySelectorAll("[data-mode]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.dataset.mode === window.gustoLook.theme())); });
    panel.querySelectorAll("[data-accent]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.dataset.accent === window.gustoLook.accent())); });
  }
  function setOpen(shown) { panel.hidden = !shown; open.setAttribute("aria-expanded", String(shown)); }
  // Opening puts the keyboard on the current theme; leaving the panel by the keys or Escape closes it, and Escape gives the keyboard back.
  open.addEventListener("click", function () {
    var shown = panel.hidden;
    setOpen(shown);
    show();
    if (shown) { (panel.querySelector('[aria-pressed="true"]') || panel.querySelector("button")).focus(); }
  });
  panel.addEventListener("focusout", function (event) {
    var to = event.relatedTarget;
    if (to && !panel.contains(to) && to !== open) { setOpen(false); }
  });
  panel.addEventListener("click", function (event) {
    var pick = event.target.closest("[data-mode], [data-accent], [data-zoom]");
    if (!pick) { return; }
    if (pick.dataset.zoom) { window.gustoLook.stepZoom(+pick.dataset.zoom); } else if (pick.dataset.mode) { window.gustoLook.setTheme(pick.dataset.mode); } else { window.gustoLook.setAccent(pick.dataset.accent); }
    show();
  });
  document.addEventListener("click", function (event) { if (!panel.hidden && !event.target.closest("#look, [data-look]")) { setOpen(false); } });
  // The desktop app's web view has no zoom keys of its own on macOS, so the page answers them: the size of the text is the size of everything.
  document.addEventListener("keydown", function (event) {
    if (!window.latenticDesktop || !(event.metaKey || event.ctrlKey) || event.altKey) { return; }
    var by = event.key === "+" || event.key === "=" ? 1 : event.key === "-" || event.key === "_" ? -1 : event.key === "0" ? 0 : null;
    if (by === null) { return; }
    event.preventDefault();
    window.gustoLook.stepZoom(by);
    show();
  });
  document.addEventListener("keydown", function (event) {
    if (event.key !== "Escape" || panel.hidden) { return; }
    var inside = panel.contains(document.activeElement);
    setOpen(false);
    if (inside) { open.focus(); }
  });
})();
