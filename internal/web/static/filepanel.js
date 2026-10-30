// A file named in a chat opens as a tab in a panel beside it, so that reading it does not leave the conversation. Each file is a tab, a
// file already open is brought forward, and the panel goes away with its last tab. Where the window is too narrow to put two things side
// by side the link is followed as it always was.
document.addEventListener("DOMContentLoaded", function () {
  var panel = document.getElementById("file-panel");
  if (!panel) { return; }
  var tabs = panel.querySelector(".fp-tabs");
  var bodies = panel.querySelector(".fp-bodies");
  var open = {};
  var made = 0;
  var opener = null;

  window.gustoResize(document.getElementById("file-panel-resize"), { name: "--files-w", key: "gusto-files-w", min: 22, max: 60, def: 36, edge: "left", share: function () { return 0.7; } });

  var roomy = function () { return window.innerWidth / (parseFloat(getComputedStyle(document.documentElement).fontSize) || 16) >= 72; };

  function show(key, focus) {
    Object.keys(open).forEach(function (each) {
      var shown = each === key;
      open[each].name.setAttribute("aria-selected", String(shown));
      open[each].name.tabIndex = shown ? 0 : -1;
      open[each].pane.hidden = !shown;
    });
    if (focus && open[key]) { open[key].name.focus(); }
  }

  // Closing the tab the keyboard is on moves it to the tab shown next, or back to the link that opened the panel when none is left.
  function close(key) {
    var gone = open[key];
    if (!gone) { return; }
    var held = panel.contains(document.activeElement);
    gone.tab.remove();
    gone.pane.remove();
    delete open[key];
    var left = Object.keys(open);
    if (!left.length) {
      panel.hidden = true;
      if (held && opener) { opener.focus(); }
      return;
    }
    show(left[left.length - 1], held);
  }

  function add(url, key, tabName) {
    var id = "fp-" + (++made);
    var tab = document.createElement("div");
    tab.className = "inline-flex h-8 max-w-56 flex-none items-center rounded-sm pl-3 text-muted has-[[aria-selected=true]]:bg-wash has-[[aria-selected=true]]:text-ink";
    tab.setAttribute("role", "presentation");
    var name = document.createElement("button");
    name.type = "button";
    name.className = "min-w-0 cursor-pointer truncate border-0 bg-transparent p-0 text-xs font-medium text-inherit";
    name.id = id + "-tab";
    name.setAttribute("role", "tab");
    name.setAttribute("aria-controls", id + "-pane");
    name.textContent = tabName || decodeURIComponent((url.searchParams.get("path") || url.pathname).split("/").pop()) || "File";
    name.title = name.textContent;
    var x = document.createElement("button");
    x.type = "button";
    x.className = "mx-1 grid size-6 flex-none cursor-pointer place-items-center rounded-xs border-0 bg-transparent p-0 text-inherit hover:bg-hover";
    x.setAttribute("aria-label", "Close " + name.textContent);
    x.innerHTML = '<svg class="size-3 fill-none stroke-current stroke-2 [stroke-linecap:round]" viewBox="0 0 24 24" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg>';
    tab.appendChild(name);
    tab.appendChild(x);
    tabs.appendChild(tab);
    var pane = document.createElement("div");
    pane.className = "px-6 py-5 focus-visible:-outline-offset-2";
    pane.id = id + "-pane";
    pane.tabIndex = 0;
    pane.setAttribute("role", "tabpanel");
    pane.setAttribute("aria-labeledby", name.id);
    pane.textContent = "Opening…";
    bodies.appendChild(pane);
    var asked = new URL(url.href);
    asked.searchParams.set("panel", "1");
    asked.searchParams.delete("from");
    open[key] = { tab: tab, name: name, pane: pane, asked: asked };
    name.addEventListener("click", function () { show(key); });
    x.addEventListener("click", function () { close(key); });
    load(open[key]);
  }

  function load(file) {
    fetch(file.asked, { credentials: "same-origin", cache: "no-store" })
      .then(function (answer) { return answer.ok ? answer.text() : Promise.reject(); })
      .then(function (html) {
        var scrolled = bodies.scrollTop;
        file.pane.innerHTML = html;
        dressed(file.pane);
        if (!file.pane.hidden) { bodies.scrollTop = scrolled; }
      })
      .catch(function () { file.pane.textContent = "This file could not be opened."; });
  }

  function keyOf(url) { return url.pathname + "?" + (url.searchParams.get("path") || ""); }

  // A turn that is drawn again names the files it changed, so an open file it names is read again: what is open stays what an agent,
  // or a colleague's agent, has just written.
  var turns = document.getElementById("turns");
  if (turns && window.MutationObserver) {
    var stale = {};
    var timer = null;
    new MutationObserver(function (changes) {
      changes.forEach(function (change) {
        [].forEach.call(change.addedNodes, function (node) {
          if (!node.querySelectorAll) { return; }
          [].forEach.call(node.querySelectorAll('a[href*="/file?path="]'), function (link) {
            var key = keyOf(new URL(link.href, location.href));
            if (open[key]) { stale[key] = true; }
          });
        });
      });
      if (timer || !Object.keys(stale).length) { return; }
      timer = setTimeout(function () {
        Object.keys(stale).forEach(function (key) { if (open[key]) { load(open[key]); } });
        stale = {};
        timer = null;
      }, 300);
    }).observe(turns, { childList: true, subtree: true });
  }

  // Scripts that come with a tab's html are not run by being put on the page, so the ones it names are loaded in turn: code is colored
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

  var moves = { ArrowRight: 1, ArrowLeft: -1, Home: -Infinity, End: Infinity };
  tabs.addEventListener("keydown", function (event) {
    var keys = Object.keys(open);
    var at = keys.map(function (key) { return open[key].name; }).indexOf(document.activeElement);
    if (at < 0) { return; }
    if (event.key === "Delete") { event.preventDefault(); close(keys[at]); return; }
    if (!(event.key in moves)) { return; }
    event.preventDefault();
    var next = Math.abs(moves[event.key]) === 1 ? (at + moves[event.key] + keys.length) % keys.length : Math.max(0, Math.min(keys.length - 1, moves[event.key]));
    show(keys[next], true);
  });

  function view(href, link) {
    opener = link;
    var url = new URL(href, location.href);
    var key = keyOf(url);
    panel.hidden = false;
    if (!open[key]) { add(url, key, link && link.dataset.tabName); }
    show(key);
  }

  document.addEventListener("click", function (event) {
    var link = event.target.closest && event.target.closest('a[href*="/file?path="], a[href*="/files/"][href*="/jobs/"], a[data-files-panel]');
    if (!link || link.hasAttribute("download") || link.hasAttribute("data-page") || event.defaultPrevented) { return; }
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) { return; }
    if (!(link.closest(".said-so-far, .fp-bodies") || link.hasAttribute("data-files-panel")) || !roomy()) { return; }
    event.preventDefault();
    view(link.href, link);
  });
  document.getElementById("fp-close").addEventListener("click", function () {
    Object.keys(open).forEach(close);
    panel.hidden = true;
  });
});
