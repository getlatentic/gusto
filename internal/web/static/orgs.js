// The organization switcher behind the mark at the top of the rail. The front door knows which organizations the
// person is in, so the list is asked of it when the menu first opens; until it answers, or if it cannot, the menu
// shows this organization and the link to the front door's page of them.
(function () {
  var panel = document.getElementById("orgs");
  var open = document.querySelector("[data-orgs]");
  if (!panel || !open) { return; }
  var list = panel.querySelector("[data-orgs-list]");
  var asked = false;
  var check = list.querySelector("svg");

  function entry(org) {
    var current = org.slug === panel.dataset.current;
    var item = document.createElement(current ? "span" : "a");
    item.className = "flex h-10 items-center gap-3 rounded-sm px-3 text-sm font-medium text-ink no-underline hover:bg-hover [&_svg.i]:size-4" + (current ? " bg-hover" : "");
    if (!current) { item.href = org.open; }
    var name = document.createElement("span");
    name.className = "min-w-0 flex-1 truncate";
    name.textContent = org.name;
    item.appendChild(name);
    if (current && check) { item.appendChild(check.cloneNode(true)); }
    return item;
  }

  function fill() {
    if (asked) { return; }
    asked = true;
    fetch(panel.dataset.from, { credentials: "include", cache: "no-store" })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (d) {
        if (!d || !Array.isArray(d.orgs) || d.orgs.length === 0) { return; }
        list.replaceChildren.apply(list, d.orgs.map(entry));
      })
      .catch(function () { asked = false; });
  }

  function setOpen(shown) {
    panel.hidden = !shown;
    open.setAttribute("aria-expanded", String(shown));
    if (shown) { fill(); }
  }
  open.addEventListener("click", function () {
    setOpen(panel.hidden);
    if (!panel.hidden) { (panel.querySelector("a") || panel).focus(); }
  });
  panel.addEventListener("focusout", function (event) {
    var to = event.relatedTarget;
    if (to && !panel.contains(to) && to !== open) { setOpen(false); }
  });
  document.addEventListener("click", function (event) { if (!panel.hidden && !event.target.closest("#orgs, [data-orgs]")) { setOpen(false); } });
  document.addEventListener("keydown", function (event) {
    if (event.key !== "Escape" || panel.hidden) { return; }
    var inside = panel.contains(document.activeElement);
    setOpen(false);
    if (inside) { open.focus(); }
  });
})();
