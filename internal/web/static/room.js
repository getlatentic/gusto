// A room's message box: an @ offers the room's agents and people to mention, a mention shows green as it is typed, and what
// others say comes in without reloading the page.
(function () {
  var menu = document.getElementById("mention-menu");
  var box = document.querySelector(".composer textarea");
  if (!menu || !box) { return; }
  var everyone = JSON.parse(menu.dataset.people);
  var marks = document.getElementById("composer-marks");
  var mention = /(^|[^\w@])(@[A-Za-z](?:[A-Za-z0-9_.-]*[A-Za-z0-9_-])?)/g;
  var asking = /(^|[^\w@])@([A-Za-z0-9_.-]*)$/;
  var shown = [], active = 0, at = -1;

  function escape(text) {
    return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }

  // The box's own text is clear, so the copy behind it shows through with its mentions coloured.
  function paint() {
    marks.innerHTML = escape(box.value).replace(mention, '$1<span class="text-accent">$2</span>') + "\n";
    marks.scrollTop = box.scrollTop;
  }

  function avatar(person) {
    var face = document.createElement("span");
    face.className = "grid size-5 flex-none place-items-center overflow-hidden rounded-full bg-sunken text-[0.625rem] font-medium text-muted";
    if (person.icon) {
      var img = document.createElement("img");
      img.src = person.icon;
      img.alt = "";
      img.className = "size-3.5 object-contain";
      face.appendChild(img);
    } else if (person.short) {
      face.textContent = person.short;
    } else {
      face.innerHTML = '<svg class="i size-3" viewBox="0 0 24 24" aria-hidden="true"><path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>';
    }
    return face;
  }

  function row(person, index) {
    var item = document.createElement("li");
    item.id = "mention-" + index;
    item.setAttribute("role", "option");
    item.setAttribute("aria-selected", index === active ? "true" : "false");
    item.className = "flex h-9 cursor-pointer items-center gap-2 rounded-lg px-2 text-sm aria-selected:bg-sunken";
    item.appendChild(avatar(person));
    var name = document.createElement("span");
    name.className = "font-medium text-ink";
    name.textContent = person.name;
    item.appendChild(name);
    if (person.role) {
      var role = document.createElement("span");
      role.className = "min-w-0 truncate text-subtle";
      role.textContent = person.role;
      item.appendChild(role);
    }
    if (person.agent) {
      var badge = document.createElement("span");
      badge.className = "ml-auto flex-none rounded-full bg-accent-wash px-1.5 text-xs leading-5 text-accent-ink";
      badge.textContent = "Agent";
      item.appendChild(badge);
    }
    item.addEventListener("mousedown", function (event) { event.preventDefault(); choose(index); });
    return item;
  }

  function heading(text) {
    var item = document.createElement("li");
    item.setAttribute("role", "presentation");
    item.className = "px-2 pt-1.5 pb-1 text-xs font-medium text-subtle";
    item.textContent = text;
    return item;
  }

  function close() {
    menu.hidden = true;
    box.removeAttribute("aria-activedescendant");
    box.setAttribute("aria-expanded", "false");
    shown = [];
  }

  function render() {
    menu.textContent = "";
    var agents = shown.filter(function (each) { return each.agent; });
    var people = shown.filter(function (each) { return !each.agent; });
    shown = agents.concat(people);
    if (agents.length) { menu.appendChild(heading("Agents")); }
    agents.forEach(function (each, i) { menu.appendChild(row(each, i)); });
    if (people.length) { menu.appendChild(heading("People")); }
    people.forEach(function (each, i) { menu.appendChild(row(each, agents.length + i)); });
    menu.hidden = false;
    box.setAttribute("aria-expanded", "true");
    box.setAttribute("aria-activedescendant", "mention-" + active);
  }

  // What is being asked for is the word after an @ just before the caret.
  function look() {
    var before = box.value.slice(0, box.selectionStart);
    var found = asking.exec(before);
    if (!found) { close(); return; }
    var typed = found[2].toLowerCase();
    at = before.length - typed.length - 1;
    shown = everyone.filter(function (each) { return each.name.toLowerCase().indexOf(typed) === 0; });
    if (!shown.length) { close(); return; }
    active = 0;
    render();
  }

  function choose(index) {
    var person = shown[index];
    if (!person) { return; }
    var end = box.selectionStart;
    var put = "@" + person.name + " ";
    box.value = box.value.slice(0, at) + put + box.value.slice(end);
    box.selectionStart = box.selectionEnd = at + put.length;
    close();
    box.dispatchEvent(new Event("input"));
    box.focus();
  }

  box.addEventListener("input", function () { paint(); look(); });
  box.addEventListener("scroll", function () { marks.scrollTop = box.scrollTop; });
  box.addEventListener("click", look);
  box.addEventListener("blur", close);
  // Registered for the capture phase so that, while the list is open, Enter picks someone rather than sending.
  box.addEventListener("keydown", function (event) {
    if (menu.hidden) { return; }
    var moves = { ArrowDown: 1, ArrowUp: -1 };
    if (event.key in moves) {
      active = (active + moves[event.key] + shown.length) % shown.length;
      render();
    } else if (event.key === "Enter" || event.key === "Tab") {
      choose(active);
    } else if (event.key === "Escape") {
      close();
    } else {
      return;
    }
    event.preventDefault();
    event.stopImmediatePropagation();
  }, true);
  box.form.addEventListener("reset", function () { setTimeout(paint, 0); });
  paint();

  // Others talk in a room too, and several agents can be working at once, so the room looks again every few seconds and puts in
  // what changed, keeping the reader at the end when they were there.
  var turns = document.getElementById("turns");
  var said = document.getElementById("said");
  var path = box.form.getAttribute("action").replace(/\/say$/, "");
  var last = turns.innerHTML;
  setInterval(function () {
    if (document.hidden) { return; }
    fetch(path + "/turns", { credentials: "same-origin" }).then(function (answer) { return answer.ok ? answer.text() : null; }).then(function (html) {
      if (html === null || html === last) { return; }
      var nearEnd = said.scrollHeight - said.scrollTop - said.clientHeight < 96;
      last = html;
      turns.innerHTML = html;
      if (nearEnd) { said.scrollTop = said.scrollHeight; }
    });
  }, 3000);
})();
