import { Suspense, use } from "react"
import type { HLJSApi } from "highlight.js"

import { escapeText, numberedLines } from "@/domain/file-view"

// highlight.js is read the first time a page shows code, so no other page waits for it.
let highlighter: Promise<HLJSApi> | undefined
const loadHighlighter = () =>
  (highlighter ??= import("highlight.js/lib/common").then((m) => m.default))

// What highlight.js writes is spans named by kind, so the block says how each kind looks, in the colors of the theme; each line is
// numbered by a counter, so a copy of the code carries no numbers.
const block =
  "overflow-x-auto rounded-md bg-muted px-5 py-4 font-mono text-sm leading-relaxed whitespace-pre [tab-size:4] " +
  "[&_code]:[counter-reset:line] [&_.ln]:before:mr-5 [&_.ln]:before:inline-block [&_.ln]:before:w-[3.25em] [&_.ln]:before:text-right [&_.ln]:before:text-subtle [&_.ln]:before:opacity-70 [&_.ln]:before:select-none [&_.ln]:before:content-[counter(line)] [&_.ln]:before:[counter-increment:line] " +
  "[&_:is(.hljs-comment,.hljs-quote)]:text-subtle [&_:is(.hljs-comment,.hljs-quote)]:italic [&_:is(.hljs-keyword,.hljs-selector-tag,.hljs-literal,.hljs-doctag)]:text-primary " +
  "[&_:is(.hljs-string,.hljs-regexp,.hljs-addition)]:text-ok [&_:is(.hljs-number,.hljs-symbol,.hljs-bullet)]:text-warn [&_:is(.hljs-title,.hljs-section,.hljs-name)]:font-semibold " +
  "[&_:is(.hljs-attr,.hljs-attribute,.hljs-built_in,.hljs-type,.hljs-variable)]:text-muted-foreground [&_.hljs-deletion]:text-bad"

// CodeView is a file read as text: colored by what it is written in, with its lines numbered. Until the colors are read it is the
// text alone, numbered.
export function CodeView({
  text,
  language,
}: {
  text: string
  language: string
}) {
  return (
    <pre className={block} data-language={language || undefined}>
      {language ? (
        <Suspense fallback={<Lines html={escapeText(text)} />}>
          <Highlighted text={text} language={language} />
        </Suspense>
      ) : (
        <Lines html={escapeText(text)} />
      )}
    </pre>
  )
}

function Highlighted({ text, language }: { text: string; language: string }) {
  const hljs = use(loadHighlighter())
  const html = hljs.getLanguage(language)
    ? hljs.highlight(text, { language, ignoreIllegals: true }).value
    : escapeText(text)
  return <Lines html={html} />
}

// Lines is code drawn as HTML, which highlight.js and escapeText have made safe, one numbered span a line.
function Lines({ html }: { html: string }) {
  return (
    <code
      data-testid="code"
      dangerouslySetInnerHTML={{
        __html: numberedLines(html)
          .map((line) => `<span class="ln">${line}</span>`)
          .join("\n"),
      }}
    />
  )
}
