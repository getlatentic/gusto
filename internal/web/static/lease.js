// Warns, on any page, that the demo host is about to remove itself, and links to where it
// is extended. It asks the host how long is left, counts down between asks, and does
// nothing on a host that has no lease. On a demo organization the bar is always there: it
// counts down the demo, links to more time and to booking a call, and says when it has ended.
(function () {
  var extend = document.currentScript && document.currentScript.getAttribute("data-extend");
  var warnAt = 300, every = 30000, left = null, bar = null, link = null, text = null, book = null, stop = null, invite = null, demo = false, strip = "2rem";

  var action = "font-semibold underline underline-offset-2";

  function anchor(label, classes) {
    var a = document.createElement("a");
    a.textContent = label;
    a.target = "_blank";
    a.rel = "noopener";
    a.className = classes;
    return a;
  }

  function build() {
    bar = document.createElement("div");
    bar.setAttribute("role", "status");
    bar.className = "fixed inset-x-0 bottom-0 z-[1000] box-border flex h-8 items-center justify-center gap-3 overflow-hidden whitespace-nowrap bg-ink px-4 text-sm font-medium text-inverse";
    text = document.createElement("span");
    link = anchor("Keep it", action);
    link.href = extend || "/lease";
    book = anchor("Book a call", action);
    book.hidden = true;
    stop = anchor("End demo", "opacity-70 hover:opacity-100");
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
    fetch("/.gusto/lease", { cache: "no-store", credentials: "omit" })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (d) {
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
  setInterval(function () { if (left !== null && left > 0) { left -= 1; draw(); } }, 1000);
})();
