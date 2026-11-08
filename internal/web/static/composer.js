// The send button is for sending something: it stays off while the message box is empty and nothing is attached, and comes on with
// the first words or file.
(function () {
  var box = document.querySelector("textarea[name=said]");
  var send = box && box.form && box.form.querySelector("button[type=submit][title=Send]");
  if (!send) { return; }
  var attached = box.form.querySelector("[data-attach]");
  function sync() { send.disabled = box.value.trim() === "" && !(attached && attached.files.length); }
  box.addEventListener("input", sync);
  box.form.addEventListener("reset", function () { setTimeout(sync, 0); });
  sync();
})();
