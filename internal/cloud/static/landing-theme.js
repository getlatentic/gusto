// Sets the theme before first paint so the page does not flash, and answers the landing page's copy buttons.
(function () {
  var dark = window.matchMedia("(prefers-color-scheme: dark)");
  function apply() { document.documentElement.dataset.theme = dark.matches ? "dark" : "light"; }
  apply();
  dark.addEventListener("change", apply);

  if (!navigator.clipboard) return;
  document.documentElement.dataset.copy = "on";
  document.addEventListener("click", function (event) {
    var button = event.target.closest("[data-copy]");
    var source = button && document.getElementById(button.dataset.copy);
    if (!source) return;
    navigator.clipboard.writeText(source.textContent.trim()).then(function () {
      button.textContent = "Copied";
      setTimeout(function () { button.textContent = "Copy"; }, 2000);
    });
  });
})();
