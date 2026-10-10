// A file named in a chat opens as a tab in a panel beside it, so that reading it does not leave the conversation. Each file is a tab, a
// file already open is brought forward, and the panel goes away with its last tab. Where the window is too narrow to put two things side
// by side the link is followed as it always was.
document.addEventListener("DOMContentLoaded", function () {
  var panel = document.getElementById("file-panel");
  if (!panel) { return; }
  var tabs = panel.querySelector(".fp-tabs");
  var bodies = panel.querySelector(".fp-bodies");
  var open = {};

  window.haasResize(document.getElementById("file-panel-resize"), { name: "--files-w", key: "haas-files-w", min: 22, max: 60, def: 36, edge: "left", share: function () { return 0.7; } });

  var roomy = function () { return window.innerWidth / (parseFloat(getComputedStyle(document.documentElement).fontSize) || 16) >= 72; };

  function show(key) {
    Object.keys(open).forEach(function (each) {
      var shown = each === key;
      open[each].tab.setAttribute("aria-selected", String(shown));
      open[each].pane.hidden = !shown;
    });
  }

  function close(key) {
    var gone = open[key];
    if (!gone) { return; }
    gone.tab.remove();
    gone.pane.remove();
    delete open[key];
    var left = Object.keys(open);
    if (!left.length) { panel.hidden = true; return; }
    show(left[left.length - 1]);
  }

  function add(url, key) {
    var tab = document.createElement("div");
    tab.className = "fp-tab";
    tab.setAttribute("role", "tab");
    var name = document.createElement("button");
    name.type = "button";
    name.className = "nm";
    name.textContent = decodeURIComponent((url.searchParams.get("path") || url.pathname).split("/").pop()) || "File";
    var x = document.createElement("button");
    x.type = "button";
    x.className = "x";
    x.setAttribute("aria-label", "Close " + name.textContent);
    x.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg>';
    tab.appendChild(name);
    tab.appendChild(x);
    tabs.appendChild(tab);
    var pane = document.createElement("div");
    pane.className = "fp-pane";
    pane.setAttribute("role", "tabpanel");
    pane.textContent = "Opening…";
    bodies.appendChild(pane);
    open[key] = { tab: tab, pane: pane };
    name.addEventListener("click", function () { show(key); });
    x.addEventListener("click", function () { close(key); });
    var asked = new URL(url.href);
    asked.searchParams.set("panel", "1");
    asked.searchParams.delete("from");
    fetch(asked, { credentials: "same-origin" })
      .then(function (answer) { return answer.ok ? answer.text() : Promise.reject(); })
      .then(function (html) { pane.innerHTML = html; dressed(pane); })
      .catch(function () { pane.textContent = "This file could not be opened."; });
  }

  // Scripts that come with a tab's html are not run by being put on the page, so the ones it names are loaded in turn: code is coloured
  // and numbered by them.
  function dressed(pane) {
    var wanted = [].slice.call(pane.querySelectorAll("script[src]")).map(function (tag) { return tag.getAttribute("src"); });
    wanted.reduce(function (before, src) {
      return before.then(function () {
        if (document.querySelector('script[data-loaded="' + src + '"]') && !/code\.js/.test(src)) { return; }
        return new Promise(function (done) {
          var tag = document.createElement("script");
          tag.src = src;
          tag.dataset.loaded = src;
          tag.onload = tag.onerror = done;
          document.body.appendChild(tag);
        });
      });
    }, Promise.resolve());
  }

  function view(href) {
    var url = new URL(href, location.href);
    var key = url.pathname + "?" + (url.searchParams.get("path") || "");
    panel.hidden = false;
    if (!open[key]) { add(url, key); }
    show(key);
  }

  document.addEventListener("click", function (event) {
    var link = event.target.closest && event.target.closest('a[href*="/file?path="], a[href*="/files/"][href*="/jobs/"]');
    if (!link || link.hasAttribute("download") || link.hasAttribute("data-page") || event.defaultPrevented) { return; }
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) { return; }
    if (!link.closest(".said-so-far, .fp-bodies") || !roomy()) { return; }
    event.preventDefault();
    view(link.href);
  });
  document.getElementById("fp-close").addEventListener("click", function () {
    Object.keys(open).forEach(close);
    panel.hidden = true;
  });
});
