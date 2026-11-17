import ts from "typescript"
import { afterEach, describe, expect, it, vi } from "vitest"

import { below, withBase } from "@/lib/base"
import { sendForm } from "@/lib/forms"

function underBase(base: string) {
  const meta = document.createElement("meta")
  meta.name = "gusto-base"
  meta.content = base
  document.head.appendChild(meta)
}

afterEach(() => {
  document.head.querySelector('meta[name="gusto-base"]')?.remove()
  vi.unstubAllGlobals()
})

describe("the path the server's pages are under", () => {
  it("is nothing at the root of a host", () => {
    expect(withBase("/chat")).toBe("/chat")
    expect(below("/chat")).toBe("/chat")
  })

  it("begins each of the server's own paths, and leaves any other address as it was", () => {
    underBase("/workspace/acme")
    expect(withBase("/chat?x=1")).toBe("/workspace/acme/chat?x=1")
    for (const elsewhere of [
      "https://example.com/x",
      "//example.com/x",
      "#apps",
      "mailto:a@b.c",
      "files",
    ])
      expect(withBase(elsewhere)).toBe(elsewhere)
    expect(withBase(undefined)).toBeUndefined()
  })

  it("is taken off a path the browser is at, and only off a path under it", () => {
    underBase("/workspace/acme")
    expect(below("/workspace/acme/jobs/1")).toBe("/jobs/1")
    expect(below("/workspace/acme")).toBe("/")
    expect(below("/workspace/acmeco/jobs/1")).toBe("/workspace/acmeco/jobs/1")
  })

  it("is taken off where a form sent the person, which the app then moves to itself", async () => {
    underBase("/workspace/acme")
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Object.defineProperty(new Response("page"), "url", {
          value: `${location.origin}/workspace/acme/jobs/1?x=1`,
        })
      )
    )
    document.body.innerHTML = `<form method="post" action="/workspace/acme/jobs"></form>`
    expect(await sendForm(document.querySelector("form")!)).toBe("/jobs/1?x=1")
  })
})

// The pages the server draws for an organization (everything but gusto cloud's own, in src/cloud) are under the base path when
// gusto cloud serves the organization at one. A link the router follows adds it itself; an address a built-in element gives the
// browser does not, so each goes through withBase.
describe("every address a built-in element gives the browser", () => {
  const attributes = new Set(["href", "action", "src", "formAction"])

  // Every page's source, by its path, as the bundler reads it.
  const sources = import.meta.glob<string>("/src/**/*.tsx", {
    query: "?raw",
    import: "default",
    eager: true,
  })
  const pages = Object.entries(sources).filter(
    ([path]) => !path.startsWith("/src/cloud/") && !path.endsWith(".test.tsx")
  )

  function unbased([file, text]: [string, string]): string[] {
    const source = ts.createSourceFile(
      file,
      text,
      ts.ScriptTarget.Latest,
      true,
      ts.ScriptKind.TSX
    )
    const found: string[] = []
    const visit = (node: ts.Node) => {
      if (
        ts.isJsxAttribute(node) &&
        attributes.has(node.name.getText(source)) &&
        node.initializer
      ) {
        const element = node.parent.parent
        const builtIn =
          (ts.isJsxOpeningElement(element) ||
            ts.isJsxSelfClosingElement(element)) &&
          /^[a-z]/.test(element.tagName.getText(source))
        const given = node.initializer
        const value = ts.isJsxExpression(given) ? given.expression : given
        const based =
          value !== undefined &&
          ts.isCallExpression(value) &&
          value.expression.getText(source) === "withBase"
        const elsewhere =
          value !== undefined &&
          (ts.isStringLiteral(value) ||
            ts.isNoSubstitutionTemplateLiteral(value)) &&
          !value.text.startsWith("/")
        if (builtIn && !based && !elsewhere) {
          const { line } = source.getLineAndCharacterOfPosition(node.getStart())
          found.push(`${file}:${line + 1}`)
        }
      }
      ts.forEachChild(node, visit)
    }
    visit(source)
    return found
  }

  it("goes through withBase", () => {
    expect(pages.length).toBeGreaterThan(100)
    expect(pages.flatMap(unbased)).toEqual([])
  })
})
