import { fireEvent, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { draw, records } from "@/pages/admin/records/fixture"
import type { ActivityRow } from "@/pages/admin/records/types"

const said = (
  Said: string,
  change: Partial<ActivityRow> = {}
): ActivityRow => ({
  At: "Today 10:00",
  Full: "15 Nov 2026 10:00:00 UTC",
  Who: "ada@acme.com",
  Said,
  From: "",
  Times: 1,
  Since: "",
  Kind: "setup.created",
  ...change,
})

describe("the activity tab", () => {
  it("lists what people did, newest first, with who and from where", async () => {
    draw(
      records("activity", (page) => {
        page.Activity = {
          Rows: [
            said("created skill set reviewer", { From: "203.0.113.9" }),
            said("signed in", {
              Who: "the shared sign-in",
              Times: 3,
              Since: "14 Nov 2026 09:00:00 UTC",
            }),
          ],
          LeftOut: 0,
          WithServer: false,
        }
      })
    )
    const first = (await screen.findByText("created skill set reviewer"))
      .parentElement!
    expect(first.textContent).toBe(
      "created skill set reviewerToday 10:00 · ada@acme.com · 203.0.113.9"
    )
    expect(first.querySelector("time")!.getAttribute("title")).toBe(
      "15 Nov 2026 10:00:00 UTC"
    )
    expect(screen.getByText("203.0.113.9").className).toContain("font-mono")
    const folded = screen.getByText("×3")
    expect(folded.getAttribute("title")).toBe("since 14 Nov 2026 09:00:00 UTC")
    const shared = folded.closest("div")!
    expect(shared.textContent).not.toContain("the shared sign-in")
  })

  it("says nothing has been recorded yet", async () => {
    draw(
      records("activity", (page) => {
        page.Activity = { Rows: null, LeftOut: 0, WithServer: false }
      })
    )
    expect(
      await screen.findByText("Nothing has been recorded yet.")
    ).toBeTruthy()
    expect(screen.queryByText(/the server started/)).toBeNull()
  })

  it("counts the server starting it left out, and shows it when asked", async () => {
    const { unmount } = draw(
      records("activity", (page) => {
        page.Activity = { Rows: null, LeftOut: 2, WithServer: false }
      })
    )
    expect(
      (await screen.findByText(/2 times the server started are left out\./))
        .textContent
    ).toBe("2 times the server started are left out. Show them")
    expect(
      screen.getByRole("link", { name: "Show them" }).getAttribute("href")
    ).toBe("/admin/records?tab=activity&server=1")
    unmount()

    const once = draw(
      records("activity", (page) => {
        page.Activity = { Rows: null, LeftOut: 1, WithServer: false }
      })
    )
    expect(
      (await screen.findByText(/1 time the server started is left out\./))
        .textContent
    ).toBe("1 time the server started is left out. Show it")
    once.unmount()

    draw(
      records("activity", (page) => {
        page.Activity = { Rows: null, LeftOut: 0, WithServer: true }
      }),
      "/admin/records?tab=activity&server=1"
    )
    expect(
      (
        await screen.findByRole("link", { name: "Show only what people did" })
      ).getAttribute("href")
    ).toBe("/admin/records?tab=activity")
    expect(screen.getByText(/Showing the server starting too\./)).toBeTruthy()
  })

  it("says how to check the record, with the head to keep", async () => {
    draw(
      records("activity", (page) => {
        page.Activity = { Rows: null, LeftOut: 0, WithServer: false }
      })
    )
    fireEvent.click(await screen.findByText("How to check it yourself"))
    expect(
      await screen.findByText(
        /Each line names the one before it, so an edit shows, and the chain holds\./
      )
    ).toBeTruthy()
    expect(
      screen.getByText("gusto changes verify -head 7:abc123 changes.jsonl")
    ).toBeTruthy()
  })
})
