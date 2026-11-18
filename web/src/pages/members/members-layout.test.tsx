import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import { beforeEach, describe, expect, it } from "vitest"

import { kindOf, membersOf } from "@/pages/members/fixture"
import { membersRoute } from "@/pages/members/members-routes"
import type { MembersData } from "@/pages/members/types"
import { renderRoutes } from "@/test/render"

function draw(page: MembersData, at = "/members") {
  return renderRoutes(at, membersRoute, { [at]: page })
}

const field = (form: Element, name: string) =>
  form.querySelector(`[name="${name}"]`) as HTMLInputElement | null

beforeEach(() => localStorage.clear())

describe("the members list", () => {
  it("lists the team's people and agents, each linking to their page, and the way to the roles and presets", async () => {
    draw(membersOf())
    const list = await screen.findByRole("navigation", { name: "Members" })
    // The navigation can be drawn before all its rows, so the rows are waited for as well.
    await waitFor(() =>
      expect(within(list).getAllByTestId("member-row")).toHaveLength(3)
    )
    const rows = within(list).getAllByTestId("member-row")
    expect(rows.map((row) => row.getAttribute("href"))).toEqual([
      "/members/people/ada@acme.com",
      "/members/people/bob@acme.com",
      "/members/agents/a1",
    ])
    expect(rows[2].textContent).toContain("Atlas")
    expect(rows[2].textContent).toContain("Research")
    expect(
      within(list)
        .getByRole("link", { name: /Ready-made roles and presets/ })
        .getAttribute("href")
    ).toBe("/presets")
  })

  it("shows only the people or only the agents on their tabs, and opens on the tab chosen last", async () => {
    const drawn = draw(membersOf())
    fireEvent.click(await screen.findByRole("tab", { name: "Agents" }))
    expect(
      screen.getAllByTestId("member-row").map((row) => row.textContent)
    ).toEqual([expect.stringContaining("Atlas")])
    expect(localStorage.getItem("gusto-member-tab")).toBe("agents")
    drawn.unmount()
    draw(membersOf())
    expect(
      (await screen.findByRole("tab", { name: "Agents" })).getAttribute(
        "aria-selected"
      )
    ).toBe("true")
    fireEvent.click(screen.getByRole("tab", { name: "People" }))
    expect(screen.getAllByTestId("member-row")).toHaveLength(2)
  })

  it("marks whoever is open", async () => {
    draw(membersOf(), "/members")
    const rows = await screen.findAllByTestId("member-row")
    expect(rows.every((row) => !row.hasAttribute("aria-current"))).toBe(true)
  })
})

