import { fireEvent, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { FilePage } from "@/pages/files/file-page"
import { answerPosts, pageOf, postedTo } from "@/pages/files/fixture"
import type { FileData } from "@/pages/files/types"
import { renderPage } from "@/test/render"

const at = "/instances/p1/file?path=docs%2Fplan.md"

function draw(page: FileData, address = at) {
  return renderPage(address, "/instances/:id/file", <FilePage />, {
    [address]: page,
  })
}

const body = () => screen.findByTestId("file-body")

describe("a file's page", () => {
  it("is named for the file, under the places it is in, with what it is", async () => {
    draw(pageOf({ Size: "12 bytes", Lines: 1 }))
    await body()
    expect(screen.getByTestId("page-title").textContent).toBe("plan.md")
    const trail = screen.getByRole("navigation", { name: "Where this is" })
    expect(
      within(trail)
        .getAllByRole("link")
        .map((link) => [link.textContent, link.getAttribute("href")])
    ).toEqual([
      ["Files", "/files"],
      ["Launch", "/files?at=p1"],
      ["docs", "/files?at=p1%2Fdocs"],
    ])
    expect(screen.getByTestId("file-facts").textContent).toBe(
      "12 bytes · 1 line"
    )
    const download = screen.getByTestId("file-download")
    expect(download.getAttribute("href")).toBe(
      "/instances/p1/raw?path=docs%2Fplan.md&download=1"
    )
    expect(download.hasAttribute("download")).toBe(true)
    expect(screen.queryByTestId("file-back")).toBeNull()
  })

  it("draws Markdown as the server drew it", async () => {
    draw(pageOf({}, { Prose: "<blockquote>quoted</blockquote>" }))
    const article = (await body()).querySelector("article.prose")
    expect(article?.innerHTML).toBe("<blockquote>quoted</blockquote>")
  })

  it("draws code colored by its language, one numbered line each", async () => {
    draw(
      pageOf({
        Name: "main.go",
        Kind: "text",
        Text: "package main\n\nfunc main() {}\n",
        Language: "go",
        Lines: 3,
      })
    )
    const pre = (await body()).querySelector("pre")
    expect(pre?.dataset.language).toBe("go")
    await expect
      .poll(() => pre?.querySelector(".hljs-keyword")?.textContent)
      .toBe("package")
    expect(pre?.querySelectorAll(".ln")).toHaveLength(3)
    expect(screen.getByTestId("code").textContent).toBe(
      "package main\n\nfunc main() {}"
    )
  })

  it("draws a table with its first row as the heading", async () => {
    draw(
      pageOf({
        Name: "data.csv",
        Kind: "table",
        Rows: [
          ["name", "total"],
          ["ada", "3"],
        ],
      })
    )
    await body()
    expect(
      screen.getAllByRole("columnheader").map((head) => head.textContent)
    ).toEqual(["name", "total"])
    expect(screen.getAllByRole("cell").map((cell) => cell.textContent)).toEqual(
      ["ada", "3"]
    )
  })

  it("shows a picture, a PDF and a sealed web page from the file itself", async () => {
    const raw = "/instances/p1/raw?path=x"
    const { unmount } = draw(
      pageOf({ Name: "pic.png", Kind: "image", Raw: raw })
    )
    expect(
      (await screen.findByRole("img", { name: "pic.png" })).getAttribute("src")
    ).toBe(raw)
    unmount()

    const frameOf = async () =>
      (await body()).querySelector("iframe") as HTMLIFrameElement
    const pdf = draw(pageOf({ Name: "paper.pdf", Kind: "pdf", Raw: raw }))
    const paper = await frameOf()
    expect([paper.title, paper.getAttribute("src")]).toEqual(["paper.pdf", raw])
    expect(paper.hasAttribute("sandbox")).toBe(false)
    pdf.unmount()

    draw(
      pageOf(
        { Name: "game.html", Kind: "html", Raw: raw },
        { Source: "/instances/p1/file?path=game.html&source=1" }
      )
    )
    const frame = await frameOf()
    expect(frame.title).toBe("game.html")
    expect(frame.getAttribute("sandbox")).toBe(
      "allow-scripts allow-forms allow-modals"
    )
    expect(
      screen.getByRole("link", { name: "View source" }).getAttribute("href")
    ).toBe("/instances/p1/file?path=game.html&source=1")
  })

  it("reads a document for its words, and says that is not how it looks", async () => {
    draw(
      pageOf({
        Name: "plan.docx",
        Kind: "office",
        Parts: [
          {
            Title: "",
            Lines: [
              { Text: "The plan", Heading: 1 },
              { Text: "First we begin.", Heading: 0 },
            ],
            Rows: null,
          },
          { Title: "Sheet 1", Lines: null, Rows: [["Name", "Total"]] },
        ],
      })
    )
    await body()
    expect(
      screen.getByRole("heading", { level: 3, name: "The plan" })
    ).toBeTruthy()
    expect(screen.getByText("First we begin.").tagName).toBe("P")
    expect(
      screen.getByText(/This is the text of the file, not how it looks/)
    ).toBeTruthy()
    const sheet = screen.getByRole("region", { name: "Sheet 1" })
    expect(
      within(sheet)
        .getAllByRole("cell")
        .map((c) => c.textContent)
    ).toEqual(["Name", "Total"])
  })

  it("offers a file it cannot show as a download, and says when one is cut short", async () => {
    draw(pageOf({ Name: "blob.bin", Kind: "binary", Cut: true }))
    await body()
    expect(
      screen.getByText("This kind of file cannot be shown here.")
    ).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "Download it" }).getAttribute("href")
    ).toBe("/instances/p1/raw?path=docs%2Fplan.md&download=1")
    expect(
      screen.getByText(
        "The file is longer than the page draws. Download it to read the rest."
      )
    ).toBeTruthy()
  })

  it("leads back the way the person came", async () => {
    draw(pageOf({}, { Back: "/files?at=p1" }))
    const back = await screen.findByTestId("file-back")
    expect(back.getAttribute("href")).toBe("/files?at=p1")
    expect(back.textContent).toBe("Back to the chat")
  })

  it("is a run's file under its record, with the way back to it", async () => {
    const address = "/jobs/j1/files/transcript.log"
    renderPage(address, "/jobs/:id/files/:name", <FilePage />, {
      [address]: pageOf(
        { Name: "transcript.log", Kind: "text", Text: "ran" },
        {
          Crumbs: [
            { label: "Activity", href: "/jobs" },
            { label: "Details", href: "/jobs/j1" },
          ],
          Back: "/jobs/j1",
          BackWord: "Back to the record",
        }
      ),
    })
    const back = await screen.findByTestId("file-back")
    expect(back.textContent).toBe("Back to the record")
    expect(back.getAttribute("href")).toBe("/jobs/j1")
    expect(
      within(screen.getByRole("navigation", { name: "Where this is" }))
        .getAllByRole("link")
        .map((link) => link.textContent)
    ).toEqual(["Activity", "Details"])
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull()
  })
})

