// Reminds a person who has not confirmed their email address, in the side panel, and mails the link again. Only
// the front door knows whether they have, so it is asked when a page with the side panel loads.
(function () {
  var card = document.querySelector("[data-reminder]");
  if (!card) { return; }
  var said = card.querySelector("[data-reminder-said]");
  var send = card.querySelector("[data-reminder-send]");

  function say(text) {
    said.textContent = text;
    said.hidden = false;
  }

  fetch(card.dataset.from, { credentials: "include", cache: "no-store" })
    .then(function (r) { return r.ok ? r.json() : null; })
    .then(function (d) {
      if (!d || d.confirmed) { return; }
      card.querySelector("[data-reminder-email]").textContent = d.email;
      card.querySelector("[data-reminder-jobs]").textContent = String(d.jobs);
      card.hidden = false;
    })
    .catch(function () {});

  send.addEventListener("click", function () {
    send.disabled = true;
    fetch(card.dataset.send, { method: "POST", credentials: "include" })
      .then(function (r) {
        if (r.ok) { say("Sent. Check your inbox."); return; }
        send.disabled = false;
        say(r.status === 429 ? "That address has been mailed too often. Try again later." : "The link could not be sent. Try again.");
      })
      .catch(function () {
        send.disabled = false;
        say("The link could not be sent. Try again.");
      });
  });
})();
