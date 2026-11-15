import { render, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { cloudPage } from "@/cloud/fixture"
import { OrgPage } from "@/cloud/pages/org/org-page"
import type { CloudPage } from "@/cloud/types"

const acme = {
  Slug: "acme-com",
  Name: "Acme",
  Admin: "ada@acme.com",
  Demo: false,
}

const adminView = (set: Partial<CloudPage> = {}) =>
  cloudPage("org", {
    Org: acme,
    Address: "acme-com.cloud.test",
    Person: { Email: "ada@acme.com", Trial: true },
    IsAdmin: true,
    Admins: 1,
    CanInvite: true,
    Computers: 0,
    Members: [
      { Email: "ada@acme.com", Role: "admin", Computer: "" },
      { Email: "bob@acme.com", Role: "runner", Computer: "i-1" },
    ],
    Invited: ["cy@acme.com"],
    Workspaces: [
      {
        ID: "w1",
        Name: "Acme",
        People: [{ Email: "ada@acme.com", Removable: false }],
        Others: ["bob@acme.com"],
      },
    ],
    ...set,
  })

describe("an organization's page", () => {
  it("says what the organization may do in a word or two each", () => {
    render(
      <OrgPage page={adminView({ Credit: { Left: "$4.50", Of: "$5.00" } })} />
    )
    const value = (id: string) =>
      within(screen.getByTestId(id)).getByTestId("figure-value").textContent
    expect(value("figure-tasks")).toBe("20")
    expect(screen.getByTestId("figure-tasks").getAttribute("title")).toBe(
      "in all, 1 at a time"
    )
    expect(value("figure-people")).toBe("2 of 1")
    expect(value("figure-computers")).toBe("0 of 1")
    expect(value("figure-credit")).toBe("$4.50")
  })

  it("says an address is not verified, and offers the link again", () => {
    render(<OrgPage page={adminView()} />)
    expect(screen.getByTestId("email-state").textContent).toBe("Not verified")
    expect(screen.getByText(/raise this to 200 tasks a month/)).toBeTruthy()
    expect(
      screen
        .getByTestId("send-link-again")
        .closest("form")
        ?.getAttribute("action")
    ).toBe("/org/verify")
  })

  it("offers an admin the other role, and does not offer the last admin runner", () => {
    render(<OrgPage page={adminView()} />)
    const people = screen.getByTestId("people")
    const row = (email: string) =>
      within(people).getByText(email).closest("li")!
    expect(within(row("ada@acme.com")).queryByRole("button")).toBeNull()
    expect(
      within(row("bob@acme.com")).getByRole("button", { name: "Make admin" })
    ).toBeTruthy()
    expect(
      within(row("bob@acme.com")).getByRole("button", { name: "Remove" })
    ).toBeTruthy()
    expect(
      within(row("bob@acme.com")).getByText("runner, computer i-1")
    ).toBeTruthy()
    expect(
      within(row("cy@acme.com")).getByRole("button", { name: "Cancel" })
    ).toBeTruthy()
    expect(
      screen.getByTestId("invite-email").closest("form")?.getAttribute("action")
    ).toBe("/org/invite")
  })

  it("shows a runner the people and nothing to change", () => {
    render(<OrgPage page={adminView({ IsAdmin: false, CanInvite: false })} />)
    expect(
      within(screen.getByTestId("people")).queryAllByRole("button")
    ).toHaveLength(0)
    expect(screen.queryByTestId("invite-email")).toBeNull()
    expect(screen.queryByTestId("workspaces")).toBeNull()
  })

  it("lets an admin fill and rename a workspace and make another", () => {
    render(<OrgPage page={adminView()} />)
    const spaces = screen.getByTestId("workspaces")
    expect(within(spaces).getByRole("button", { name: "Rename" })).toBeTruthy()
    expect(within(spaces).getByRole("button", { name: "Add" })).toBeTruthy()
    expect(screen.getByTestId("make-workspace")).toBeTruthy()
  })

  it("offers to open gusto once its server is ready", () => {
    render(<OrgPage page={adminView({ CanOpen: true, Server: "ready" })} />)
    expect(screen.getByTestId("open-gusto").getAttribute("href")).toBe(
      "/open?org=acme-com"
    )
    expect(screen.getByText("Ready")).toBeTruthy()
  })

  it("asks no one in a demo to confirm an address", () => {
    render(<OrgPage page={adminView({ Org: { ...acme, Demo: true } })} />)
    expect(screen.queryByTestId("email-state")).toBeNull()
    expect(screen.queryByTestId("send-link-again")).toBeNull()
    expect(
      screen.getByRole("link", { name: "the demo page" }).getAttribute("href")
    ).toBe("/demo/wait")
  })

  it("offers a plan to subscribe to where gusto cloud bills", () => {
    render(
      <OrgPage
        page={adminView({
          Plan: {
            Words: "Free",
            Offer: "Subscribe for $25 a seat",
            Subscribe: true,
            Manage: false,
          },
        })}
      />
    )
    expect(screen.getByTestId("subscribe").textContent).toBe(
      "Subscribe for $25 a seat"
    )
    expect(screen.queryByTestId("manage-billing")).toBeNull()
  })
})
