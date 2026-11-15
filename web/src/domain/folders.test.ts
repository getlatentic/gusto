import { describe, expect, it } from "vitest"

import {
  chatHref,
  chosenPlace,
  emptyWords,
  explore,
  fileKindOf,
  filesHref,
  matching,
  nameAt,
  parentOf,
  startExplorer,
  step,
  trailOf,
  viewerHref,
  type ShownFolders,
} from "@/domain/folders"

const shown: ShownFolders = {
  at: "p1/docs",
  crumbs: [
    { name: "Your cloud computer", at: "" },
    { name: "Project", at: "p1" },
    { name: "docs", at: "p1/docs" },
  ],
  folders: [
    { name: "Drafts", at: "p1/docs/Drafts", modified: 1 },
    { name: "Final", at: "p1/docs/Final", modified: 2 },
  ],
  files: [{ name: "notes.md", size: 10, modified: 3 }],
}

describe("the window that chooses a folder", () => {
  it("remembers where the person was, and goes back there", () => {
    let state = startExplorer("")
    state = explore(state, { type: "open", at: "p1" })
    state = explore(state, { type: "select", at: "p1/x" })
    state = explore(state, { type: "open", at: "p1/docs" })
    expect(state).toMatchObject({
      at: "p1/docs",
      back: ["", "p1"],
      selected: "",
    })
    state = explore(state, { type: "back" })
    expect(state).toMatchObject({ at: "p1", back: [""] })
  })

  it("chooses the folder selected, or the one open", () => {
    const open = startExplorer("p1")
    expect(chosenPlace(open)).toBe("p1")
    expect(chosenPlace(explore(open, { type: "select", at: "p1/a" }))).toBe(
      "p1/a"
    )
  })

  it("moves the selection within the folders that match, and names places", () => {
    expect(step(shown, "", "", 1)).toBe("p1/docs/Drafts")
    expect(step(shown, "", "p1/docs/Drafts", 1)).toBe("p1/docs/Final")
    expect(step(shown, "", "p1/docs/Final", 1)).toBe("p1/docs/Final")
    expect(step(shown, "fin", "", 1)).toBe("p1/docs/Final")
    expect(matching(shown.files, "NOTES").length).toBe(1)
    expect(parentOf(shown)).toBe("p1")
    expect(nameAt(shown, "p1/docs/Final")).toBe("Final")
    expect(nameAt(shown, "p1/docs")).toBe("docs")
  })

  it("says why a list is empty", () => {
    const empty = { ...shown, folders: [], files: [] }
    expect(emptyWords(empty, "x")).toBe("Nothing here matches.")
    expect(emptyWords(empty, "")).toBe("This folder is empty.")
    expect(emptyWords({ ...empty, at: "" }, "")).toMatch(/No projects yet/)
  })
})

describe("the Files page", () => {
  it("is at its own address, and opens a file with the way back to it", () => {
    expect(filesHref("")).toBe("/files")
    expect(filesHref("p1/docs")).toBe("/files?at=p1%2Fdocs")
    expect(viewerHref("p1/docs", "notes.md")).toBe(
      "/instances/p1/file?path=docs%2Fnotes.md&from=%2Ffiles%3Fat%3Dp1%252Fdocs"
    )
    expect(viewerHref("p1", "a b.md")).toBe(
      "/instances/p1/file?path=a%20b.md&from=%2Ffiles%3Fat%3Dp1"
    )
    expect(chatHref("p1/docs")).toBe("/chat?at=p1%2Fdocs")
  })

  it("names the top Files, and links each place above the one open", () => {
    expect(trailOf(shown)).toEqual({
      crumbs: [
        { label: "Files", href: "/files" },
        { label: "Project", href: "/files?at=p1" },
      ],
      title: "docs",
    })
    expect(trailOf({ ...shown, at: "", crumbs: [shown.crumbs[0]] })).toEqual({
      crumbs: [],
      title: "Files",
    })
  })

  it("marks a file by what its name says it is", () => {
    expect(fileKindOf("a.PNG")).toBe("image")
    expect(fileKindOf("a.csv")).toBe("table")
    expect(fileKindOf("main.go")).toBe("code")
    expect(fileKindOf("README")).toBe("file")
  })
})
