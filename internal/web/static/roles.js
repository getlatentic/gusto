// The new-agent page: a list of roles on the left, the one picked on the right, and a search and a field of work to narrow the list.
(function () {
  var root = document.getElementById("roles");
  if (!root) { return; }
  var picks = [].slice.call(root.querySelectorAll(".pick-role"));
  var details = [].slice.call(root.querySelectorAll(".role-detail"));
  var chips = [].slice.call(root.querySelectorAll(".chip"));
  var search = document.getElementById("role-search");
  var none = root.querySelector(".none");
  var select = document.getElementById("role-select");
  var field = "";

  function show(name) {
    picks.forEach(function (pick) { pick.setAttribute("aria-pressed", String(pick.dataset.role === name)); });
    details.forEach(function (detail) { detail.hidden = detail.dataset.for !== name; });
    if (select) { select.value = name; }
  }
  function narrow() {
    var needle = search.value.trim().toLowerCase();
    var shown = picks.filter(function (pick) {
      var fits = (!field || pick.dataset.field === field) && (!needle || pick.textContent.toLowerCase().indexOf(needle) >= 0);
      pick.hidden = !fits;
      return fits;
    });
    none.hidden = shown.length > 0;
    var current = picks.filter(function (pick) { return pick.getAttribute("aria-pressed") === "true" && !pick.hidden; })[0];
    if (!current && shown.length) { show(shown[0].dataset.role); }
  }
  picks.forEach(function (pick) { pick.addEventListener("click", function () { show(pick.dataset.role); }); });
  chips.forEach(function (chip) {
    chip.addEventListener("click", function () {
      field = chip.dataset.field;
      chips.forEach(function (other) { other.setAttribute("aria-pressed", String(other === chip)); });
      narrow();
    });
  });
  search.addEventListener("input", narrow);
  if (select) { select.addEventListener("change", function () { show(select.value); }); }
})();
