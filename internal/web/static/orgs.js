// The organization switcher behind the mark at the top of the rail. Gusto cloud knows which organizations and
// workspaces the person is in, so the list is asked of it when the menu first opens; until it answers, or if it
// cannot, the menu shows this organization and the link to gusto cloud's page of them. An organization the person
// is in two or more workspaces of lists them under its name; one with a single workspace is one line.
(function () {
  var panel = document.getElementById("orgs");
  var open = document.querySelector("[data-orgs]");
  if (!panel || !open) { return; }
  var list = panel.querySelector("[data-orgs-list]");
  var asked = false;
  var check = list.querySelector("svg");

  function line(text, href, current, inset) {
    var item = document.createElement(current ? "span" : "a");
    item.className = "flex h-10 items-center gap-3 rounded-sm px-3 text-sm font-medium text-ink no-underline hover:bg-hover [&_svg.i]:size-4" + (current ? " bg-hover" : "") + (inset ? " pl-6" : "");
    if (!current) { item.href = href; }
    var name = document.createElement("span");
    name.className = "min-w-0 flex-1 truncate";
    name.textContent = text;
    item.appendChild(name);
    if (current && check) { item.appendChild(check.cloneNode(true)); }
    return item;
  }

  function entries(org) {
    var here = org.slug === panel.dataset.current;
    var spaces = Array.isArray(org.workspaces) ? org.workspaces : [];
    if (spaces.length < 2) { return [line(org.name, org.open, here, false)]; }
    var heading = document.createElement("p");
    heading.className = "truncate px-3 pt-2 pb-1 text-xs font-medium text-muted";
    heading.textContent = org.name;
    return [heading].concat(spaces.map(function (space) {
      var current = here && space.id === panel.dataset.workspace;
      return line(space.name, here ? "/workspace/" + encodeURIComponent(space.id) : space.open, current, true);
    }));
  }

  function fill() {
    if (asked) { return; }
    asked = true;
    fetch(panel.dataset.from, { credentials: "include", cache: "no-store" })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (d) {
        if (!d || !Array.isArray(d.orgs) || d.orgs.length === 0) { return; }
        list.replaceChildren.apply(list, [].concat.apply([], d.orgs.map(entries)));
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
