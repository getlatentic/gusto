// The members list's tabs show everybody, only the people, or only the agents. The tab chosen last is kept for the next page.
(function () {
  var tabs = document.querySelectorAll("[data-member-tab]");
  if (!tabs.length) { return; }
  function show(name) {
    tabs.forEach(function (tab) { tab.setAttribute("aria-selected", String(tab.dataset.memberTab === name)); });
    document.querySelectorAll("[data-member-kind]").forEach(function (row) { row.hidden = name !== "all" && row.dataset.memberKind !== name; });
  }
  tabs.forEach(function (tab) {
    tab.addEventListener("click", function () {
      show(tab.dataset.memberTab);
      try { localStorage.setItem("gusto-member-tab", tab.dataset.memberTab); } catch (e) {}
    });
  });
  var kept = null;
  try { kept = localStorage.getItem("gusto-member-tab"); } catch (e) {}
  if (kept && document.querySelector('[data-member-tab="' + kept + '"]')) { show(kept); }
})();
