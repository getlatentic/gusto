// The theme, the accent and the text size are the person's, kept on this device and set before the page is drawn so that it does not
// flash. It is loaded in the head of every page, and the account menu changes them through
// window.gustoLook.
(function () {
  var root = document.documentElement, theme = "system", accent = "green", zoom = 100, dark = matchMedia("(prefers-color-scheme: dark)");
  var steps = [80, 90, 100, 110, 125, 150, 175, 200];
  try { theme = localStorage.getItem("gusto-theme") || theme; accent = localStorage.getItem("gusto-accent") || accent; zoom = +localStorage.getItem("gusto-zoom") || zoom; } catch (e) {}
  function apply() { root.dataset.theme = theme === "dark" || (theme === "system" && dark.matches) ? "dark" : "light"; root.dataset.accent = accent; root.style.fontSize = zoom + "%"; }
  function keep(key, value) { try { localStorage.setItem(key, value); } catch (e) {} }
  dark.addEventListener("change", apply);
  apply();
  window.gustoLook = {
    theme: function () { return theme; }, accent: function () { return accent; }, zoom: function () { return zoom; },
    stepZoom: function (by) {
      var at = steps.indexOf(zoom); if (at < 0) { at = 2; }
      zoom = by === 0 ? 100 : steps[Math.max(0, Math.min(steps.length - 1, at + by))];
      keep("gusto-zoom", String(zoom)); apply();
    },
    setTheme: function (value) { theme = value; keep("gusto-theme", value); apply(); },
    setAccent: function (value) { accent = value; keep("gusto-accent", value); apply(); }
  };
})();
