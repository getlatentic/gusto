import { fireEvent, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import {
  draw,
  everyControlIsNamed,
  records,
} from "@/pages/admin/records/fixture"

const empty = { Rows: null, LeftOut: 0, WithServer: false }

describe("records", () => {
  it("says what the page is, offers its tabs, and says the record is intact", async () => {
    draw(records("activity", (page) => (page.Activity = empty)))
    expect(
      await screen.findByText(
        "A signed record of everything that happened, and how long work is kept"
      )
    ).toBeTruthy()
    const tabs = screen.getAllByRole("tab")
    expect(tabs.map((tab) => tab.textContent)).toEqual([
      "Activity",
      "Task checks",
      "Keeping",
    ])
    expect(tabs.map((tab) => tab.getAttribute("href"))).toEqual([
      "/admin/records?tab=activity",
      "/admin/records?tab=checks",
      "/admin/records?tab=keeping",
    ])
    const health = screen.getByRole("status")
    expect(health.textContent).toBe(
      "Intact: 3 events, all signed, the last task checks out"
    )
    expect(health.querySelector("[title]")!.getAttribute("title")).toBe(
      "3 events, all signed, the last task checks out"
    )
    everyControlIsNamed()
  })

  it("downloads the record from its menu, as a file", async () => {
    draw(records("activity", (page) => (page.Activity = empty)))
    fireEvent.click(await screen.findByRole("button", { name: "More" }))
    const item = await screen.findByRole("menuitem", {
      name: "Download the record",
    })
    expect(item.getAttribute("href")).toBe("/admin/activity.jsonl")
  })

  it("with no log, offers no download and no tabs when there is one", async () => {
    draw(
      records("checks", (page) => {
        page.Logged = false
        page.Health.Logged = false
        page.Tabs = page.Tabs.filter((tab) => tab.Key === "checks")
        page.Checks = {
          Rows: null,
          Verified: 0,
          Unsigned: 0,
          Unchecked: 0,
          Failing: 0,
        }
      })
    )
    expect((await screen.findByRole("status")).textContent).toBe(
      "Intact: the last task checks out"
    )
    expect(screen.queryByRole("button", { name: "More" })).toBeNull()
    expect(screen.queryByRole("tab")).toBeNull()
    expect(document.body.textContent).not.toContain("activity.jsonl")
  })

  it("says why what was asked was refused", async () => {
    draw(
      records("keeping", (page) => (page.Refused = "no task has the id x")),
      "/admin/records?tab=keeping&refused=no+task+has+the+id+x"
    )
    expect((await screen.findByRole("alert")).textContent).toBe(
      "no task has the id x"
    )
  })

  it("says the record is broken on every tab, in the colour for trouble", async () => {
    const broken = (tab: string) =>
      records(tab, (page) => {
        page.Health = {
          ...page.Health,
          Broken: true,
          LogBad: true,
          Log: "the activity log does not hold together: line 4",
        }
        page.Activity = empty
        page.Checks = {
          Rows: null,
          Verified: 1,
          Unsigned: 0,
          Unchecked: 0,
          Failing: 0,
        }
      })
    for (const tab of ["activity", "checks"]) {
      const { unmount } = draw(broken(tab))
      const alert = await screen.findByRole("alert")
      expect(alert.querySelector("strong")?.textContent).toBe(
        "The record is broken."
      )
      expect(alert.textContent).toBe(
        "The record is broken. the activity log does not hold together: line 4. the last task checks out."
      )
      expect(alert.className).toContain("text-bad")
      expect(screen.queryByText("How to check it yourself")).toBeNull()
      expect(document.body.textContent).not.toContain("gusto changes verify")
      unmount()
    }
  })
})
