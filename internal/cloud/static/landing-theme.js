// The page follows the visitor's light or dark setting, and the setting is read before the page is drawn, so it does not flash.
(function () {
  var dark = window.matchMedia("(prefers-color-scheme: dark)");
  function apply() { document.documentElement.dataset.theme = dark.matches ? "dark" : "light"; }
  apply();
  dark.addEventListener("change", apply);
})();
