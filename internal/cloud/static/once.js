// A form that does something once, such as joining by an invitation, is sent once: a press while it is on its way does
// nothing, and the button says it is working.
document.querySelectorAll("form[data-once]").forEach(function (form) {
  form.addEventListener("submit", function (event) {
    if (form.dataset.sent) { event.preventDefault(); return; }
    form.dataset.sent = "1";
    var button = form.querySelector("button[type=submit]");
    if (!button) { return; }
    button.setAttribute("aria-disabled", "true");
    if (button.dataset.busy) { button.textContent = button.dataset.busy; }
  });
});
