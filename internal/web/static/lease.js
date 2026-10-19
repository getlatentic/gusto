// Warns, on any page, that the demo host is about to remove itself, and links to where it
// is extended. It asks the host how long is left, counts down between asks, and does
// nothing on a host that has no lease. On a demo organisation the bar is always there: it
// counts down the demo, links to more time and to booking a call, and says when it has ended.
(function () {
  var extend = document.currentScript && document.currentScript.getAttribute("data-extend");
  var warnAt = 300, every = 30000, left = null, bar = null, link = null, text = null, book = null, stop = null, demo = false;

  function build() {
    bar = document.createElement("div");
    bar.setAttribute("role", "status");
    bar.style.cssText = "position:fixed;left:50%;bottom:1rem;transform:translateX(-50%);z-index:1000;" +
      "max-width:calc(100vw - 2rem);padding:0.625rem 1rem;border-radius:0.625rem;background:#0E1511;color:#F5FBF6;" +
      "font:500 0.875rem system-ui,sans-serif;box-shadow:0 0.25rem 1rem rgba(0,0,0,.25);display:flex;gap:0.75rem;align-items:center";
    text = document.createElement("span");
    link = document.createElement("a");
    link.textContent = "Keep it";
    link.href = extend || "/lease";
    link.target = "_blank";
    link.rel = "noopener";
    link.style.cssText = "color:#D8FF36;font-weight:600";
    book = document.createElement("a");
    book.textContent = "Book a call";
    book.target = "_blank";
    book.rel = "noopener";
    book.style.cssText = "color:#D8FF36;font-weight:600;display:none";
    stop = document.createElement("a");
    stop.textContent = "End demo";
    stop.target = "_blank";
    stop.rel = "noopener";
    stop.style.cssText = "color:#9FB3A5;display:none";
    bar.appendChild(text);
    bar.appendChild(link);
    bar.appendChild(book);
    bar.appendChild(stop);
    document.body.appendChild(bar);
  }

  function draw() {
    if (left === null) { return; }
    if (!demo && left > warnAt) { if (bar) { bar.style.display = "none"; } return; }
    if (!bar) { build(); }
    bar.style.display = "flex";
    if (demo) {
      if (left <= 0) {
        text.textContent = "Your demo has ended.";
        link.style.display = "none";
        stop.style.display = "none";
      } else {
        var dm = Math.floor(left / 60), ds = left % 60;
        text.textContent = "Demo ends in " + dm + ":" + (ds < 10 ? "0" : "") + ds + ".";
        link.textContent = "More time";
        link.style.display = left <= warnAt ? "" : "none";
        stop.style.display = stop.href ? "" : "none";
      }
      book.style.display = book.href ? "" : "none";
      return;
    }
    if (left <= 0) {
      text.textContent = "This demo host is removing itself now.";
      link.style.display = "none";
      return;
    }
    var m = Math.floor(left / 60), s = left % 60;
    text.textContent = "This demo host removes itself in " + m + ":" + (s < 10 ? "0" : "") + s + ".";
    link.style.display = "";
  }

  function ask() {
    fetch("/.gusto/lease", { cache: "no-store", credentials: "omit" })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (d) {
        if (d && typeof d.seconds === "number") {
          left = d.seconds; demo = d.demo === true;
          if (demo && bar === null) { build(); }
          if (demo && d.extend) { link.href = d.extend; }
          if (demo && d.book && book) { book.href = d.book; }
          if (demo && d.end && stop) { stop.href = d.end; }
          draw();
        } else { left = null; }
      })
      .catch(function () {});
  }

  ask();
  setInterval(ask, every);
  setInterval(function () { if (left !== null && left > 0) { left -= 1; draw(); } }, 1000);
})();
