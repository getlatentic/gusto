// A file of code, read: colored by what it is written in, with its lines numbered. The text is on the page already and is what a reader
// without scripts sees; this only dresses it.
(function () {
  if (!window.hljs) { return; }
  var lineClass = "before:mr-5 before:inline-block before:w-[3.25em] before:text-right before:text-subtle before:opacity-70 before:select-none before:content-[counter(line)] before:[counter-increment:line]";
  // What highlight.js writes is spans named by kind, so the block says how each kind looks, in the colors of the theme.
  var blockClass = "[counter-reset:line] [&_.hljs-comment]:italic [&_.hljs-comment]:text-subtle [&_.hljs-quote]:italic [&_.hljs-quote]:text-subtle [&_.hljs-keyword]:text-info [&_.hljs-selector-tag]:text-info [&_.hljs-literal]:text-info [&_.hljs-doctag]:text-info [&_.hljs-string]:text-ok [&_.hljs-regexp]:text-ok [&_.hljs-addition]:text-ok [&_.hljs-number]:text-warn [&_.hljs-symbol]:text-warn [&_.hljs-bullet]:text-warn [&_.hljs-title]:font-semibold [&_.hljs-title]:text-ink [&_.hljs-section]:font-semibold [&_.hljs-name]:font-semibold [&_.hljs-attr]:text-muted [&_.hljs-attribute]:text-muted [&_.hljs-built\\_in]:text-muted [&_.hljs-type]:text-muted [&_.hljs-variable]:text-muted [&_.hljs-deletion]:text-bad";
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
      return '<span class="' + lineClass + '">' + before + line + "</span>".repeat(open.length) + "</span>";
    });
    block.innerHTML = lines.join("\n");
    block.className = blockClass;
    block.parentNode.classList.add("numbered");

  }

  function escapeText(raw) { return raw.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;"); }
})();
