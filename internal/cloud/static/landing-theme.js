// Sets the theme before first paint so the page does not flash.
(function () {
  var dark = window.matchMedia("(prefers-color-scheme: dark)");
  function apply() { document.documentElement.dataset.theme = dark.matches ? "dark" : "light"; }
  apply();
  dark.addEventListener("change", apply);
})();
