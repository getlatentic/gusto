import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { draw, records } from "@/pages/admin/records/fixture"
import type { RunChecks } from "@/pages/admin/records/types"

const checks = (change: Partial<RunChecks>): RunChecks => ({
  Rows: null,
  Verified: 0,
  Unsigned: 0,
  Unchecked: 0,
  Failing: 0,
  ...change,
})

const run = (ID: string, State: string) => ({
  ID,
  Brief: `brief ${ID}`,
  Who: "amy@acme.test",
  When: "27 Sep 01:00",
  State,
  Href: `/jobs/${ID}`,
})

describe("the task checks tab", () => {
  it("counts the runs and lists only the ones that need a look", async () => {
    draw(
      records("checks", (page) => {
        page.Checks = checks({
          Verified: 1,
          Unsigned: 1,
          Failing: 1,
          Rows: [
            run("unsigned-1", "unsigned"),
            run("bad-1", "does not check out"),
          ],
        })
      })
    )
    expect(await screen.findByText("Needs a look")).toBeTruthy()
    for (const label of ["verified", "not signed", "failed the check"]) {
      expect(screen.getByText(label)).toBeTruthy()
    }
    expect(screen.getByText("failed the check").nextSibling!.textContent).toBe(
      "1"
    )
    expect(
      (screen.getByText("failed the check").nextSibling as HTMLElement)
        .className
    ).toContain("text-bad")
    expect(
      screen
        .getByRole("link", { name: "brief unsigned-1" })
        .getAttribute("href")
    ).toBe("/jobs/unsigned-1#proof")
    const bad = screen.getByRole("link", { name: "brief bad-1" }).closest("tr")!
    expect(bad.textContent).toContain("failed")
    expect(bad.textContent).not.toContain("does not check out")
    expect(
      screen.getByText(
        "1 run no longer match what was recorded. Open one to see which file changed."
      )
    ).toBeTruthy()
    expect(screen.queryByText("Sweep now")).toBeNull()
    expect(screen.queryByText("How to check it yourself")).toBeNull()
  })

  it("says every run checks out, or that none has finished", async () => {
    const { unmount } = draw(
      records("checks", (page) => (page.Checks = checks({ Verified: 2 })))
    )
    expect(
      await screen.findByText(
        "Every one of the newest 2 finished runs checks out."
      )
    ).toBeTruthy()
    expect(screen.queryByText("Needs a look")).toBeNull()
    unmount()
    draw(records("checks", (page) => (page.Checks = checks({}))))
    expect(await screen.findByText("No run has finished yet.")).toBeTruthy()
  })
})
