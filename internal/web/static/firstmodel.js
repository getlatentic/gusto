// The form a person adds their first model with: a provider fills in where it answers and how, and List my models asks the
// provider which models the key can use.
(function () {
  var form = document.querySelector("form.first-model");
  if (!form) return;
  var preset = form.querySelector("#preset");
  var address = form.querySelector("#address");
  var auth = form.querySelector("#auth");
  var said = form.querySelector("#list-said");
  var found = document.getElementById("found-models");
  var models = form.querySelector("#models");

  var custom = form.querySelector("#custom");

  // A provider the app knows fills in where it answers and how, and those two fields are then out of the way; another provider
  // asks for them.
  function choose() {
    var option = preset.options[preset.selectedIndex];
    var known = !!option.value;
    custom.hidden = known;
    if (known) {
      address.value = option.value;
      auth.value = option.getAttribute("data-auth") === "anthropic" ? "anthropic" : "";
    } else {
      address.value = "";
      auth.value = "";
      address.focus();
    }
  }
  preset.addEventListener("change", choose);
  choose();

  form.querySelector("#list-models").addEventListener("click", function () {
    said.textContent = "Asking the provider…";
    var body = new URLSearchParams({ address: address.value, auth: auth.value, key: form.querySelector("#key").value });
    fetch("/setup/models", { method: "POST", body: body, headers: { "Content-Type": "application/x-www-form-urlencoded" } })
      .then(function (answer) { return answer.json(); })
      .then(function (listed) {
        if (listed.error) { said.textContent = listed.error; return; }
        found.innerHTML = "";
        listed.models.forEach(function (name) {
          var option = document.createElement("option");
          option.value = name;
          found.appendChild(option);
        });
        said.textContent = listed.models.length + " models. Start typing one, or pick from the list.";
        models.focus();
      })
      .catch(function () { said.textContent = "The provider could not be asked."; });
  });
})();
