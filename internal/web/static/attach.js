// Files attached to a room's message. The + in the message box picks them, each is listed above the words with a way to take it off, and
// they are sent with the message by the form, so the file and the message about it arrive together.
(function () {
  var input = document.querySelector("[data-attach]");
  var list = document.getElementById("attached");
  if (!input || !list || !window.DataTransfer) { return; }
  var form = input.form;
  var box = form.querySelector("textarea[name=said]");
  var unsent = document.getElementById("unsent");
  var most = Number(input.dataset.most) || 20;
  var biggest = Number(input.dataset.bytes) || 64 * 1024 * 1024;
  var chosen = [];

  function same(a, b) { return a.name === b.name && a.size === b.size && a.lastModified === b.lastModified; }

  function size(bytes) {
    if (bytes >= 1 << 20) { return (bytes / (1 << 20)).toFixed(1) + " MB"; }
    if (bytes >= 1 << 10) { return Math.round(bytes / (1 << 10)) + " KB"; }
    return bytes === 1 ? "1 byte" : bytes + " bytes";
  }

  function say(words) {
    if (!unsent) { return; }
    unsent.textContent = words;
    unsent.hidden = !words;
    if (words) { unsent.setAttribute("role", "alert"); }
  }

  function chip(file, index) {
    var item = document.createElement("li");
    item.className = "flex max-w-full items-center gap-1.5 rounded-full bg-sunken py-1 pr-1 pl-3 text-xs text-ink";
    var name = document.createElement("span");
    name.className = "min-w-0 truncate";
    name.textContent = file.name;
    name.title = file.name;
    var how = document.createElement("span");
    how.className = "flex-none text-subtle tabular-nums";
    how.textContent = size(file.size);
    var drop = document.createElement("button");
    drop.type = "button";
    drop.className = "grid size-5 flex-none cursor-pointer place-items-center rounded-full border-0 bg-transparent p-0 text-muted hover:bg-hover hover:text-ink";
    drop.setAttribute("aria-label", "Remove " + file.name);
    drop.innerHTML = '<svg class="i size-3" viewBox="0 0 24 24" aria-hidden="true"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>';
    drop.addEventListener("click", function () {
      chosen.splice(index, 1);
      sync();
      input.focus();
    });
    item.append(name, how, drop);
    return item;
  }

  // The input is what the form sends, so it is made to hold exactly what is listed.
  function sync() {
    var held = new DataTransfer();
    chosen.forEach(function (file) { held.items.add(file); });
    input.files = held.files;
    list.textContent = "";
    chosen.forEach(function (file, index) { list.appendChild(chip(file, index)); });
    list.hidden = chosen.length === 0;
    if (box) { box.dispatchEvent(new Event("input")); }
  }

  input.addEventListener("change", function () {
    var refused = "";
    Array.prototype.forEach.call(input.files, function (file) {
      if (chosen.some(function (each) { return same(each, file); })) { return; }
      if (file.size > biggest) { refused = file.name + " is more than " + size(biggest) + ", so it was not attached."; return; }
      if (file.name.indexOf("@") >= 0) { refused = file.name + " has an @ in its name, which the room would read as a mention. Rename it to attach it."; return; }
      if (chosen.length >= most) { refused = "A message takes at most " + most + " files."; return; }
      chosen.push(file);
    });
    say(refused);
    sync();
  });

  form.addEventListener("reset", function () {
    chosen = [];
    setTimeout(sync, 0);
  });
})();
