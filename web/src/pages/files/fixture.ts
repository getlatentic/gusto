import { vi } from "vitest"

import type { ShownFolders } from "@/domain/folders"
import type { FileData, ViewedFile } from "@/pages/files/types"

export const top: ShownFolders = {
  at: "",
  crumbs: [{ name: "Your cloud computer", at: "" }],
  folders: [{ name: "Launch", at: "p1", modified: 1 }],
  files: [],
}

export const docs: ShownFolders = {
  at: "p1/docs",
  crumbs: [
    { name: "Your cloud computer", at: "" },
    { name: "Launch", at: "p1" },
    { name: "docs", at: "p1/docs" },
  ],
  folders: [{ name: "Drafts", at: "p1/docs/Drafts", modified: 1 }],
  files: [
    { name: "plan.md", size: 2048, modified: 2 },
    { name: "chart.png", size: 10, modified: 3 },
  ],
}

export function fileOf(change: Partial<ViewedFile> = {}): ViewedFile {
  return {
    Name: "plan.md",
    Path: "docs/plan.md",
    Dir: "docs",
    Kind: "markdown",
    Size: "12 bytes",
    Modified: 0,
    Raw: "/instances/p1/raw?path=docs%2Fplan.md",
    Download: "/instances/p1/raw?path=docs%2Fplan.md&download=1",
    Text: "# Plan",
    Editable: false,
    Source: "",
    Hash: "",
    Language: "",
    Lines: 1,
    Cut: false,
    Rows: null,
    Parts: null,
    ...change,
  }
}

export function pageOf(
  file: Partial<ViewedFile> = {},
  change: Partial<FileData> = {}
): FileData {
  return {
    File: fileOf(file),
    Crumbs: [
      { label: "Files", href: "/files" },
      { label: "Launch", href: "/files?at=p1" },
      { label: "docs", href: "/files?at=p1%2Fdocs" },
    ],
    Prose: "<h1>Plan</h1>",
    Back: "",
    BackWord: "",
    Save: "",
    Source: "",
    ...change,
  }
}

// answerPosts answers every POST with the status and body given, a string as it is and anything else as JSON, and every other request as it was answered before; it gives the
// mock, whose calls are what was sent.
export function answerPosts(status: number, body: unknown) {
  const before = globalThis.fetch
  const sent = vi.fn(async (path: string, init?: RequestInit) =>
    init?.method === "POST"
      ? new Response(typeof body === "string" ? body : JSON.stringify(body), {
          status,
          headers: { "Content-Type": "application/json" },
        })
      : before(path, init)
  )
  vi.stubGlobal("fetch", sent)
  return sent
}

// postedTo is the path and fields of the last POST sent.
export function postedTo(sent: ReturnType<typeof answerPosts>) {
  const post = sent.mock.calls
    .filter(([, init]) => init?.method === "POST")
    .at(-1)
  return post
    ? { path: post[0], fields: new URLSearchParams(String(post[1]?.body)) }
    : undefined
}