describe("changing a file", () => {
  const editable = () =>
    pageOf(
      { Editable: true, Source: "\n# Plan <b>\n", Hash: "h1" },
      { Save: "/instances/p1/file?path=docs%2Fplan.md" }
    )

  it("opens all of it to be changed, with what it was, and leaves it on Cancel", async () => {
    draw(editable())
    fireEvent.click(await screen.findByRole("button", { name: "Edit" }))
    const text = screen.getByRole("textbox", {
      name: "The text of plan.md",
    }) as HTMLTextAreaElement
    expect(text.value).toBe("\n# Plan <b>\n")
    expect(text.name).toBe("text")
    expect(text.getAttribute("spellcheck")).toBe("false")
    const form = screen.getByTestId("file-edit") as HTMLFormElement
    expect(form.getAttribute("action")).toBe(
      "/instances/p1/file?path=docs%2Fplan.md"
    )
    expect((form.elements.namedItem("seen") as HTMLInputElement).value).toBe(
      "h1"
    )
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull()
    expect(screen.queryByTestId("file-body")).toBeNull()

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }))
    expect(screen.queryByTestId("file-edit")).toBeNull()
    expect(screen.getByTestId("file-body")).toBeTruthy()
  })

  it("sends what was written and what the file was, and shows the file again once it is saved", async () => {
    draw(editable())
    fireEvent.click(await screen.findByRole("button", { name: "Edit" }))
    const sent = answerPosts(200, { hash: "h2" })
    fireEvent.change(screen.getByTestId("file-text"), {
      target: { value: "# New plan" },
    })
    fireEvent.click(screen.getByRole("button", { name: "Save" }))
    await screen.findByTestId("file-body")
    const post = postedTo(sent)
    expect(post?.path).toBe("/instances/p1/file?path=docs%2Fplan.md")
    expect(Object.fromEntries(post!.fields)).toEqual({
      seen: "h1",
      text: "# New plan",
    })
  })

  it("says why a save was refused, and keeps what was written", async () => {
    draw(editable())
    fireEvent.click(await screen.findByRole("button", { name: "Edit" }))
    answerPosts(409, {
      error: "somebody changed this file after you opened it.",
    })
    fireEvent.change(screen.getByTestId("file-text"), {
      target: { value: "mine" },
    })
    fireEvent.click(screen.getByRole("button", { name: "Save" }))
    expect((await screen.findByRole("alert")).textContent).toBe(
      "It was not saved: somebody changed this file after you opened it."
    )
    expect((screen.getByTestId("file-text") as HTMLTextAreaElement).value).toBe(
      "mine"
    )
  })

  it("is not offered for a file that cannot be changed here", async () => {
    draw(pageOf())
    await body()
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull()
  })
})
