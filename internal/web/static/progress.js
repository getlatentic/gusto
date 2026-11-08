// What each agent that is working in a room is doing right now, written into its turn in place of "Working". Several agents can be
// working at once, so one request asks for all of them every two seconds while the page is showing and a turn is running.
(function () {
  var turns = document.getElementById("turns");
  var form = document.getElementById("say-form");
  if (!turns || !form) { return; }
  var path = form.getAttribute("action").replace(/\/say$/, "");
  var doing = {};
  var asking = false;

  function words(progress) {
    return progress.what ? progress.what + (progress.steps > 1 ? " · step " + progress.steps : "") : "";
  }

  function show(article, progress) {
    var place = article.querySelector("[data-progress]");
    var now = words(progress);
    if (!place || !now) { return; }
    var held = place.dataset.now === now + "\u0000" + (progress.detail || "");
    if (held) { return; }
    place.dataset.now = now + "\u0000" + (progress.detail || "");
    place.textContent = now;
    if (progress.detail) {
      var detail = document.createElement("span");
      detail.className = "ml-2 font-mono text-xs text-subtle";
      detail.textContent = progress.detail;
      detail.title = progress.detail;
      place.appendChild(detail);
    }
  }

  // A turn drawn again, as when its agent is told of a change, comes with "Working" until the next answer, so what is known is put
  // back at once.
  function settle() {
    turns.querySelectorAll("[data-pending]").forEach(function (article) {
      if (doing[article.id]) { show(article, doing[article.id]); }
    });
  }

  function ask() {
    if (asking || document.hidden || !turns.querySelector("[data-progress]")) { return; }
    asking = true;
    fetch(path + "/progress", { credentials: "same-origin", cache: "no-store" })
      .then(function (answer) { return answer.ok ? answer.json() : null; })
      .then(function (found) {
        if (found) { doing = found; settle(); }
      })
      .catch(function () {})
      .then(function () { asking = false; });
  }

  new MutationObserver(settle).observe(turns, { childList: true, subtree: true });
  setInterval(ask, 2000);
  document.addEventListener("visibilitychange", ask);
  ask();
})();
