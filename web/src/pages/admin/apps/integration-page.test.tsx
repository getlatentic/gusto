import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { IntegrationPage } from "@/pages/admin/apps/integration-page"
import type {
  Integration,
  IntegrationData,
  ShownVersion,
} from "@/pages/admin/apps/types"
import { renderPage } from "@/test/render"

const route = "/admin/integrations/:name"

const reports: Integration = {
  Name: "reports",
  On: "",
  Before: "",
  Programs: "",
  For: null,
  Versions: [
    {
      ID: "v1abc",
      Label: "1.0.0",
      By: "boss@example.com",
      Source: { kind: "git", address: "file:///repo", commit: "1234abcd" },
      Imported: 1_760_000_000,
    },
  ],
  History: [
    {
      at: 1_760_000_000,
      by: "boss@example.com",
      action: "imported",
      version: "v1abc",
    },
  ],
}

const shown: ShownVersion = {
  ID: "v1abc",
  Label: "1.0.0",
  Description: "Reports for the team.",
  Skills: [{ Name: "summarize", Description: "Summarize a report." }],
  Servers: [
    {
      Name: "api",
      Transport: "streamable-http",
      URL: "https://api.example.com/mcp",
      Command: "",
      Args: null,
    },
  ],
  Problems: null,
}

const program = {
  Name: "db",
  Transport: "stdio",
  URL: "",
  Command: "node",
  Args: ["${PLUGIN_ROOT}/db.js"],
}

function draw(at: string, page: IntegrationData) {
  return renderPage(at, route, <IntegrationPage />, { [at]: page })
}

describe("an imported plugin", () => {
  it("is off with a version to turn on, and shows what the version holds and where it came from", async () => {
    draw("/admin/integrations/reports?version=v1abc", {
      Integration: reports,
      Shown: shown,
      Refused: "",
    })
    expect(await screen.findByText("summarize")).toBeTruthy()
    expect(screen.getByText(/api\.example\.com/)).toBeTruthy()
    expect(screen.getAllByText("off").length).toBeGreaterThan(0)
    const turnOn = screen
      .getByRole("button", { name: "Turn on" })
      .closest("form")!
    expect(turnOn.getAttribute("action")).toBe(
      "/admin/integrations/reports/enable"
    )
    expect(
      turnOn.querySelector('input[name="version"]')!.getAttribute("value")
    ).toBe("v1abc")
    expect(screen.getByText("git file:///repo @ 1234abcd")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Turn off" })).toBeNull()
    expect(
      screen.getByRole("link", { name: "Apps & skills" }).getAttribute("href")
    ).toBe("/admin/apps?tab=skills")
  })

  it("chooses who it is on for, holding the addresses one to a line", async () => {
    draw("/admin/integrations/reports", {
      Integration: {
        ...reports,
        On: "v1abc",
        For: ["boss@example.com", "*@finance.example.com"],
      },
      Refused: "",
    })
    const addresses = (await screen.findByLabelText(
      "Addresses"
    )) as HTMLTextAreaElement
    expect(addresses.value).toBe("boss@example.com\n*@finance.example.com\n")
    expect(addresses.closest("form")!.getAttribute("action")).toBe(
      "/admin/integrations/reports/for"
    )
    expect(
      screen
        .getByRole("button", { name: "Turn off" })
        .closest("form")!
        .getAttribute("action")
    ).toBe("/admin/integrations/reports/disable")
    expect(screen.queryByRole("button", { name: "Turn on" })).toBeNull()
  })

  it("puts the earlier version back, and says what turning another on changes", async () => {
    draw("/admin/integrations/reports?version=v1abc", {
      Integration: { ...reports, On: "v2def", Before: "v1abc" },
      Shown: shown,
      Changes: {
        SkillsAdded: null,
        SkillsRemoved: null,
        SkillsChanged: ["summarize"],
        ServersAdded: null,
        ServersRemoved: null,
        ServersChanged: ["api"],
        Manifest: null,
        None: false,
      },
      Against: "v2def",
      Refused: "",
    })
    const back = await screen.findByRole("button", {
      name: "Put the earlier version back",
    })
    expect(back.closest("form")!.getAttribute("action")).toBe(
      "/admin/integrations/reports/restore"
    )
    expect(screen.getByText("Skills changed: summarize")).toBeTruthy()
    expect(
      screen.getByText(/\(address, program or headers\): api/)
    ).toBeTruthy()
  })

  it("shows a version's programs and the way to allow them, then that they are allowed", async () => {
    const page = {
      Integration: reports,
      Shown: { ...shown, Servers: [program] },
      Refused: "",
    }
    const first = draw("/admin/integrations/reports?version=v1abc", page)
    expect(await screen.findByText("node ${PLUGIN_ROOT}/db.js")).toBeTruthy()
    expect(screen.getByText(/waits for the gateway/)).toBeTruthy()
    const allow = screen
      .getByRole("button", { name: "Allow the programs" })
      .closest("form")!
    expect(allow.getAttribute("action")).toBe(
      "/admin/integrations/reports/programs/allow"
    )
    expect(
      allow.querySelector('input[name="version"]')!.getAttribute("value")
    ).toBe("v1abc")
    expect(
      allow.querySelector('input[name="understood"]')!.getAttribute("value")
    ).toBe("yes")
    first.unmount()

    draw("/admin/integrations/reports?version=v1abc", {
      ...page,
      Integration: { ...reports, On: "v1abc", Programs: "v1abc" },
    })
    expect(await screen.findByText("Allowed.")).toBeTruthy()
    expect(
      screen
        .getByRole("button", { name: "Stop allowing them" })
        .closest("form")!
        .getAttribute("action")
    ).toBe("/admin/integrations/reports/programs/refuse")
  })

  it("shows why a change was refused", async () => {
    draw("/admin/integrations/reports?refused=x", {
      Integration: reports,
      Refused: "tick that you have read",
    })
    expect((await screen.findByRole("alert")).textContent).toBe(
      "tick that you have read"
    )
  })
})
