import { fireEvent, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { SkillSetPage } from "@/pages/admin/apps/skill-set-page"
import type { SkillSetData } from "@/pages/admin/apps/types"
import { renderPage } from "@/test/render"

const at = "/admin/skill-sets/reviewer"
const route = "/admin/skill-sets/:name"

const reviewer: SkillSetData = {
  Set: {
    Name: "reviewer",
    Off: false,
    Skills: [
      {
        Name: "review",
        Description: "Use to review.",
        Instructions: "<p>Read the <code>diff</code>.</p>",
      },
    ],
  },
  Uses: [],
  Servers: [
    { Name: "memory", Where: "mcp-memory", Chosen: true },
    { Name: "search", Where: "https://search.example.com/mcp", Chosen: false },
  ],
  Imported: [
    { Name: "reports", On: false, Chosen: true },
    { Name: "notes", On: true, Chosen: false },
  ],
  Earlier: [
    {
      Skill: "review",
      At: "1760000000123456789",
      Fingerprint: "abcdef123456",
      Present: true,
    },
  ],
  Refused: "",
}

const draw = (page: SkillSetData, where = at) =>
  renderPage(where, route, <SkillSetPage />, { [where]: page })

const formOf = (button: string) =>
  screen.getByRole("button", { name: button }).closest("form")!

const values = (form: HTMLFormElement, name: string) =>
  [...form.querySelectorAll<HTMLInputElement>(`input[name="${name}"]`)]
    .filter((input) => input.type === "hidden" || input.checked)
    .map((input) => input.value)

describe("a skill set in the console", () => {
  it("removes a skill, shows its instructions, and adds another", async () => {
    draw(reviewer)
    expect(await screen.findByText("Use to review.")).toBeTruthy()
    expect(formOf("Remove").getAttribute("action")).toBe(
      "/admin/skill-sets/reviewer/skills/review/drop"
    )
    fireEvent.click(screen.getByRole("button", { name: "Instructions" }))
    expect(screen.getByText("diff").tagName).toBe("CODE")
    const add = formOf("Add the skill")
    expect(add.getAttribute("action")).toBe("/admin/skill-sets/reviewer/skills")
    for (const name of ["skill", "description"]) {
      expect(
        add.querySelector(`input[name="${name}"]`)!.hasAttribute("required")
      ).toBe(true)
    }
    expect(add.querySelector('textarea[name="body"]')).toBeTruthy()
    expect(
      screen
        .getByRole("link", { name: "Download as an Agent Plugin" })
        .getAttribute("href")
    ).toBe("/admin/skill-sets/reviewer/plugin.zip")
  })

  it("puts an earlier version back by the exact moment it was kept", async () => {
    draw(reviewer)
    expect(await screen.findByText("Earlier versions")).toBeTruthy()
    const back = formOf("Put it back")
    expect(back.getAttribute("action")).toBe(
      "/admin/skill-sets/reviewer/skills/review/restore"
    )
    expect(
      back.querySelector<HTMLInputElement>('input[name="at"]')!.value
    ).toBe("1760000000123456789")
  })

  it("saves its servers and its plugins each keeping the other", async () => {
    draw(reviewer)
    await screen.findByText("MCP servers")
    const [servers, plugins] = screen
      .getAllByRole("button", { name: "Save" })
      .map((b) => b.closest("form")!)
    for (const form of [servers, plugins]) {
      expect(form.getAttribute("action")).toBe("/admin/skill-sets")
      expect(values(form, "name")).toEqual(["reviewer"])
      expect(values(form, "server")).toEqual(["memory"])
      expect(values(form, "integration")).toEqual(["reports"])
    }
    expect(screen.getByText("https://search.example.com/mcp")).toBeTruthy()
    expect(plugins.textContent).toContain("reports off")
  })

  it("is turned on from its bar, and says while it is off that nobody is offered it", async () => {
    draw({ ...reviewer, Set: { ...reviewer.Set, Off: true } })
    expect((await screen.findByRole("status")).textContent).toContain(
      "This skill set is off"
    )
    expect(formOf("Turn on").getAttribute("action")).toBe(
      "/admin/skill-sets/reviewer/enable"
    )
  })

  it("is turned off from its bar when it is on", async () => {
    draw(reviewer)
    expect(
      (await screen.findByRole("button", { name: "Turn off" }))
        .closest("form")!
        .getAttribute("action")
    ).toBe("/admin/skill-sets/reviewer/disable")
    expect(screen.queryByText(/This skill set is off/)).toBeNull()
  })

  it("says what each plugin it names gives it, and that one that is off stops its tasks", async () => {
    draw({
      ...reviewer,
      Imported: [],
      Uses: [
        { Name: "reports", Skills: 0, Servers: 0, Held: null, On: false },
        {
          Name: "notes",
          Skills: 1,
          Servers: 2,
          Held: ["db: waits for the gateway"],
          On: true,
        },
      ],
    })
    expect(
      await screen.findByText(
        "reports: off, so a task with this setup does not start until an administrator turns it on"
      )
    ).toBeTruthy()
    expect(
      screen.getByText(
        "notes: 1 skill, 2 servers · not used: db: waits for the gateway"
      )
    ).toBeTruthy()
  })

  it("says when it has no skills and the deployment offers no servers", async () => {
    draw({
      ...reviewer,
      Set: { ...reviewer.Set, Skills: [] },
      Servers: [],
      Imported: [],
      Earlier: [],
    })
    expect(await screen.findByText(/No skills yet\./)).toBeTruthy()
    expect(
      screen.getByText(/This deployment offers no MCP servers\./)
    ).toBeTruthy()
    expect(screen.queryByText("Download as an Agent Plugin")).toBeNull()
    expect(screen.queryByText("Integrations")).toBeNull()
  })

  it("shows why a change was refused", async () => {
    const where = at + "?refused=x"
    draw({ ...reviewer, Refused: "say which version to put back" }, where)
    expect((await screen.findByRole("alert")).textContent).toBe(
      "say which version to put back"
    )
  })
})
