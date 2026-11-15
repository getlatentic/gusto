import { fireEvent, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { answerPosts, pageOf } from "@/pages/files/fixture"
import { PanelFile } from "@/pages/files/panel-file"
import { renderPage } from "@/test/render"

const href =
  "/instances/p1/file?path=docs%2Fplan.md&from=%2Finstances%2Fp1%2Fthreads%2F1"

function draw(data: Record<string, unknown>) {
  return renderPage(
    "/instances/p1/threads/1",
    "/instances/:id/threads/:thread",
    <PanelFile href={href} />,
    data
  )
}

describe("a file beside a conversation", () => {
  it("is drawn in the panel, without the way back, which the conversation is", async () => {
    const { asked } = draw({
      [href]: pageOf({}, { Back: "/instances/p1/threads/1" }),
    })
    const panel = await screen.findByTestId("panel-file")
    await within(panel).findByTestId("file-body")
    expect(within(panel).getByRole("heading", { level: 2 }).textContent).toBe(
      "plan.md"
    )
    expect(
      within(panel)
        .getAllByRole("link")
        .map((link) => link.textContent)
        .slice(0, 3)
    ).toEqual(["Files", "Launch", "docs"])
    expect(within(panel).queryByTestId("file-back")).toBeNull()
    expect(panel.querySelector("iframe")).toBeNull()
    expect(document.title).not.toContain("plan.md")
    expect(asked).toEqual([`/app/api${href}`])
  })

  it("changes a file in place", async () => {
    draw({
      [href]: pageOf(
        { Editable: true, Source: "draft", Hash: "h1" },
        { Save: "/instances/p1/file?path=docs%2Fplan.md" }
      ),
    })
    fireEvent.click(await screen.findByRole("button", { name: "Edit" }))
    expect(
      (
        screen.getByRole("textbox", {
          name: "The text of plan.md",
        }) as HTMLTextAreaElement
      ).value
    ).toBe("draft")
    answerPosts(200, { hash: "h2" })
    fireEvent.click(screen.getByRole("button", { name: "Save" }))
    await screen.findByTestId("file-body")
  })

  it("shows a web page's source in its place", async () => {
    const page = { Name: "game.html", Kind: "html" as const }
    draw({
      [href]: pageOf(page, {
        Source: "/instances/p1/file?path=game.html&source=1",
      }),
      [`${href}&source=1`]: pageOf({
        ...page,
        Kind: "text",
        Text: "<h1>x</h1>",
        Language: "xml",
      }),
    })
    fireEvent.click(await screen.findByRole("link", { name: "View source" }))
    await expect
      .poll(() => screen.queryByTestId("code")?.textContent)
      .toBe("<h1>x</h1>")
  })
})
