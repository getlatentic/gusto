// A room, live. One WebSocket carries what others say and what agents do into the page the moment it happens, who is here and who
// is typing, and sends what you say, stop and have read. It is Centrifuge's client (centrifuge.js) over the server's /live.
//
// The room's page is complete without it. Until the socket is subscribed, and whenever it is not, room.js asks every few
// seconds whether anything changed, as it always did; and a message sent while the socket is down is sent by the form.
//
// The stream is positioned: the page says where in the room's event log it was drawn (data-seq, data-epoch), and the server
// replays what happened since, in order, so nothing is lost or shown twice across a reconnect or a restart. When it cannot
// (the log was reset, or the page was left too long) the whole conversation is fetched again.
(function () {
  var turns = document.getElementById("turns");
  var form = document.getElementById("say-form");
  if (!turns || !form || !turns.dataset.live || typeof Centrifuge === "undefined") { return; }

  var room = turns.dataset.room, thread = turns.dataset.thread, me = turns.dataset.me;
  var owner = "sharing" in turns.dataset;
  var said = document.getElementById("said");
  var box = form.querySelector("textarea");
  var send = form.querySelector("button[type=submit][title=Send]");
  var typingLine = document.getElementById("typing");
  var typingNews = document.getElementById("typing-news");
  var here = document.getElementById("here");
  var path = form.getAttribute("action").replace(/\/say$/, "");
  var empty = document.getElementById("empty");
  var live = window.gustoLive = { online: false };

  var client = new Centrifuge((location.protocol === "https:" ? "wss://" : "ws://") + location.host + turns.dataset.live, {
    minReconnectDelay: 500,
    maxReconnectDelay: 10000,
    timeout: 8000
  });

  // ---- the conversation -------------------------------------------------------------------------------------------------

  function nearEnd() { return said.scrollHeight - said.scrollTop - said.clientHeight < 96; }

  // A live update is drawn for nobody in particular; what is yours, and what only the room's owner is offered, is settled here.
  function settle(root) {
    root.querySelectorAll("[data-who]").forEach(function (each) {
      var own = each.dataset.who === me;
      each.classList.remove.apply(each.classList, (own ? each.dataset.theirs : each.dataset.ours).split(" "));
      each.classList.add.apply(each.classList, (own ? each.dataset.ours : each.dataset.theirs).split(" "));
    });
    if (owner) {
      root.querySelectorAll("[data-owner-only]").forEach(function (each) { each.hidden = false; });
    }
  }

  function fragment(html) {
    var holder = document.createElement("div");
    holder.innerHTML = html;
    return holder.firstElementChild;
  }

  // The room's greeting is for a room with nothing said in it.
  function greeted() {
    if (empty && turns.querySelector("article")) { empty.remove(); empty = null; }
  }

  function put(frame) {
    var node = fragment(frame.html);
    if (!node) { return; }
    var follow = nearEnd();
    var existing = document.getElementById(frame.id);
    if (existing) { existing.replaceWith(node); } else { turns.appendChild(node); }
    greeted();
    settle(node);
    node.querySelectorAll("[data-who]").forEach(function (each) { delete typists[each.dataset.who]; });
    showTyping();
    if (follow) { said.scrollTop = said.scrollHeight; }
    markSeen();
  }

  // The whole conversation again: after the stream could not be recovered, or when something changed that is not one turn. What
  // arrives while it is being fetched is held and applied after it, since the page it brings may be older than that.
  var fetching = false;
  var held = [];
  function snapshot() {
    if (fetching) { return; }
    fetching = true;
    fetch(path + "/turns", { credentials: "same-origin" })
      .then(function (answer) { return answer.ok ? answer.text() : null; })
      .then(function (html) {
        if (html === null) { return; }
        var follow = nearEnd();
        turns.innerHTML = html;
        greeted();
        settle(turns);
        if (follow) { said.scrollTop = said.scrollHeight; }
      })
      .catch(function () {})
      .then(function () {
        fetching = false;
        var later = held;
        held = [];
        later.forEach(apply);
      });
  }

  function apply(frame) {
    if (frame.v !== 1) { location.reload(); return; }
    if ((frame.type === "turn" || frame.type === "note") && frame.thread && frame.thread !== thread) { return; }
    if (fetching) { held.push(frame); return; }
    try {
      if (frame.type === "turn" || frame.type === "note") { put(frame); }
      else if (frame.type === "people" || frame.type === "refresh") { snapshot(); }
    } catch (e) {
      snapshot();
    }
  }

  // ---- the stream and the live channel ---------------------------------------------------------------------------------

  var stream = client.newSubscription("room:" + room, {
    positioned: true,
    recoverable: true,
    since: { offset: Number(turns.dataset.seq), epoch: turns.dataset.epoch }
  });
  stream.on("subscribed", function (ctx) {
    live.online = true;
    if (ctx.wasRecovering && !ctx.recovered) { snapshot(); }
    markSeen();
  });
  stream.on("subscribing", function () { live.online = false; });
  stream.on("unsubscribed", function (ctx) {
    live.online = false;
    // Taken out of the room, or the room is gone.
    if (ctx.code === 2403 || ctx.code === 103) { location.href = "/chat"; }
  });
  stream.on("publication", function (ctx) { apply(ctx.data); });

  var channel = client.newSubscription("live:" + room, { joinLeave: true });
  channel.on("subscribed", function () {
    channel.presence().then(function (result) { everyone = {}; Object.keys(result.clients).forEach(function (id) { arrive(result.clients[id]); }); drawHere(); }).catch(function () {});
  });
  channel.on("join", function (ctx) { arrive(ctx.info); drawHere(); });
  channel.on("leave", function (ctx) { delete everyone[ctx.info.client]; drawHere(); });
  channel.on("publication", function (ctx) {
    var data = ctx.data;
    if (data && data.type === "typing" && data.who !== me && (!data.thread || data.thread === thread)) {
      typists[data.who] = { name: data.name, until: Date.now() + 5000 };
      showTyping();
    }
  });
  // A socket that cannot open is tried again with a growing delay. When it is because the person signed out, which a page cannot
  // read from the failed upgrade, asking for the conversation tells it, and it stops trying; the page then goes on as it did before.
  var failures = 0;
  client.on("connected", function () { failures = 0; });
  client.on("error", function () {
    failures += 1;
    if (failures % 3 !== 0) { return; }
    fetch(path + "/version", { credentials: "same-origin", redirect: "manual" }).then(function (answer) {
      if (answer.type === "opaqueredirect" || answer.status === 401 || answer.status === 403) { client.disconnect(); }
    }).catch(function () {});
  });
  live.client = client;
  stream.subscribe();
  channel.subscribe();
  client.connect();

  // ---- who is here and who is typing -----------------------------------------------------------------------------------

  var everyone = {};
  var typists = {};

  function arrive(info) {
    if (!info || !info.user || info.user === me) { return; }
    everyone[info.client] = info;
  }

  // One avatar for each other person, however many tabs they have open.
  function drawHere() {
    if (!here) { return; }
    var seen = {};
    here.textContent = "";
    Object.keys(everyone).forEach(function (id) {
      var info = everyone[id];
      if (seen[info.user]) { return; }
      seen[info.user] = true;
      var detail = info.connInfo || {};
      var face = document.createElement("span");
      face.className = "grid size-6 place-items-center rounded-full bg-sunken text-[0.625rem] font-medium text-muted ring-2 ring-canvas";
      face.title = (detail.name || "Someone") + " is here";
      face.textContent = detail.short || "";
      here.appendChild(face);
    });
    here.hidden = !here.firstChild;
  }

  function typingWords() {
    var now = Date.now();
    var names = [];
    Object.keys(typists).forEach(function (who) {
      if (typists[who].until < now) { delete typists[who]; } else { names.push(typists[who].name); }
    });
    if (!names.length) { return ""; }
    if (names.length === 1) { return names[0] + " is typing…"; }
    if (names.length === 2) { return names[0] + " and " + names[1] + " are typing…"; }
    return "Several people are typing…";
  }

  var shownWords = "";
  function showTyping() {
    var words = typingWords();
    if (typingLine) { typingLine.textContent = words; }
    if (typingNews && words !== shownWords) { typingNews.textContent = words; }
    shownWords = words;
  }
  setInterval(showTyping, 1000);

  // Typing is told to the room at most every three seconds, and never kept.
  var toldAt = 0;
  if (box) {
    box.addEventListener("input", function () {
      var now = Date.now();
      if (!live.online || !box.value.trim() || now - toldAt < 3000) { return; }
      toldAt = now;
      call("typing", { thread: thread }).catch(function () {});
    });
  }

  // ---- commands --------------------------------------------------------------------------------------------------------

  var counter = 0;
  function newID() {
    counter += 1;
    var random = window.crypto && crypto.getRandomValues ? crypto.getRandomValues(new Uint32Array(2)).join("") : String(Math.random()).slice(2);
    return "c" + Date.now().toString(36) + "-" + counter + "-" + random;
  }

  // call sends a command over the socket and gives the server's answer. A command is sent with an id of its own, so that one sent
  // again after the answer was lost is done once; the caller keeps the id for as long as it may resend.
  function call(method, body, id, retry) {
    var command = Object.assign({ v: 1, id: id || newID(), room: room }, body);
    if (retry) { command.retry = true; }
    return client.rpc(method, command).then(function (reply) { return reply.data; });
  }

  // What was sent and not yet answered is kept with its id, so pressing send again for the same words is a resend.
  var pending = null;

  if (box) {
    form.addEventListener("submit", function (event) {
      var text = box.value.trim();
      if (!live.online || !text) { return; }
      event.preventDefault();
      if (send && send.disabled) { return; }
      if (send) { send.disabled = true; }
      var resend = pending !== null && pending.text === text;
      if (!resend) { pending = { id: newID(), text: text }; }
      call("say", { thread: thread, text: text }, pending.id, resend).then(function (result) {
        if (!result || !result.ok) { throw new Error((result && result.error) || "not sent"); }
        pending = null;
        form.reset();
        box.dispatchEvent(new Event("input"));
        if (typingLine) { typingLine.textContent = typingWords(); }
        said.scrollTop = said.scrollHeight;
      }).catch(function (why) {
        // Not heard. The words stay in the box, and the next press sends them again under the same id.
        if (send) { send.disabled = false; }
        if (typingNews) { typingNews.textContent = "That was not sent: " + (why && why.message ? why.message : "the connection dropped") + ". Press send to try again."; }
      });
    });
  }

  turns.addEventListener("submit", function (event) {
    var match = /\/jobs\/([^/]+)\/stop$/.exec(event.target.getAttribute("action") || "");
    if (!match || !live.online) { return; }
    event.preventDefault();
    call("stop", { ref: match[1] }).then(function (result) {
      if (!result || !result.ok) { event.target.submit(); }
    }).catch(function () { event.target.submit(); });
  });

  // What you have read is told to the server once the page is showing, not for every update.
  var seenTimer = null;
  function markSeen() {
    if (document.hidden || seenTimer) { return; }
    seenTimer = setTimeout(function () {
      seenTimer = null;
      if (live.online && !document.hidden) { call("seen", { thread: thread, at: Math.floor(Date.now() / 1000) }).catch(function () {}); }
    }, 1000);
  }
  document.addEventListener("visibilitychange", function () { if (!document.hidden) { markSeen(); } });

  window.addEventListener("pagehide", function () { client.disconnect(); });
})();
