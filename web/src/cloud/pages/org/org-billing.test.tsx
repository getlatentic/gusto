import { fireEvent, render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { OrgBilling } from "@/cloud/pages/org/org-billing"
import type { CloudBilling } from "@/cloud/types"

const trial: CloudBilling = {
  Admin: true,
  Active: false,
  Words: "Free trial",
  Provider: "",
  Seats: 0,
  People: 2,
  Offers: [
    {
      Provider: "stripe",
      Words: "Subscribe with Stripe: 2 seats at $25 a month",
    },
    {
      Provider: "paypal",
      Words: "Subscribe with PayPal: 2 seats at $25 a month",
    },
  ],
  Manage: false,
  Cancel: false,
  Proposals: null,
}

describe("an organization's billing", () => {
  it("offers an admin a subscription through each processor", () => {
    render(<OrgBilling billing={trial} />)
    expect(screen.getByTestId("plan").textContent).toBe("Free trial")
    const offers = screen.getAllByTestId("subscribe")
    expect(offers.map((button) => button.textContent)).toEqual([
      "Subscribe with Stripe: 2 seats at $25 a month",
      "Subscribe with PayPal: 2 seats at $25 a month",
    ])
    const form = offers[1].closest("form")!
    expect(form.getAttribute("action")).toBe("/org/billing/subscribe")
    expect(new FormData(form).get("provider")).toBe("paypal")
  })

  it("says what is paid, and keeps managing and cancelling behind ⋯", async () => {
    render(
      <OrgBilling
        billing={{
          ...trial,
          Active: true,
          Words: "Paid: 2 seats at $25 a month",
          Provider: "PayPal",
          Seats: 2,
          Offers: null,
          Manage: true,
          Cancel: true,
        }}
      />
    )
    expect(screen.getByText("2 paid, 2 in use")).toBeTruthy()
    expect(screen.queryByTestId("subscribe")).toBeNull()
    fireEvent.click(screen.getByRole("button", { name: "More for billing" }))
    const items = (await screen.findAllByRole("menuitem")).map(
      (item) => item.textContent
    )
    expect(items).toEqual([
      "Card, invoices and plan",
      "New agent token",
      "Cancel the subscription",
    ])
  })

  it("lets an admin approve or decline a change of seats an agent proposed", () => {
    render(
      <OrgBilling
        billing={{
          ...trial,
          Provider: "PayPal",
          Offers: null,
          Proposals: [
            {
              ID: "p1",
              Reason: "Two people joined",
              ApproveURL: "",
              Seats: 4,
              ByAgent: true,
              Pending: true,
              Awaiting: false,
            },
          ],
        }}
      />
    )
    expect(screen.getByTestId("proposal").textContent).toMatch(
      /^An agent proposes 4 seats\. Two people joined/
    )
    expect(screen.getByTestId("approve-proposal").textContent).toBe(
      "Approve and go to PayPal"
    )
    expect(
      screen
        .getByTestId("decline-proposal")
        .closest("form")
        ?.getAttribute("action")
    ).toBe("/org/billing/proposals/decline")
  })

  it("shows a runner the plan and nothing to change", () => {
    render(<OrgBilling billing={{ ...trial, Admin: false }} />)
    expect(screen.getByTestId("plan").textContent).toBe("Free trial")
    expect(screen.queryByTestId("subscribe")).toBeNull()
    expect(
      screen.queryByRole("button", { name: "More for billing" })
    ).toBeNull()
  })
})
