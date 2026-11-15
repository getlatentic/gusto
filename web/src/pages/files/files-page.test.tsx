import { fireEvent, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { answerPosts, docs, postedTo, top } from "@/pages/files/fixture"
import { FilesPage } from "@/pages/files/files-page"
import type { FilesData } from "@/pages/files/types"
import { renderPage } from "@/test/render"

function draw(at: string, page: FilesData) {
  return renderPage(at, "/files", <FilesPage />, { [at]: page })
}

const names = () =>
  within(screen.getByTestId("files-list"))
    .getAllByRole("link")
    .map((link) => [link.textContent, link.getAttribute("href")])

describe("the Files page", () => {
  it("is called Files at the top, lists the person's projects, and makes a project", async () => {
    draw("/files", { Here: top, Back: "" })
    await screen.findByTestId("files-list")
    expect(screen.getByTestId("page-title").textContent).toBe("Files")
    expect(names()).toEqual([["Launch", "/files?at=p1"]])
    expect(screen.getByRole("button", { name: "New project" })).toBeTruthy()
    expect(screen.queryByTestId("chat-here")).toBeNull()
    expect(screen.queryByTestId("files-back")).toBeNull()
  })

  it("is named for the folder open, under the places above it, with a way to chat in it", async () => {
    draw("/files?at=p1%2Fdocs", { Here: docs, Back: "" })
    await screen.findByTestId("files-list")
    expect(screen.getByTestId("page-title").textContent).toBe("docs")
    const trail = screen.getByRole("navigation", { name: "Where this is" })
    expect(
      within(trail)
        .getAllByRole("link")
        .map((link) => [link.textContent, link.getAttribute("href")])
    ).toEqual([
      ["Files", "/files"],
      ["Launch", "/files?at=p1"],
    ])
    expect(screen.getByTestId("chat-here").getAttribute("href")).toBe(
      "/chat?at=p1%2Fdocs"
    )
    expect(screen.getByRole("button", { name: "New folder" })).toBeTruthy()
    expect(names()).toEqual([
      ["Drafts", "/files?at=p1%2Fdocs%2FDrafts"],
      [
        "plan.md",
        "/instances/p1/file?path=docs%2Fplan.md&from=%2Ffiles%3Fat%3Dp1%252Fdocs",
      ],
      [
        "chart.png",
        "/instances/p1/file?path=docs%2Fchart.png&from=%2Ffiles%3Fat%3Dp1%252Fdocs",
      ],
    ])
    expect(screen.getByText("2 KB")).toBeTruthy()
    expect(
      screen.getAllByRole("columnheader").map((head) => head.textContent)
    ).toEqual(["Name", "Modified", "Size"])
  })

  it("filters the folder by name, and says when nothing matches", async () => {
    draw("/files?at=p1%2Fdocs", { Here: docs, Back: "" })
    const filter = await screen.findByRole("searchbox", {
      name: "Filter this folder",
    })
    fireEvent.change(filter, { target: { value: "PLAN" } })
    expect(names().map(([name]) => name)).toEqual(["plan.md"])
    fireEvent.change(filter, { target: { value: "zzz" } })
    expect(screen.getByText("Nothing here matches.")).toBeTruthy()
  })

  it("says when a folder is empty, and when there are no projects", async () => {
    draw("/files?at=p1", {
      Here: { ...docs, at: "p1", folders: [], files: null },
      Back: "",
    })
    expect(await screen.findByText("This folder is empty.")).toBeTruthy()
  })

  it("makes a folder here when its name is given, and says why one was refused", async () => {
    draw("/files?at=p1%2Fdocs", { Here: docs, Back: "" })
    fireEvent.click(await screen.findByRole("button", { name: "New folder" }))
    const name = screen.getByRole("textbox", { name: "Name of the new folder" })
    expect(name.getAttribute("placeholder")).toBe("New folder")
    expect(name.getAttribute("maxlength")).toBe("80")

    const sent = answerPosts(200, { at: "p1/docs/Notes" })
    fireEvent.change(name, { target: { value: "Notes" } })
    fireEvent.keyDown(name, { key: "Enter" })
    await screen.findByTestId("new-folder")
    expect(postedTo(sent)).toMatchObject({ path: "/chat/folders" })
    expect(Object.fromEntries(postedTo(sent)!.fields)).toEqual({
      at: "p1/docs",
      name: "Notes",
      harness: "",
    })
    await expect
      .poll(() =>
        screen.queryByRole("textbox", { name: "Name of the new folder" })
      )
      .toBeNull()

    fireEvent.click(screen.getByRole("button", { name: "New folder" }))
    answerPosts(400, "a folder is named with one plain word or phrase")
    const again = screen.getByRole("textbox", {
      name: "Name of the new folder",
    })
    fireEvent.change(again, { target: { value: "bad/name" } })
    fireEvent.keyDown(again, { key: "Enter" })
    expect(
      (await screen.findByText(/a folder is named with one plain word/)).id
    ).toBe("new-folder-problem")
    expect(again.getAttribute("aria-invalid")).toBe("true")

    fireEvent.keyDown(again, { key: "Escape" })
    expect(
      screen.queryByRole("textbox", { name: "Name of the new folder" })
    ).toBeNull()
  })

  it("leads back to the chat the person came from", async () => {
    draw("/files?at=p1&from=%2Finstances%2Fp1%2Fthreads%2F1", {
      Here: { ...docs, at: "p1", crumbs: docs.crumbs.slice(0, 2) },
      Back: "/instances/p1/threads/1",
    })
    const back = await screen.findByTestId("files-back")
    expect(back.getAttribute("href")).toBe("/instances/p1/threads/1")
    expect(back.textContent).toBe("Back to the chat")
  })

  it("says a folder that is not there is not there", async () => {
    renderPage("/files?at=gone", "/files", <FilesPage />, {})
    expect((await screen.findByRole("alert")).textContent).toBe(
      "There is nothing here."
    )
  })
})