describe("the new agent dialog", () => {
  it("is opened by the address, and says why the last agent was refused", async () => {
    draw(
      membersOf(
        (page) => (page.Refused = "choose an agent type this server can run")
      ),
      "/members?new=1&refused=x&refused-seal=y"
    )
    const dialog = await screen.findByTestId("new-agent-dialog")
    expect(
      within(dialog).getByRole("heading", { name: "New agent" })
    ).toBeTruthy()
    expect(within(dialog).getByRole("alert").textContent).toBe(
      "choose an agent type this server can run"
    )
    const form = within(dialog).getByTestId("new-agent-form")
    expect(form.getAttribute("action")).toBe("/members/agents")
    expect(field(form, "name")?.required).toBe(true)
    expect(field(form, "name")?.maxLength).toBe(32)
    expect(field(form, "specialty")?.maxLength).toBe(80)
    expect(field(form, "setup")).toBeNull()
  })

  it("is opened by the + where nobody can be invited", async () => {
    draw(membersOf())
    expect(screen.queryByRole("button", { name: "Add a member" })).toBeNull()
    const plus = await screen.findByTestId("new-agent")
    expect(plus.getAttribute("aria-label")).toBe("New agent")
    fireEvent.click(plus)
    expect(await screen.findByTestId("new-agent-dialog")).toBeTruthy()
  })

  it("offers every agent type, the first that can run chosen, and says why one cannot run", async () => {
    draw(
      membersOf((page) => {
        page.NewAgent.Kinds.push(
          kindOf({
            Name: "claude",
            Title: "Claude Code",
            Ready: false,
            Why: "npm install -g @anthropic-ai/claude-code",
          })
        )
      }),
      "/members?new=1"
    )
    const dialog = await screen.findByTestId("new-agent-dialog")
    const codex = within(dialog).getByRole("radio", {
      name: /Codex/,
    }) as HTMLInputElement
    const claude = within(dialog).getByRole("radio", {
      name: /Claude Code/,
    }) as HTMLInputElement
    expect([codex.value, codex.checked, codex.disabled]).toEqual([
      "codex",
      true,
      false,
    ])
    expect([claude.value, claude.disabled]).toEqual(["claude", true])
    expect(
      within(dialog).getByText(
        "Not on this server: npm install -g @anthropic-ai/claude-code"
      )
    ).toBeTruthy()
    expect(within(dialog).queryByRole("button", { name: "Install" })).toBeNull()
  })

  it("installs an agent type this computer lacks, and comes back to the dialog", async () => {
    draw(
      membersOf((page) => {
        page.NewAgent.Kinds.push(
          kindOf({
            Name: "claude",
            Title: "Claude Code",
            Ready: false,
            Installable: true,
          })
        )
      }),
      "/members?new=1"
    )
    const dialog = await screen.findByTestId("new-agent-dialog")
    expect(within(dialog).getByText("Not on this computer yet")).toBeTruthy()
    const install = within(dialog).getByRole("button", { name: "Install" })
    const form = document.getElementById(
      install.getAttribute("form")!
    ) as HTMLFormElement
    expect(form.getAttribute("action")).toBe("/agents/claude/install")
    expect(field(form, "back")?.value).toBe("/members?new=1")
  })

  it("cannot make an agent where no agent type can run, and says so", async () => {
    draw(
      membersOf((page) => {
        page.NewAgent = {
          Kinds: [kindOf({ Ready: false, Why: "missing" })],
          FirstReady: "",
          Offered: [],
        }
      }),
      "/members?new=1"
    )
    const dialog = await screen.findByTestId("new-agent-dialog")
    expect(
      (
        within(dialog).getByRole("button", {
          name: "Make the agent",
        }) as HTMLButtonElement
      ).disabled
    ).toBe(true)
    expect(within(dialog).getByRole("status").textContent).toContain(
      "No agent type can run on this server yet."
    )
    expect(within(dialog).queryByText("Model")).toBeNull()
  })
})

describe("inviting a person", () => {
  const invite = {
    Action: "https://cloud.test/org/invite?org=acme-com",
    Team: "main",
    Back: "http://gusto.test/members",
  }

  it("posts to gusto cloud's form from the + menu, naming the team and this page", async () => {
    draw(membersOf((page) => (page.Invite = invite)))
    fireEvent.click(await screen.findByRole("button", { name: "Add a member" }))
    fireEvent.click(
      await screen.findByRole("menuitem", { name: "Invite a person" })
    )
    const form = within(await screen.findByTestId("invite-dialog")).getByTestId(
      "invite-form"
    )
    expect(form.getAttribute("action")).toBe(invite.Action)
    expect(field(form, "team")?.value).toBe("main")
    expect(field(form, "back")?.value).toBe(invite.Back)
    expect(field(form, "email")?.type).toBe("email")
    expect(field(form, "email")?.required).toBe(true)
  })

  it("offers New agent beside it in the menu", async () => {
    draw(membersOf((page) => (page.Invite = invite)))
    fireEvent.click(await screen.findByRole("button", { name: "Add a member" }))
    fireEvent.click(await screen.findByRole("menuitem", { name: "New agent" }))
    expect(await screen.findByTestId("new-agent-dialog")).toBeTruthy()
  })

  it("says the invitation was sent when gusto cloud says so", async () => {
    draw(
      membersOf((page) => ((page.Invite = invite), (page.Sent = true))),
      "/members?sent=1"
    )
    expect((await screen.findByRole("status")).textContent).toBe(
      "The invitation was sent. The person is mailed a link that works for seven days."
    )
  })
})
