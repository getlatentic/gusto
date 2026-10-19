// A file of code, read: colored by what it is written in, with its lines numbered. The text is on the page already and is what a reader
// without scripts sees; this only dresses it.
(function () {
  if (!window.hljs) { return; }
  document.querySelectorAll("pre.code:not(.numbered) code").forEach(dress);

  function dress(block) {
    var language = block.parentNode.dataset.language;
    var text = block.textContent;
    var html = (language && hljs.getLanguage(language) ? hljs.highlight(text, { language: language, ignoreIllegals: true }).value : escapeText(text)).replace(/\n$/, "");

    // A colored stretch can run over several lines, so each line closes what it has open and the next one opens it again.
    var open = [];
    var lines = html.split("\n").map(function (line) {
      var before = open.join("");
      line.replace(/<(\/?)span([^>]*)>/g, function (tag, closing) { if (closing) { open.pop(); } else { open.push(tag); } return tag; });
      return '<span class="ln">' + before + line + "</span>".repeat(open.length) + "</span>";
    });
    block.innerHTML = lines.join("\n");
    block.parentNode.classList.add("numbered");

  }

  function escapeText(raw) { return raw.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;"); }
})();
