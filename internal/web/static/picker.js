// A picker is a button that names what is chosen and a menu above it that lists the choices, each with a quieter line under its name.
// It carries its value in a hidden input, tells the page when the choice changes ("pick"), and can be given other choices
// (picker.setOptions), which is how choosing a harness changes the models on offer.
(function () {
  function start(root) {
    var input = root.querySelector("input");
    var button = root.querySelector("button");
    var label = root.querySelector(".label");
    var menu = root.querySelector(".menu");
    var options = JSON.parse(root.dataset.options || "[]");

    function render() {
      menu.textContent = "";
      options.forEach(function (option) {
        var item = document.createElement("button");
        item.type = "button";
        item.setAttribute("role", "option");
        item.dataset.value = option.value;
        var chosen = option.value === input.value;
        item.setAttribute("aria-selected", String(chosen));
        var name = document.createElement("span");
        name.className = "name";
        name.textContent = option.label;
        item.appendChild(name);
        if (option.detail) {
          var detail = document.createElement("span");
          detail.className = "detail";
          detail.textContent = option.detail;
          item.appendChild(detail);
        }
        menu.appendChild(item);
      });
      var current = options.find(function (option) { return option.value === input.value; });
      label.textContent = current ? current.label : (root.dataset.label || "");
    }

    function close() { root.classList.remove("open"); button.setAttribute("aria-expanded", "false"); }
    function open() {
      root.classList.add("open");
      button.setAttribute("aria-expanded", "true");
      var chosen = menu.querySelector('[aria-selected="true"]') || menu.firstElementChild;
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

    button.addEventListener("click", function () { root.classList.contains("open") ? close() : open(); });
    menu.addEventListener("click", function (event) {
      var item = event.target.closest("[role=option]");
      if (item) { choose(item.dataset.value); }
    });
    root.addEventListener("keydown", function (event) {
      var items = Array.prototype.slice.call(menu.children);
      var at = items.indexOf(document.activeElement);
      if (event.key === "Escape" && root.classList.contains("open")) { event.stopPropagation(); close(); button.focus(); }
      else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        if (!root.classList.contains("open")) { open(); return; }
        var next = event.key === "ArrowDown" ? Math.min(at + 1, items.length - 1) : Math.max(at - 1, 0);
        items[next].focus();
      }
    });
    document.addEventListener("click", function (event) { if (!root.contains(event.target)) { close(); } });

    root.picker = {
      get value() { return input.value; },
      setOptions: function (next, value) { options = next; input.value = value || ""; render(); },
      // select chooses one of the options as if it had been picked, and lock says whether the person may choose another.
      select: function (value) { if (value !== input.value) { choose(value); } },
      lock: function (locked) { button.disabled = locked; root.classList.toggle("locked", locked); if (locked) { close(); } }
    };
    render();
  }
  document.querySelectorAll("[data-picker]").forEach(start);
})();
