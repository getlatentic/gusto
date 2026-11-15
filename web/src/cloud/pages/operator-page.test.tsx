import { render, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { operatorPage } from "@/cloud/fixture"
import { OperatorPage } from "@/cloud/pages/operator-page"

describe("the operator's page", () => {
  it("pauses new demos, or takes them again", () => {
    const { unmount } = render(<OperatorPage page={operatorPage()} />)
    expect(screen.getByTestId("pause").textContent).toBe("Pause new demos")
    expect(
      (document.querySelector("input[name=paused]") as HTMLInputElement).value
    ).toBe("on")
    unmount()
    render(<OperatorPage page={operatorPage({ Paused: true })} />)
    expect(screen.getByTestId("demos-state").textContent).toMatch(
      /^New demos are paused\./
    )
    expect(screen.getByTestId("pause").textContent).toBe("Accept new demos")
    expect(
      (document.querySelector("input[name=paused]") as HTMLInputElement).value
    ).toBe("off")
  })

  it("shows the limits it holds, and the way back to the starting ones once they are changed", () => {
    render(
      <OperatorPage
        page={operatorPage({
          LimitsSet: true,
          Limits: { perDay: 25, perPerson: 1, perCompany: 4, maxOrgs: 3 },
        })}
      />
    )
    expect(
      (
        screen.getByLabelText(
          "Most one address may start in a day"
        ) as HTMLInputElement
      ).value
    ).toBe("1")
    expect(
      (screen.getByLabelText("Most running at once") as HTMLInputElement).max
    ).toBe("50")
    expect(screen.getByText(/These are the limits you set\./)).toBeTruthy()
    expect(
      screen.getByRole("button", { name: "Go back to the starting limits" })
    ).toBeTruthy()
  })

  it("lists who asked for a demo and the hosts that are up", () => {
    render(
      <OperatorPage
        page={operatorPage({
          Hosts: {
            Known: true,
            Error: "",
            Rows: [{ ID: "i-0123", Up: "5 min ago" }],
          },
          Leads: [
            {
              Asked: "just now",
              AskedAt: "1 Oct 2026 09:00 UTC",
              Name: "Ada Lovelace",
              Email: "ada@acme.com",
              Company: "Acme",
              Outcome: "starting",
            },
          ],
        })}
      />
    )
    expect(
      within(screen.getByTestId("hosts")).getByText("5 min ago")
    ).toBeTruthy()
    expect(
      within(screen.getByTestId("leads")).getByText("ada@acme.com")
    ).toBeTruthy()
    expect(screen.getByText(/1 so far/)).toBeTruthy()
    expect(screen.getByText("No demo is running.")).toBeTruthy()
  })

  it("says why the hosts could not be listed", () => {
    render(
      <OperatorPage
        page={operatorPage({
          Hosts: { Known: false, Error: "no credentials", Rows: null },
        })}
      />
    )
    expect(
      screen.getByText("The hosts could not be listed: no credentials")
    ).toBeTruthy()
  })
})
