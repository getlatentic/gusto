import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { PeoplePage } from "@/pages/admin/people"
import { renderPage } from "@/test/render"
import type { PeopleData, PersonRow } from "@/lib/types"

const person = (
  email: string,
  name: string,
  role: string,
  extra: Partial<PersonRow> = {}
): PersonRow => ({
  Email: email,
  Name: name,
  Role: role,
  LastSignIn: 0,
  Budget: { Set: false, Limit: 0, Passed: false },
  Spent: null,
  ...extra,
})

function list(change: (page: PeopleData) => void): PeopleData {
  const page: PeopleData = {
    Refused: "",
    Tab: "people",
    Tabs: [
      {
        Key: "people",
        Label: "People",
        Href: "/admin/people?tab=people",
        Count: 0,
        On: true,
      },
    ],
    People: [],
    Pages: { Shown: "", Previous: "", Next: "" },
    Admins: 1,
    Tokens: true,
  }
  change(page)
  return page
}

const draw = (page: PeopleData, at = "/admin/people") =>
  renderPage(at, "/admin/people", <PeoplePage />, {
    [at]: page,
    "/console": { sections: [], orgName: "Acme" },
  })

describe("people", () => {
  it("shows the only admin's role in words, and offers a runner the choice, behind gusto cloud", async () => {
    draw(
      list((page) => {
        page.RoleAction = "https://cloud.test/org/people/role?org=acme-com"
        page.Back = "http://example.com/admin/people"
        page.People = [
          person("ada@acme.com", "Ada", "admin"),
          person("bob@acme.com", "Bob", "runner"),
        ]
      })
    )
    await screen.findByText("ada@acme.com")
    expect(screen.queryByRole("combobox", { name: "Role of Ada" })).toBeNull()
    expect(screen.getByText("admin").className).toContain("capitalize")
    expect(screen.getByRole("combobox", { name: "Role of Bob" })).toBeTruthy()
    const form = screen
      .getByRole("combobox", { name: "Role of Bob" })
      .closest("form")!
    expect(form.getAttribute("action")).toBe(
      "https://cloud.test/org/people/role?org=acme-com"
    )
    expect(
      (form.querySelector('input[name="back"]') as HTMLInputElement).value
    ).toBe("http://example.com/admin/people")
  })

  it("offers no choice of role without gusto cloud", async () => {
    draw(
      list((page) => (page.People = [person("bob@acme.com", "Bob", "runner")]))
    )
    await screen.findByText("bob@acme.com")
    expect(screen.queryByRole("combobox")).toBeNull()
  })

  it("says what each spent, against their budget, and when they reached it", async () => {
    draw(
      list((page) => {
        page.People = [
          person("bob@acme.com", "Bob", "runner", {
            Spent: { calls: 2, usd: 5, unmeasured: 0 },
            Budget: { Set: true, Limit: 4, Passed: true },
          }),
          person("cy@acme.com", "Cy", "runner"),
        ]
      })
    )
    await screen.findByText("bob@acme.com")
    const bob = screen.getByText("bob@acme.com").closest("tr")!
    expect(bob.textContent).toContain("$5.00 of $4.00 reached")
    expect(bob.textContent).toContain("never")
    expect(
      screen.getByText("cy@acme.com").closest("tr")!.textContent
    ).toContain("nothing")
  })

  it("has an Invited tab with its count, and lists only who is invited there", async () => {
    draw(
      list((page) => {
        page.Tab = "invited"
        page.Tabs = [
          {
            Key: "people",
            Label: "People",
            Href: "/admin/people?tab=people",
            Count: 0,
            On: false,
          },
          {
            Key: "invited",
            Label: "Invited",
            Href: "/admin/people?tab=invited",
            Count: 1,
            On: true,
          },
        ]
        page.People = undefined
        page.Invited = [
          {
            Email: "dee@acme.com",
            By: "ada@acme.com",
            Expires: 1795227849,
            Expired: false,
          },
        ]
      }),
      "/admin/people?tab=invited"
    )
    expect(await screen.findByText("dee@acme.com")).toBeTruthy()
    expect(
      screen.getByRole("tab", { name: "Invited 1" }).getAttribute("href")
    ).toBe("/admin/people?tab=invited")
    expect(screen.getByText(/Who is in Acme/)).toBeTruthy()
  })
})
