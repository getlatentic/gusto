// A menu behind a "…" button is a <details data-menu>. It closes on a click outside it and on Escape, as a menu does.
(function () {
  function closeAll(except) {
    document.querySelectorAll("details[data-menu][open]").forEach(function (menu) {
      if (menu !== except) menu.removeAttribute("open");
    });
  }
  document.addEventListener("click", function (event) {
    closeAll(event.target.closest("details[data-menu]"));
  });
  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape") closeAll(null);
  });
})();
