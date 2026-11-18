// Warns, on any page, that the demo host is about to remove itself, and links to where it
// is extended. It asks the host how long is left, counts down between asks, and does
// nothing on a host that has no lease. On a demo organization the bar is always there: it
// counts down the demo, links to more time and to booking a call, and when it has ended takes the page to the one that
// says so.
(function () {
  var extend = document.currentScript && document.currentScript.getAttribute("data-extend");
  // The path the page's server is under, when a gusto cloud serves the organization at one: the host's lease is asked there.
  var base = (document.querySelector('meta[name="gusto-base"]') || {}).content || "";
  var warnAt = 300, every = 30000, left = null, bar = null, link = null, text = null, book = null, stop = null, invite = null, demo = false, strip = "2rem";

  // The bar is drawn on the app's pages and on gusto cloud's, whose stylesheets differ, so it styles itself from the tokens
  // both load.
  var action = "color:inherit;font-weight:600;text-decoration:underline;text-underline-offset:2px";

  function anchor(label, style) {
    var a = document.createElement("a");
    a.textContent = label;
    a.target = "_blank";
    a.rel = "noopener";
    a.style.cssText = style;
    return a;
  }

  function build() {
    bar = document.createElement("div");
    bar.setAttribute("role", "status");
    bar.dataset.testid = "lease-bar";
    bar.style.cssText = "position:fixed;inset:auto 0 0 0;z-index:1000;box-sizing:border-box;display:flex;height:" + strip + ";align-items:center;" +
      "justify-content:center;gap:0.75rem;overflow:hidden;white-space:nowrap;padding:0 1rem;background:var(--gu-text);color:var(--gu-text-inverse);" +
      "font:500 0.875rem/1 var(--gu-font-text)";
    text = document.createElement("span");
    link = anchor("Keep it", action);
    link.href = extend || "/lease";
    book = anchor("Book a call", action);
    book.hidden = true;
    stop = anchor("End demo", "color:inherit;opacity:0.8");
    stop.hidden = true;
    invite = anchor("Invite a colleague", action);
    invite.hidden = true;
    bar.append(text, link, invite, book, stop);
    document.body.appendChild(bar);
  }

  // The page is shorter by the strip, so nothing sits under it.
  function reserve(on) { document.documentElement.style.setProperty("--lease-h", on ? strip : "0rem"); }

  function draw() {
    if (left === null) { return; }
    if (!demo && left > warnAt) { if (bar) { bar.hidden = true; } reserve(false); return; }
    if (!bar) { build(); }
    bar.hidden = false;
    reserve(true);
    if (demo) {
      if (left <= 0) {
        text.textContent = "Your demo has ended.";
        link.hidden = true;
        stop.hidden = true;
        invite.hidden = true;
      } else {
        var dm = Math.floor(left / 60), ds = left % 60;
        text.textContent = "Demo ends in " + dm + ":" + (ds < 10 ? "0" : "") + ds + ".";
        link.textContent = "More time";
        link.hidden = left > warnAt;
        stop.hidden = !stop.href;
        invite.hidden = !invite.href;
      }
      book.hidden = !book.href;
      return;
    }
    if (left <= 0) {
      text.textContent = "This demo host is removing itself now.";
      link.hidden = true;
      return;
    }
    var m = Math.floor(left / 60), s = left % 60;
    text.textContent = "This demo host removes itself in " + m + ":" + (s < 10 ? "0" : "") + s + ".";
    link.hidden = false;
  }

  function ask() {
    fetch(base + "/.gusto/lease", { cache: "no-store", credentials: "omit" })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (d) {
        if (d && d.ended === true) { location.reload(); return; }
        if (d && typeof d.seconds === "number") {
          left = d.seconds; demo = d.demo === true;
          if (demo && !window.gustoDemo) {
            window.gustoDemo = { book: d.book || "" };
            document.dispatchEvent(new CustomEvent("gusto:demo", { detail: window.gustoDemo }));
          }
          if (demo && bar === null) { build(); }
          if (demo && d.extend) { link.href = d.extend; }
          if (demo && d.book && book) { book.href = d.book; }
          if (demo && d.end && stop) { stop.href = d.end; }
          if (demo && d.invite && invite) { invite.href = d.invite; }
          draw();
        } else { left = null; }
      })
      .catch(function () {});
  }

  ask();
  setInterval(ask, every);
  // At the end of a demo the host is asked at once, and answers that it has ended.
  setInterval(function () { if (left !== null && left > 0) { left -= 1; draw(); if (left === 0 && demo) { ask(); } } }, 1000);
})();
