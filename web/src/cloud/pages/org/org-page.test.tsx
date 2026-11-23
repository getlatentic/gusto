import { fireEvent, render, screen, within } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

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
    Teams: [
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

  it("offers an admin the other role behind each person's menu, and the last admin no runner", async () => {
    render(<OrgPage page={adminView()} />)
    expect(
      screen.queryByRole("button", { name: "More for ada@acme.com" })
    ).toBeNull()
    expect(screen.getByText("runner, computer i-1")).toBeTruthy()
    fireEvent.click(
      screen.getByRole("button", { name: "More for bob@acme.com" })
    )
    const items = (await screen.findAllByRole("menuitem")).map(
      (item) => item.textContent
    )
    expect(items).toEqual(["Make admin", "Remove"])
  })

  it("sends a person's change as its own form", async () => {
    render(<OrgPage page={adminView()} />)
    fireEvent.click(
      screen.getByRole("button", { name: "More for bob@acme.com" })
    )
    const submitted = vi.fn((event: Event) => event.preventDefault())
    document.addEventListener("submit", submitted)
    fireEvent.click(await screen.findByRole("menuitem", { name: "Make admin" }))
    document.removeEventListener("submit", submitted)
    const form = submitted.mock.calls[0][0].target as HTMLFormElement
    expect(form.getAttribute("action")).toBe("/org/people/role")
    expect(new FormData(form).get("role")).toBe("admin")
  })

  it("invites from a dialog, and cancels an invitation from its menu", async () => {
    render(<OrgPage page={adminView()} />)
    fireEvent.click(screen.getByTestId("invite"))
    expect(
      (await screen.findByTestId("invite-email"))
        .closest("form")
        ?.getAttribute("action")
    ).toBe("/org/invite")
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }))
    fireEvent.click(
      screen.getByRole("button", { name: "More for cy@acme.com" })
    )
    expect(
      await screen.findByRole("menuitem", { name: "Cancel the invitation" })
    ).toBeTruthy()
  })

  it("shows a runner the people and nothing to change", () => {
    render(<OrgPage page={adminView({ IsAdmin: false, CanInvite: false })} />)
    expect(
      within(screen.getByTestId("people")).queryAllByRole("button")
    ).toHaveLength(0)
    expect(screen.queryByTestId("invite")).toBeNull()
    expect(screen.queryByTestId("teams")).toBeNull()
  })

  it("lets an admin make a team, and rename and fill one from its menu", async () => {
    render(<OrgPage page={adminView()} />)
    expect(screen.getByTestId("make-team").textContent).toBe("New team")
    fireEvent.click(screen.getByRole("button", { name: "More for Acme" }))
    const items = (await screen.findAllByRole("menuitem")).map(
      (item) => item.textContent
    )
    expect(items).toEqual(["Rename", "Add a person"])
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
})
