// A button with data-reveal shows the section it names, and puts the person in its first field.
(function () {
  document.querySelectorAll("[data-reveal]").forEach(function (button) {
    var section = document.getElementById(button.dataset.reveal);
    if (!section) { return; }
    button.addEventListener("click", function () {
      section.hidden = !section.hidden;
      var field = section.querySelector("input, select, textarea");
      if (!section.hidden && field) { field.focus(); }
    });
  });
})();
