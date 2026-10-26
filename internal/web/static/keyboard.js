// Safari on iOS leaves the layout viewport as it is when the on-screen keyboard opens and pans the page up, which takes the header off the
// screen. The page is made as tall as the visible part instead. Chrome on Android shrinks the page itself (interactive-widget=resizes-content),
// so there the page is never taller than the visible part and nothing here changes.
(function () {
  var view = window.visualViewport;
  if (!view || !window.matchMedia("(pointer: coarse)").matches) { return; }

  // A page this much taller than the visible part is covered by a keyboard; the browser bar moving is smaller.
  var KEYBOARD_MIN = 80;

  var fit = function () {
    var body = document.body;
    body.style.height = "";
    if (view.scale > 1) { return; }
    if (body.offsetHeight - view.height > KEYBOARD_MIN) {
      body.style.height = "calc(" + view.height + "px - var(--lease-h, 0rem))";
    }
    if (window.scrollY > 0 || view.offsetTop > 0) { window.scrollTo(0, 0); }
  };

  view.addEventListener("resize", fit);
  view.addEventListener("scroll", fit);
  window.addEventListener("resize", fit);
})();
