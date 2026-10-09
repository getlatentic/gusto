// The Files page: the explorer, open at the place the address names, with the address following it so that the way back is the browser's.
(function () {
  var container = document.getElementById("files-app");
  if (!container) { return; }
  new window.Explorer(container, {
    mode: "browse",
    start: new URLSearchParams(location.search).get("at") || "",
    onMove: function (at) { history.replaceState(null, "", at ? "/files?at=" + encodeURIComponent(at) : "/files"); }
  });
})();
