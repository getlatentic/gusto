// The appearance panel: the theme and the accent, kept on this device and applied by the script in the page head before anything is drawn.
(function () {
  var panel = document.getElementById("look");
  var open = document.querySelector("[data-look]");
  if (!panel || !open || !window.haasLook) { return; }
  function show() {
    panel.querySelectorAll("[data-mode]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.dataset.mode === window.haasLook.theme())); });
    panel.querySelectorAll("[data-accent]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.dataset.accent === window.haasLook.accent())); });
  }
  open.addEventListener("click", function () { panel.hidden = !panel.hidden; show(); });
  panel.addEventListener("click", function (event) {
    var pick = event.target.closest("[data-mode], [data-accent]");
    if (!pick) { return; }
    if (pick.dataset.mode) { window.haasLook.setTheme(pick.dataset.mode); } else { window.haasLook.setAccent(pick.dataset.accent); }
    show();
  });
  document.addEventListener("click", function (event) { if (!panel.hidden && !event.target.closest("#look, [data-look]")) { panel.hidden = true; } });
  document.addEventListener("keydown", function (event) { if (event.key === "Escape") { panel.hidden = true; } });
})();
