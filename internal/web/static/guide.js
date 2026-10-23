// A short guided tour for a demo host. It does nothing anywhere else: the lease script says when
// this is a demo, and the tour opens then. Each step ticks itself off as the visitor does it, and
// what has been done is kept in this browser so a reload does not start over.
(function () {
  var KEY = "gusto-guide";
  var PROMPT = "Draft a one-page policy for how our 40-person company should use AI agents, and save it as policy.md.";
  var state = { done: {}, closed: false };
  var panel = null, list = null, toggle = null, book = "";

  try { state = Object.assign(state, JSON.parse(localStorage.getItem(KEY) || "{}")); } catch (e) {}
  function save() { try { localStorage.setItem(KEY, JSON.stringify(state)); } catch (e) {} }

  var steps = [
    { key: "ask", title: "Ask an agent",
      hint: "Send the suggested question. The agent works in its own sandbox and writes a file.",
      label: "Start a chat", href: function () { return "/chat?prompt=" + encodeURIComponent(PROMPT); } },
    { key: "verify", title: "See what it did",
      hint: "Every reply has details: the files it changed, what it cost and the sites it was refused. Open them from the reply.",
      label: "Open the details", href: function () { var a = document.querySelector('a[href^="/jobs/"]'); return a ? a.getAttribute("href") : ""; } },
    { key: "control", title: "See the controls",
      hint: "Spending limits, who can do what, and every model call, in one place for the admin.",
      label: "Open the admin", href: function () { return "/admin"; } },
    { key: "talk", title: "Bring it to your team",
      hint: "Talk to the people who built it, or keep exploring until the demo ends.",
      label: "Book a call", href: function () { return book; }, external: true }
  ];

  function observe() {
    var path = location.pathname;
    if (/^\/instances\/[^/]+\/threads\/[^/]+/.test(path) && document.querySelector('a[href^="/jobs/"]')) { state.done.ask = true; }
    if (/^\/jobs\/[^/]+/.test(path)) { state.done.ask = true; state.done.verify = true; }
    if (/^\/admin/.test(path)) { state.done.control = true; }
  }

  function make(tag, classes, text) {
    var node = document.createElement(tag);
    if (classes) { node.className = classes; }
    if (text) { node.textContent = text; }
    return node;
  }

  var tick = '<svg class="size-3 opacity-100" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg>';

  function draw() {
    var count = steps.filter(function (s) { return state.done[s.key]; }).length;
    toggle.querySelector(".count").textContent = count + " of " + steps.length;
    toggle.setAttribute("aria-expanded", String(!state.closed));
    list.hidden = state.closed;
    list.textContent = "";
    var current = steps.find(function (s) { return !state.done[s.key]; });
    steps.forEach(function (step) {
      var done = state.done[step.key] === true;
      var active = current === step;
      var row = make("li", "grid grid-cols-[1.5rem_minmax(0,1fr)] gap-3 py-3 first:pt-0 last:pb-0");
      var mark = make("span", "mt-0.5 grid size-5 place-items-center rounded-full text-xs font-semibold " + (done ? "bg-ok-wash text-ok" : active ? "bg-solid text-on-solid" : "bg-wash text-subtle"));
      if (done) { mark.innerHTML = tick; } else { mark.textContent = String(steps.indexOf(step) + 1); }
      var body = make("div", "grid min-w-0 gap-1");
      body.appendChild(make("span", "text-sm font-semibold " + (done ? "text-muted" : "text-ink"), step.title));
      var hint = step.key === "ask" && location.pathname === "/chat" ? "The question is ready in the box. Press send and the agent starts." : step.hint;
      if (active || (!done && step.key === "talk")) { body.appendChild(make("span", "text-xs leading-relaxed text-muted", hint)); }
      var href = step.href();
      if (step.key === "ask" && location.pathname === "/chat" && !done) { href = ""; }
      if (!done && href && (active || step.key === "talk")) {
        var go = make("a", "mt-1 inline-flex w-fit items-center rounded-sm bg-solid px-3 py-1.5 text-xs font-semibold text-on-solid no-underline hover:bg-solid-hover", step.label);
        go.href = href;
        if (step.external) { go.target = "_blank"; go.rel = "noopener"; go.addEventListener("click", function () { state.done.talk = true; save(); draw(); }); }
        body.appendChild(go);
      }
      row.append(mark, body);
      list.appendChild(row);
    });
  }

  var docked = "mt-auto grid gap-3 rounded-md border border-line bg-surface p-3";
  var floating = "fixed bottom-[calc(var(--lease-h,0rem)+0.75rem)] left-3 z-[900] grid w-[min(21rem,calc(100vw-1.5rem))] gap-3 rounded-md border border-line bg-surface p-3 shadow-3";

  // Beside the conversations when they are on screen, so it never sits over what a person is typing; a small card
  // that starts closed when they are not.
  function mount() {
    var side = document.getElementById("sidepanel");
    var shown = Boolean(side) && (side.checkVisibility ? side.checkVisibility({ visibilityProperty: true }) : side.offsetParent !== null);
    panel.className = shown ? docked : floating;
    if (shown && panel.parentNode !== side) { side.appendChild(panel); }
    if (!shown && panel.parentNode !== document.body) { document.body.appendChild(panel); if (!state.chose) { state.closed = true; } }
    draw();
  }

  function build() {
    panel = make("aside", docked);
    panel.setAttribute("aria-label", "Demo guide");
    toggle = make("button", "flex w-full cursor-pointer items-center justify-between gap-3 text-left");
    toggle.type = "button";
    toggle.setAttribute("aria-controls", "demo-guide-steps");
    toggle.innerHTML = '<span class="text-sm font-semibold text-ink">Take the tour</span><span class="count rounded-full bg-wash px-2 py-0.5 text-xs font-medium text-muted"></span>';
    toggle.addEventListener("click", function () { state.closed = !state.closed; state.chose = true; save(); draw(); });
    list = make("ol", "m-0 grid list-none divide-y divide-line p-0");
    list.id = "demo-guide-steps";
    panel.append(toggle, list);
    var resizing = null;
    window.addEventListener("resize", function () { clearTimeout(resizing); resizing = setTimeout(mount, 150); });
  }

  function start(detail) {
    if (panel) { return; }
    book = (detail && detail.book) || "";
    observe();
    save();
    build();
    mount();
  }

  if (window.gustoDemo) { start(window.gustoDemo); }
  document.addEventListener("gusto:demo", function (event) { start(event.detail); });
})();
