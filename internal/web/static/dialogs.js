// Dialogs over a page: what names one opens it, a close button or a click outside closes it, a search box narrows the list it names,
// and an address that asks for one, as a change made in it comes back with, opens it and is put back so a reload does not open it
// again.
(function () {
  function open(id) {
    var dialog = document.getElementById(id);
    if (!dialog) { return; }
    document.querySelectorAll("dialog[open]").forEach(function (other) { if (other !== dialog) { other.close(); } });
    dialog.showModal();
  }
  document.querySelectorAll("[data-open]").forEach(function (opener) {
    opener.addEventListener("click", function (event) { event.preventDefault(); open(opener.dataset.open); });
  });
  document.querySelectorAll("dialog [data-close]").forEach(function (closer) {
    closer.addEventListener("click", function () { closer.closest("dialog").close(); });
  });
  document.querySelectorAll("dialog").forEach(function (dialog) {
    dialog.addEventListener("click", function (event) { if (event.target === dialog) { dialog.close(); } });
  });
  document.querySelectorAll("[data-filter]").forEach(function (search) {
    var list = document.getElementById(search.dataset.filter);
    if (!list) { return; }
    search.addEventListener("input", function () {
      var wanted = search.value.trim().toLowerCase();
      list.querySelectorAll("[data-name]").forEach(function (row) { row.hidden = wanted !== "" && row.dataset.name.toLowerCase().indexOf(wanted) < 0; });
    });
  });
  var asked = new URLSearchParams(location.search);
  var wanted = "";
  document.querySelectorAll("dialog[data-asked]").forEach(function (dialog) {
    if (!wanted && dialog.dataset.asked.split(" ").some(function (name) { return asked.has(name); })) { wanted = dialog.id; }
  });
  if (wanted) {
    open(wanted);
    history.replaceState(null, "", location.pathname + location.hash);
  }
})();
