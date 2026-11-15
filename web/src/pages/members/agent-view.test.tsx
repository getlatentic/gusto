import { fireEvent, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { agentOf, agentPageOf } from "@/pages/members/fixture"
import { MembersPage } from "@/pages/members/members-page"
import type { AgentApp, AgentData } from "@/pages/members/types"
import { renderPage } from "@/test/render"

function draw(page: AgentData, query = "") {
  const at = `/members/agents/a1${query}`
  return renderPage(at, "/members/*", <MembersPage />, { [at]: page })
}

const field = (form: Element, name: string) =>
  form.querySelector(`[name="${name}"]`) as HTMLInputElement | null

const tasks: AgentApp = {
  ID: "tasks",
  Name: "Tasks",
  Choice: "inherit",
  Default: false,
  SignInAgain: false,
}

describe("an agent's page", () => {
  it("names the agent, its type and model, and offers to message it", async () => {
    draw(agentPageOf())
    const page = await screen.findByTestId("agent-page")
    expect(within(page).getByRole("heading", { level: 2 }).textContent).toBe(
      "Atlas"
    )
    expect(within(page).getByText("Codex · big-1")).toBeTruthy()
    const message = within(page).getByTestId("message-agent-form")
    expect(message.getAttribute("action")).toBe("/members/agents/a1/message")
    expect(
      within(screen.getByTestId("members-list"))
        .getAllByTestId("member-row")
        .find((row) => row.getAttribute("aria-current") === "page")
        ?.getAttribute("href")
    ).toBe("/members/agents/a1")
  })

  it("does not offer to message or remove an agent the person may not work with", async () => {
    draw(
      agentPageOf(
        (page) => (page.Agent = agentOf({ Mine: false, Usable: false }))
      )
    )
    await screen.findByTestId("agent-page")
    expect(screen.queryByTestId("message-agent-form")).toBeNull()
    expect(screen.queryByRole("button", { name: "Remove Atlas" })).toBeNull()
  })

  it("asks before removing the agent, and says what removing it does", async () => {
    draw(agentPageOf())
    const sent = vi.spyOn(globalThis, "fetch")
    fireEvent.click(await screen.findByRole("button", { name: "Remove Atlas" }))
    const asking = await screen.findByRole("alertdialog")
    expect(within(asking).getByText("Remove Atlas?")).toBeTruthy()
    expect(
      within(asking).getByText(
        "It leaves every room it is in, and what it said there stays."
      )
    ).toBeTruthy()
    expect(sent.mock.calls.some(([, init]) => init?.method === "POST")).toBe(
      false
    )
    expect(screen.getByTestId("remove-agent-form").getAttribute("action")).toBe(
      "/members/agents/a1/remove"
    )
  })

  it("says why the last change was refused", async () => {
    draw(
      agentPageOf((page) => (page.Refused = "give the person's email address"))
    )
    expect((await screen.findByRole("alert")).textContent).toBe(
      "give the person's email address"
    )
  })

  it("has a tab of each part, each its own address, and Apps only where the server offers apps", async () => {
    const drawn = draw(agentPageOf(), "?tab=access")
    const tabs = await screen.findByTestId("agent-tabs")
    expect(tabs.getAttribute("aria-label")).toBe("About Atlas")
    const links = within(tabs).getAllByRole("link")
    expect(links.map((link) => link.textContent)).toEqual([
      "Profile",
      "Access",
      "Skills",
    ])
    expect(links[1].getAttribute("href")).toBe("/members/agents/a1?tab=access")
    expect(links[1].getAttribute("aria-current")).toBe("page")
    drawn.unmount()
    draw(
      agentPageOf(
        (page) => (page.Apps = { Apps: [], Person: true, Works: true })
      ),
      "?tab=apps"
    )
    const apps = within(await screen.findByTestId("agent-tabs")).getByRole(
      "link",
      {
        name: "Apps",
      }
    )
    expect(apps.getAttribute("aria-current")).toBe("page")
  })

  it("opens the profile for a tab it does not have", async () => {
    draw(agentPageOf(), "?tab=apps")
    expect(await screen.findByTestId("agent-profile-form")).toBeTruthy()
  })
})

describe("the profile tab", () => {
  it("lets the person who made the agent change its name, specialty, model and persona", async () => {
    draw(agentPageOf(), "?tab=profile")
    const form = await screen.findByTestId("agent-profile-form")
    expect(form.getAttribute("action")).toBe("/members/agents/a1")
    expect(field(form, "tab")?.value).toBe("profile")
    expect(field(form, "name")?.value).toBe("Atlas")
    expect(field(form, "specialty")?.value).toBe("Research")
    expect(field(form, "model")?.value).toBe("acme/big-1")
    expect(
      (
        within(form).getByRole("textbox", {
          name: "Persona",
        }) as HTMLTextAreaElement
      ).value
    ).toBe("You are careful.")
    expect(within(form).getByText("Ada")).toBeTruthy()
    expect(within(form).getByText("2m ago")).toBeTruthy()
    expect(within(form).getByRole("button", { name: "Save" })).toBeTruthy()
  })

  it("shows anybody else the profile to read, with nothing to change", async () => {
    draw(
      agentPageOf(
        (page) =>
          (page.Agent = agentOf({
            Mine: false,
            Specialty: "",
            Persona: "",
            Brain: "",
          }))
      )
    )
    const page = await screen.findByTestId("agent-page")
    expect(screen.queryByTestId("agent-profile-form")).toBeNull()
    expect(page.querySelector('[name="persona"]')).toBeNull()
    expect(within(page).getByText("Not set")).toBeTruthy()
    expect(within(page).getByText("None set.")).toBeTruthy()
    expect(within(page).getByText("Default")).toBeTruthy()
  })

  it("lists the rooms of the person's that the agent is in", async () => {
    draw(agentPageOf())
    const rooms = await screen.findByRole("region", { name: "In your rooms" })
    expect(
      within(rooms).getByRole("link", { name: "Launch" }).getAttribute("href")
    ).toBe("/instances/r1/threads/t1")
  })
})

describe("the access tab", () => {
  it("lists who works with the agent, and lets its maker let somebody in or stop them", async () => {
    draw(agentPageOf(), "?tab=access")
    const list = await screen.findByTestId("access-list")
    expect(within(list).getByText("Made it")).toBeTruthy()
    const stop = within(list)
      .getByRole("button", { name: "Remove Bob" })
      .closest("form")!
    expect(stop.getAttribute("action")).toBe("/members/agents/a1/access")
    expect(field(stop, "who")?.value).toBe("bob@acme.com")
    expect(field(stop, "action")?.value).toBe("remove")
    const add = screen.getByTestId("access-form")
    expect(add.getAttribute("action")).toBe("/members/agents/a1/access")
    expect(field(add, "who")?.type).toBe("email")
    expect(field(add, "who")?.required).toBe(true)
    expect(screen.getByLabelText("Let somebody work with it")).toBeTruthy()
  })

  it("shows anybody else who may work with it, with nothing to change", async () => {
    draw(
      agentPageOf((page) => (page.Agent = agentOf({ Mine: false }))),
      "?tab=access"
    )
    const list = await screen.findByTestId("access-list")
    expect(within(list).getByText("Can work with it")).toBeTruthy()
    expect(screen.queryByTestId("access-form")).toBeNull()
  })
})

describe("the skills tab", () => {
  it("says an agent given no skills works with what its agent type knows", async () => {
    draw(agentPageOf(), "?tab=skills")
    expect(
      await screen.findByText(
        "No skills given. Atlas works with what its agent type knows."
      )
    ).toBeTruthy()
  })

  it("lists the skills of the set it is given, or says the set holds none yet", async () => {
    const drawn = draw(
      agentPageOf((page) => {
        page.Agent = agentOf({ Setup: "health" })
        page.Skills = {
          ...page.Skills,
          Given: true,
          Title: "Health",
          Skills: [{ Name: "triage", Description: "Sorts what comes in" }],
        }
      }),
      "?tab=skills"
    )
    const set = await screen.findByRole("list", { name: "Health" })
    expect(set.textContent).toContain("triage")
    expect(set.textContent).toContain("Sorts what comes in")
    drawn.unmount()
    draw(
      agentPageOf((page) => {
        page.Skills = {
          ...page.Skills,
          Given: true,
          Title: "Health",
          Skills: [],
        }
      }),
      "?tab=skills"
    )
    expect(await screen.findByText("Health holds no skills yet.")).toBeTruthy()
  })

  it("gives the agent a role or a skill set as soon as one is chosen", async () => {
    draw(agentPageOf(), "?tab=skills")
    const form = await screen.findByTestId("skills-form")
    expect(form.getAttribute("action")).toBe("/members/agents/a1")
    expect(field(form, "tab")?.value).toBe("skills")
    expect(field(form, "setup")?.value).toBe("")
    const sent = vi
      .spyOn(HTMLFormElement.prototype, "requestSubmit")
      .mockImplementation(() => {})
    fireEvent.click(within(form).getByRole("combobox"))
    expect(await screen.findByText("Ready-made roles")).toBeTruthy()
    expect(screen.getByText("Skill sets")).toBeTruthy()
    await userEvent.click(await screen.findByRole("option", { name: "Health" }))
    expect(sent).toHaveBeenCalledTimes(1)
    expect(sent.mock.contexts[0]).toBe(form)
    expect(field(form, "setup")?.value).toBe("health")
  })

  it("does not offer skills to change to anybody else", async () => {
    draw(
      agentPageOf((page) => (page.Agent = agentOf({ Mine: false }))),
      "?tab=skills"
    )
    await screen.findByText(/The skills Atlas is given in every turn/)
    expect(screen.queryByTestId("skills-form")).toBeNull()
  })
})

describe("the apps tab", () => {
  const withApps = (apps: AgentData["Apps"]) =>
    agentPageOf((page) => (page.Apps = apps))

  it("offers each of the person's apps with their default, and sends a choice as soon as it is made", async () => {
    draw(withApps({ Apps: [tasks], Person: true, Works: true }), "?tab=apps")
    const list = await screen.findByTestId("apps-list")
    const choice = within(list).getByRole("combobox", {
      name: "What Atlas may do with your Tasks",
    })
    expect(choice.textContent).toContain("Inherit (off)")
    const form = choice.closest("form")!
    expect(form.getAttribute("action")).toBe("/members/agents/a1/apps")
    expect(field(form, "app")?.value).toBe("tasks")
    expect(field(form, "choice")?.value).toBe("inherit")
    const sent = vi
      .spyOn(HTMLFormElement.prototype, "requestSubmit")
      .mockImplementation(() => {})
    fireEvent.click(choice)
    await userEvent.click(await screen.findByRole("option", { name: "Allow" }))
    expect(sent).toHaveBeenCalledTimes(1)
    expect(field(form, "choice")?.value).toBe("allow")
  })

  it("asks for a sign-in to an app that no longer works, with nothing to choose", async () => {
    draw(
      withApps({
        Apps: [{ ...tasks, SignInAgain: true }],
        Person: true,
        Works: true,
      }),
      "?tab=apps"
    )
    const list = await screen.findByTestId("apps-list")
    expect(within(list).getByText("sign in again")).toBeTruthy()
    expect(
      within(list).getByRole("link", { name: "Sign in" }).getAttribute("href")
    ).toBe("/connections/tasks/connect")
    expect(list.querySelector('[name="choice"]')).toBeNull()
  })

  it("says who chooses when the person cannot", async () => {
    const drawn = draw(
      withApps({ Apps: [], Person: true, Works: false }),
      "?tab=apps"
    )
    expect(
      await screen.findByText(/Only the people who work with Atlas choose here/)
    ).toBeTruthy()
    drawn.unmount()
    const again = draw(
      withApps({ Apps: [], Person: false, Works: false }),
      "?tab=apps"
    )
    expect(
      await screen.findByText("Sign in as yourself to choose.")
    ).toBeTruthy()
    again.unmount()
    draw(withApps({ Apps: null, Person: true, Works: true }), "?tab=apps")
    expect(await screen.findByText(/No apps connected/)).toBeTruthy()
  })
})

describe("a missing agent", () => {
  it("says there is nothing here", async () => {
    renderPage("/members/agents/gone", "/members/*", <MembersPage />, {})
    expect((await screen.findByRole("alert")).textContent).toBe(
      "There is nothing here."
    )
  })
})
