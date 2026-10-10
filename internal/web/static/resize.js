// A divider that can be dragged, or moved by the arrow keys, to change the width of what is beside it. The width is kept on this device and
// is a custom property of the page, so everything that reads it follows. Double-clicking puts it back.
(function () {
  var root = document.documentElement;
  var rem = function () { return parseFloat(getComputedStyle(root).fontSize) || 16; };

  window.haasResize = function (handle, o) {
    if (!handle) { return; }
    var read = function () {
      var held = parseFloat(root.style.getPropertyValue(o.name));
      return isFinite(held) ? held : o.def;
    };
    var apply = function (width, keep) {
      var limit = Math.min(o.max, (o.share() * window.innerWidth) / rem());
      width = Math.max(o.min, Math.min(limit, width));
      root.style.setProperty(o.name, width + "rem");
      handle.setAttribute("aria-valuenow", String(Math.round(width)));
      if (keep) { try { localStorage.setItem(o.key, String(width)); } catch (e) {} }
      return width;
    };
    handle.setAttribute("aria-valuemin", String(o.min));
    handle.setAttribute("aria-valuemax", String(o.max));
    apply(read(), false);

    var from = null;
    handle.addEventListener("pointerdown", function (event) {
      from = { x: event.clientX, width: read() };
      handle.setPointerCapture(event.pointerId);
      root.classList.add("resizing");
      event.preventDefault();
    });
    handle.addEventListener("pointermove", function (event) {
      if (!from) { return; }
      var moved = (event.clientX - from.x) / rem();
      apply(from.width + (o.edge === "right" ? moved : -moved), false);
    });
    var end = function () {
      if (!from) { return; }
      from = null;
      root.classList.remove("resizing");
      apply(read(), true);
    };
    handle.addEventListener("pointerup", end);
    handle.addEventListener("pointercancel", end);
    handle.addEventListener("keydown", function (event) {
      var step = event.key === "ArrowRight" ? 1 : event.key === "ArrowLeft" ? -1 : 0;
      if (!step) { return; }
      event.preventDefault();
      apply(read() + (o.edge === "right" ? step : -step), true);
    });
    handle.addEventListener("dblclick", function () { apply(o.def, true); });
    window.addEventListener("resize", function () { apply(read(), false); });
  };

  window.haasResize(document.getElementById("panel-resize"), { name: "--panel-w", key: "haas-panel-w", min: 14, max: 28, def: 16, edge: "right", share: function () { return 0.4; } });
})();
