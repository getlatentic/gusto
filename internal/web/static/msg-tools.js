// A message's toolbar: copy puts the message's text on the clipboard, and reply starts a message to its sender with an @ mention.
// Messages that arrive live carry the same buttons, so the clicks are taken on the document.
(function () {
  var box = document.querySelector(".composer textarea");

  function copy(button) {
    var body = button.closest("[data-msg]").querySelector("[data-msg-body]");
    if (!body || !navigator.clipboard) { return; }
    navigator.clipboard.writeText(body.innerText.trim()).then(function () {
      var said = button.getAttribute("aria-label");
      button.setAttribute("aria-label", "Copied");
      button.dataset.done = "";
      setTimeout(function () { button.setAttribute("aria-label", said); delete button.dataset.done; }, 1500);
    });
  }

  function reply(button) {
    if (!box) { return; }
    var put = "@" + button.dataset.reply + " ";
    if (box.value.indexOf(put) !== 0) { box.value = put + box.value; }
    box.focus();
    box.setSelectionRange(box.value.length, box.value.length);
    box.dispatchEvent(new Event("input", { bubbles: true }));
  }

  document.addEventListener("click", function (event) {
    var button = event.target.closest("[data-msg-copy], [data-reply]");
    if (!button) { return; }
    if (button.hasAttribute("data-msg-copy")) { copy(button); } else { reply(button); }
  });
})();
