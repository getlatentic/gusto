// A text file is changed in the page it is read in, or in its tab beside a chat. Saving sends what the file was when it was
// opened, and the server refuses it when the file has changed since, so nobody's change is put over without them seeing it.
// It is loaded with every file that can be changed, and listens once for all of them.
(function () {
  if (window.gustoFileEdit) { return; }
  window.gustoFileEdit = true;

  function parts(within) {
    var file = within.closest("[data-file]");
    return file && { file: file, form: file.querySelector("[data-file-edit]"), view: file.querySelector("[data-file-view]"), edit: file.querySelector("[data-edit]") };
  }

  function editing(each, on) {
    each.form.hidden = !on;
    each.view.hidden = on;
    if (each.edit) { each.edit.hidden = on; }
    if (on) { each.form.querySelector("textarea").focus(); }
  }

  document.addEventListener("click", function (event) {
    var button = event.target.closest && event.target.closest("[data-edit], [data-edit-cancel]");
    var each = button && parts(button);
    if (!each || !each.form) { return; }
    editing(each, button.hasAttribute("data-edit"));
    if (button.hasAttribute("data-edit-cancel") && each.file.closest(".fp-bodies")) {
      each.file.dispatchEvent(new CustomEvent("gusto-file-saved", { bubbles: true }));
    }
  });

  document.addEventListener("submit", function (event) {
    var form = event.target;
    if (!form.matches || !form.matches("[data-file-edit]")) { return; }
    event.preventDefault();
    var each = parts(form);
    var said = form.querySelector("[data-edit-said]");
    var save = form.querySelector("button[type=submit]");
    save.disabled = true;
    said.hidden = true;
    fetch(form.action, { method: "POST", credentials: "same-origin", body: new URLSearchParams(new FormData(form)) })
      .then(function (answer) { return answer.json().then(function (body) { return { ok: answer.ok, body: body }; }); })
      .then(function (result) {
        if (!result.ok) { throw new Error(result.body.error || "it was not saved"); }
        form.querySelector("[name=seen]").value = result.body.hash;
        // A tab beside a chat is read again by the panel; a page of its own is drawn again.
        if (each.file.closest(".fp-bodies")) {
          editing(each, false);
          each.file.dispatchEvent(new CustomEvent("gusto-file-saved", { bubbles: true }));
        } else {
          location.reload();
        }
      })
      .catch(function (why) {
        said.textContent = "It was not saved: " + why.message;
        said.hidden = false;
        save.disabled = false;
      });
  });
})();
