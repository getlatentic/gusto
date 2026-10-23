// A picker is a button that names what is chosen and a menu above it that lists the choices, each with a quieter line under its name.
// It carries its value in a hidden input, tells the page when the choice changes ("pick"), and can be given other choices
// (picker.setOptions), which is how choosing a harness changes the models on offer.
(function () {
  var groupClass = "px-3 pt-3 pb-1 text-xs font-semibold text-subtle";
  var separatorClass = "mx-1 my-2 h-px bg-line";
  var itemClass = "relative grid min-h-11 w-full cursor-pointer content-center gap-0.5 rounded-sm border-0 bg-transparent px-3 py-2 pr-9 text-left text-ink hover:bg-wash focus-visible:bg-wash focus-visible:-outline-offset-2";
  var actionClass = " sticky bottom-0 min-h-10 bg-surface shadow-[0_-0.5rem_0.5rem_-0.25rem_var(--gu-surface)]";
  var checkMark = '<svg class="i absolute top-3.5 right-3 size-3.5 opacity-100" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg>';

  // An agent's mark is drawn as its owner gives it, with no box. Marks that are black turn white on a dark page.
  var markClass = "size-5 flex-none object-contain";
  var darkFiles = /\/agents\/(opencode|pi)\.svg(\?.*)?$/;
  var invertFiles = /\/agents\/(codex|hermes)\./;
  function mark(src, className) {
    var picture = document.createElement("img");
    picture.className = className;
    picture.alt = "";
    picture.src = src;
    return picture;
  }
  function plate(src) {
    var holder = document.createElement("span");
    holder.className = "inline-flex flex-none";
    holder.setAttribute("aria-hidden", "true");
    if (darkFiles.test(src)) {
      holder.appendChild(mark(src, markClass + " dark:hidden"));
      holder.appendChild(mark(src.replace(/\.svg(\?.*)?$/, "-dark.svg"), markClass + " hidden dark:block"));
    } else {
      holder.appendChild(mark(src, markClass + (invertFiles.test(src) ? " dark:invert" : "")));
    }
    return holder;
  }

  function start(root) {
    var input = root.querySelector("input");
    var button = root.querySelector("button");
    var label = root.querySelector(".label");
    var menu = root.querySelector(".menu");
    var options = JSON.parse(root.dataset.options || "[]");
    var locked = false;
    var badge = null;
    menu.id = root.id + "-menu";
    menu.setAttribute("aria-label", root.dataset.label || "");
    button.setAttribute("aria-controls", menu.id);

    function render() {
      menu.textContent = "";
      options.forEach(function (option) {
        if (option.kind === "heading" || option.kind === "separator") {
          var plain = document.createElement("div");
          plain.className = option.kind === "heading" ? groupClass : separatorClass;
          plain.setAttribute("role", "presentation");
          plain.textContent = option.label || "";
          menu.appendChild(plain);
          return;
        }
        var item = document.createElement("button");
        item.className = itemClass + (option.kind === "action" ? actionClass : "");
        item.type = "button";
        item.tabIndex = -1;
        item.setAttribute("role", "option");
        item.dataset.value = option.value;
        var chosen = option.value === input.value;
        item.setAttribute("aria-selected", String(chosen));
        var name = document.createElement("span");
        name.className = "text-sm font-medium" + (option.kind === "action" ? " flex items-center gap-2" : "");
        name.textContent = option.label;
        if (option.icon) {
          name.className += " flex items-center gap-2";
          name.insertBefore(plate(option.icon), name.firstChild);
        }
        item.appendChild(name);
        if (option.detail) {
          var detail = document.createElement("span");
          detail.className = "text-xs whitespace-nowrap text-subtle";
          detail.textContent = option.detail;
          item.appendChild(detail);
        }
        if (chosen) { item.insertAdjacentHTML("beforeend", checkMark); }
        menu.appendChild(item);
      });
      var current = options.find(function (option) { return !option.kind && option.value === input.value; });
      label.textContent = current ? current.label : (root.dataset.label || "");
      if (badge) { badge.remove(); badge = null; }
      if (current && current.icon) {
        badge = plate(current.icon);
        badge.classList.add("mr-1.5");
        label.parentNode.insertBefore(badge, label);
      }
      // The name says what is being chosen as well as what is chosen, and the whole choice is there when the label is cut.
      button.setAttribute("aria-label", current ? root.dataset.label + ": " + current.label : root.dataset.label || "");
      label.title = current ? current.label : "";
    }

    function close() { root.classList.remove("open"); button.setAttribute("aria-expanded", "false"); }
    function open() {
      if (locked) { return; }
      root.classList.add("open");
      button.setAttribute("aria-expanded", "true");
      var chosen = menu.querySelector('[aria-selected="true"]') || menu.querySelector("button");
      if (chosen) { chosen.focus(); }
    }
    function choose(value) {
      var changed = value !== input.value;
      input.value = value;
      render();
      close();
      button.focus();
      if (changed) { root.dispatchEvent(new CustomEvent("pick", { bubbles: true })); }
    }

    button.addEventListener("click", function () { if (root.classList.contains("open")) { close(); } else { open(); } });
    menu.addEventListener("click", function (event) {
      var item = event.target.closest("[role=option]");
      if (item) { choose(item.dataset.value); }
    });
    var moves = { ArrowDown: 1, ArrowUp: -1, Home: -Infinity, End: Infinity };
    root.addEventListener("keydown", function (event) {
      var items = Array.prototype.slice.call(menu.querySelectorAll("button"));
      var at = items.indexOf(document.activeElement);
      if (event.key === "Escape" && root.classList.contains("open")) { event.stopPropagation(); close(); button.focus(); }
      else if (event.key in moves) {
        event.preventDefault();
        if (!root.classList.contains("open")) { open(); return; }
        items[Math.max(0, Math.min(items.length - 1, at + moves[event.key]))].focus();
      }
    });
    // Tabbing out of the menu closes it. A click gives no element to go to, and the click handler below sees to that.
    root.addEventListener("focusout", function (event) { if (event.relatedTarget && !root.contains(event.relatedTarget)) { close(); } });
    document.addEventListener("click", function (event) { if (!root.contains(event.target)) { close(); } });

    root.picker = {
      get value() { return input.value; },
      setOptions: function (next, value) { options = next; input.value = value || ""; render(); },
      // select chooses one of the options as if it had been picked, and lock says whether the person may choose another.
      select: function (value) { if (value !== input.value) { choose(value); } },
      // A locked picker stays reachable, so that its button's title can say why it is locked.
      lock: function (lock) { locked = lock; button.setAttribute("aria-disabled", String(lock)); root.classList.toggle("locked", lock); if (lock) { close(); } }
    };
    render();
  }
  document.querySelectorAll("[data-picker]").forEach(start);
})();
