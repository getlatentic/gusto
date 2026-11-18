import { screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { ada, membersOf, personPageOf } from "@/pages/members/fixture"
import { membersRoute } from "@/pages/members/members-routes"
import type { PersonData } from "@/pages/members/types"
import { renderRoutes } from "@/test/render"

function draw(page: PersonData) {
  const at = page.Person.Where
  return renderRoutes(at, membersRoute, {
    "/members": membersOf(),
    [at]: page,
  })
}

describe("a person's page", () => {
  it("says who they are, the rooms the person looking shares with them, and the agents they made", async () => {
    draw(personPageOf())
    const page = await screen.findByTestId("person-page")
    expect(within(page).getByRole("heading", { level: 2 }).textContent).toBe(
      "Ada"
    )
    expect(within(page).getByText("ada@acme.com")).toBeTruthy()
    const rooms = within(page).getByRole("region", { name: "Rooms you share" })
    expect(within(rooms).getByRole("link").getAttribute("href")).toBe(
      "/instances/r1/threads/t1"
    )
    const made = within(page).getByRole("region", { name: "Agents they made" })
    expect(within(made).getByRole("link").getAttribute("href")).toBe(
      "/members/agents/a1"
    )
    expect(within(made).getByRole("link").textContent).toContain("Atlas")
  })

  it("says the shared sign-in signs in with the server's password", async () => {
    draw(
      personPageOf((page) => {
        page.Person = {
          ...ada,
          Who: "the shared sign-in",
          Name: "the shared sign-in",
          Initials: "",
          SharedSignIn: true,
          Where: "/members/people/the%20shared%20sign-in",
        }
      })
    )
    expect(
      await screen.findByText("Signs in with this server's password")
    ).toBeTruthy()
  })

  it("lets the person write their own bio, up to its length", async () => {
    draw(personPageOf())
    const form = await screen.findByTestId("bio-form")
    expect(form.getAttribute("action")).toBe("/members/people/ada@acme.com/bio")
    const bio = within(form).getByRole("textbox", {
      name: "Your bio",
    }) as HTMLTextAreaElement
    expect([bio.name, bio.value, bio.maxLength]).toEqual([
      "bio",
      "I run launches.",
      1000,
    ])
    expect(within(form).getByText("Up to 1000 characters")).toBeTruthy()
  })

  it("says why their bio was refused", async () => {
    draw(
      personPageOf(
        (page) => (page.Refused = "a bio is at most 1000 characters")
      )
    )
    const form = await screen.findByTestId("bio-form")
    expect(within(form).getByRole("alert").textContent).toBe(
      "a bio is at most 1000 characters"
    )
  })

  it("shows anybody else the bio to read, with no form, and says when there is none", async () => {
    const drawn = draw(
      personPageOf((page) => (page.Bio = { ...page.Bio, Mine: false }))
    )
    expect(await screen.findByText("I run launches.")).toBeTruthy()
    expect(screen.queryByTestId("bio-form")).toBeNull()
    expect(document.querySelector('[name="bio"]')).toBeNull()
    drawn.unmount()
    draw(
      personPageOf((page) => (page.Bio = { Text: "", Mine: false, Max: 1000 }))
    )
    expect(await screen.findByText("Nothing written yet.")).toBeTruthy()
  })
})
