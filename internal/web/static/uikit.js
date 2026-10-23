// The UI kit's switches for the theme and the accent, which the page head's gustoLook keeps and applies.
(function () {
  var look = window.gustoLook;
  if (!look) { return; }
  var switches = document.querySelectorAll("[data-kit-theme], [data-kit-accent]");
  function show() {
    switches.forEach(function (b) {
      var on = b.dataset.kitTheme ? b.dataset.kitTheme === look.theme() : b.dataset.kitAccent === look.accent();
      b.setAttribute("aria-pressed", String(on));
    });
  }
  document.addEventListener("click", function (event) {
    var pick = event.target.closest("[data-kit-theme], [data-kit-accent]");
    if (!pick) { return; }
    if (pick.dataset.kitTheme) { look.setTheme(pick.dataset.kitTheme); } else { look.setAccent(pick.dataset.kitAccent); }
    show();
  });
  show();
})();
