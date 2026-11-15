import { fireEvent, screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { AppsPage } from "@/pages/admin/apps/apps-page"
import type { AppsData } from "@/pages/admin/apps/types"
import { renderPage } from "@/test/render"

const tabs = [
  { Key: "apps", Label: "Apps", Href: "/admin/apps?tab=apps", On: true },
  { Key: "skills", Label: "Skills", Href: "/admin/apps?tab=skills", On: false },
]

const tasks = {
  ID: "tasks",
  Name: "Tasks",
  What: "Tasks in your tracker",
  On: false,
  Usable: true,
  NeedSignIn: 0,
}

function appsTab(apps: AppsData["Apps"], turnedOn = ""): AppsData {
  return {
    Tabs: tabs,
    Tab: "apps",
    Refused: "",
    Apps: apps,
    TurnedOn: turnedOn,
  }
}

const skillsTab: AppsData = {
  Tabs: tabs.map((tab) => ({ ...tab, On: tab.Key === "skills" })),
  Tab: "skills",
  Refused: "",
  Skills: [
    {
      Name: "reviewer",
      Href: "/admin/skill-sets/reviewer",
      Kind: "Skill set",
      Detail: "1 skill",
      On: false,
    },
    {
      Name: "reports",
      Href: "/admin/integrations/reports",
      Kind: "Imported plugin",
      Detail: "2 versions",
      On: true,
    },
  ],
  Writes: true,
  Imports: true,
  Servers: ["memory"],
  Imported: [{ Name: "reports", On: false, Chosen: false }],
}

const switchFor = (name: string) =>
  screen.getByRole("switch", { name: `${name} for everyone` })

describe("Apps & skills: apps", () => {
  it("turns an app that is off on with its switch, and says how agents use it", async () => {
    renderPage("/admin/apps", "/admin/apps", <AppsPage />, {
      "/admin/apps": appsTab([tasks]),
    })
    expect(await screen.findByText("Tasks")).toBeTruthy()
    expect(switchFor("Tasks").getAttribute("aria-checked")).toBe("false")
    expect(switchFor("Tasks").closest("form")!.action).toContain(
      "/admin/connectors/tasks/on"
    )
    expect(screen.queryByRole("button", { name: "Turn on" })).toBeNull()
    const sent = vi
      .spyOn(HTMLFormElement.prototype, "requestSubmit")
      .mockImplementation(() => {})
    fireEvent.click(switchFor("Tasks"))
    expect(sent).toHaveBeenCalledTimes(1)
    expect(sent.mock.contexts[0]).toBe(switchFor("Tasks").closest("form"))
    sent.mockRestore()
    expect(
      screen.getByText(
        /Agents use it only as that person, and only for what they ask\./
      )
    ).toBeTruthy()
    expect(
      screen.getByText(/What agents may use, for everyone in/)
    ).toBeTruthy()
  })

  it("turns an app that is on off, and says it is on when it was just turned on", async () => {
    renderPage("/admin/apps", "/admin/apps", <AppsPage />, {
      "/admin/apps": appsTab([{ ...tasks, On: true }], "Tasks"),
    })
    expect((await screen.findByRole("status")).textContent).toContain(
      "Tasks is on. Each person connects their own account"
    )
    expect(switchFor("Tasks").getAttribute("aria-checked")).toBe("true")
    expect(switchFor("Tasks").closest("form")!.action).toContain(
      "/admin/connectors/tasks/off"
    )
  })

  it("says an app needs setting up, and how many people need to sign in again", async () => {
    renderPage("/admin/apps", "/admin/apps", <AppsPage />, {
      "/admin/apps": appsTab([
        { ...tasks, NeedSignIn: 1 },
        { ...tasks, ID: "crm", Name: "CRM", Usable: false },
      ]),
    })
    expect(
      await screen.findByText("1 person needs to sign in again")
    ).toBeTruthy()
    expect(screen.getByText("needs setup")).toBeTruthy()
    expect(
      screen.queryByRole("switch", { name: "CRM for everyone" })
    ).toBeNull()
  })

  it("shows a refusal", async () => {
    renderPage("/admin/apps?refused=x", "/admin/apps", <AppsPage />, {
      "/admin/apps?refused=x": {
        ...appsTab([tasks]),
        Refused: "CRM needs setting up",
      },
    })
    expect((await screen.findByRole("alert")).textContent).toBe(
      "CRM needs setting up"
    )
  })
})

describe("Apps & skills: skills", () => {
  it("lists each skill set and plugin, on or off, each a link to its page", async () => {
    renderPage("/admin/apps?tab=skills", "/admin/apps", <AppsPage />, {
      "/admin/apps?tab=skills": skillsTab,
    })
    const reviewer = (
      await screen.findByRole("link", { name: "reviewer" })
    ).closest("li")!
    expect(reviewer.querySelector("a")!.getAttribute("href")).toBe(
      "/admin/skill-sets/reviewer"
    )
    expect(reviewer.textContent).toContain("Skill set · 1 skill")
    expect(reviewer.textContent).toContain("off")
    const reports = screen.getByRole("link", { name: "reports" }).closest("li")!
    expect(reports.textContent).toContain("on")
    expect(
      screen.getByRole("tab", { name: "Skills" }).getAttribute("aria-selected")
    ).toBe("true")
  })

  it("opens the ways to add a skill, and each way shows only its form", async () => {
    renderPage("/admin/apps?tab=skills", "/admin/apps", <AppsPage />, {
      "/admin/apps?tab=skills": skillsTab,
    })
    fireEvent.click(await screen.findByRole("button", { name: "Add a skill" }))
    for (const way of [
      "Write a skill set",
      "Import from git",
      "Import an upload",
    ]) {
      expect(screen.getByText(way)).toBeTruthy()
    }
    const create = screen
      .getByRole("button", { name: "Create" })
      .closest("form")!
    expect(create.getAttribute("action")).toBe("/admin/skill-sets")
    expect(
      create.querySelector('input[name="name"]')!.getAttribute("pattern")
    ).toBe("[a-z0-9][a-z0-9\\-]*")
    expect(screen.getByText("memory")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Import from git" })).toBeNull()

    fireEvent.click(screen.getByText("Import from git"))
    const git = screen
      .getByRole("button", { name: "Import from git" })
      .closest("form")!
    expect(git.getAttribute("action")).toBe("/admin/integrations/import")
    expect(git.getAttribute("enctype")).toBe("multipart/form-data")
    expect(git.querySelector('input[name="kind"]')!.getAttribute("value")).toBe(
      "git"
    )
    expect(screen.queryByRole("button", { name: "Create" })).toBeNull()

    fireEvent.click(screen.getByText("Import an upload"))
    const upload = screen
      .getByRole("button", { name: "Import the upload" })
      .closest("form")!
    expect(upload.getAttribute("action")).toBe("/admin/integrations/import")
    expect(
      upload.querySelector('input[name="archive"]')!.getAttribute("type")
    ).toBe("file")
  })

  it("says there are no skills yet, and how to add one", async () => {
    renderPage("/admin/apps?tab=skills", "/admin/apps", <AppsPage />, {
      "/admin/apps?tab=skills": { ...skillsTab, Skills: [], Writes: false },
    })
    expect(await screen.findByText("No skills yet")).toBeTruthy()
    expect(screen.getByText(/Import an Agent Plugin to add one\./)).toBeTruthy()
  })
})
