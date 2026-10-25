// A demo's cell takes most of a minute to start, so it is asked for when the visitor first types in the form, and is up or
// nearly by the time they send it.
(function () {
  var form = document.querySelector("form[data-warm]");
  if (!form) { return; }
  form.addEventListener("input", function () {
    fetch(form.dataset.warm, { method: "POST", credentials: "same-origin" }).catch(function () {});
  }, { once: true });
})();
