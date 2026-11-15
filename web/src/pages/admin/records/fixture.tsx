import { screen } from "@testing-library/react"
import { expect } from "vitest"

import { RecordsPage } from "@/pages/admin/records/records-page"
import type { RecordsData } from "@/pages/admin/records/types"
import { renderPage } from "@/test/render"

// records is a server's Records data with every tab, the record intact, and change applying to it before it is given.
export function records(
  tab: string,
  change: (page: RecordsData) => void = () => {}
): RecordsData {
  const page: RecordsData = {
    Tabs: ["activity", "checks", "keeping"].map((key) => ({
      Key: key,
      Label: {
        activity: "Activity",
        checks: "Task checks",
        keeping: "Keeping",
      }[key]!,
      Href: `/admin/records?tab=${key}`,
      Count: 0,
      On: key === tab,
    })),
    Tab: tab,
    Refused: "",
    Logged: true,
    Health: {
      Logged: true,
      Broken: false,
      Log: "3 events, all signed",
      Runs: "the last task checks out",
      LogBad: false,
      RunsBad: false,
      Head: "7:abc123",
    },
  }
  change(page)
  return page
}

export const draw = (
  page: RecordsData,
  at = `/admin/records?tab=${page.Tab}`
) =>
  renderPage(at, "/admin/records", <RecordsPage />, {
    [at]: page,
  })

// everyControlIsNamed checks that every link and button says what it is, and every field has a label.
export function everyControlIsNamed() {
  for (const role of ["link", "button", "tab", "menuitem"]) {
    for (const each of screen.queryAllByRole(role)) {
      expect(
        each.textContent?.trim() || each.getAttribute("aria-label")
      ).toBeTruthy()
    }
  }
  for (const field of document.querySelectorAll("input:not([type=hidden])")) {
    expect(
      field.id && document.querySelector(`label[for="${field.id}"]`)
    ).toBeTruthy()
  }
}
