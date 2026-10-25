// The side list's tabs: every conversation, the newest first, the rooms with something unread, and the rooms that mention you.
// The tab chosen last is kept for the next page, since moving between conversations should not lose your place in the list.
(function () {
  var tabs = document.querySelectorAll("[data-side-tab]");
  if (!tabs.length) { return; }
  function show(name) {
    tabs.forEach(function (tab) { tab.setAttribute("aria-selected", tab.dataset.sideTab === name ? "true" : "false"); });
    document.querySelectorAll("[data-side-panel]").forEach(function (panel) { panel.hidden = panel.dataset.sidePanel !== name; });
  }
  tabs.forEach(function (tab) {
    tab.addEventListener("click", function () {
      show(tab.dataset.sideTab);
      try { localStorage.setItem("gusto-side-tab", tab.dataset.sideTab); } catch (e) {}
    });
  });
  var kept = null;
  try { kept = localStorage.getItem("gusto-side-tab"); } catch (e) {}
  if (kept && document.querySelector('[data-side-tab="' + kept + '"]')) { show(kept); }
})();
