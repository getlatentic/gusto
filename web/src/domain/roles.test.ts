import { describe, expect, it } from "vitest"

import { narrowRoles, pickedRole, type Role } from "@/domain/roles"

const role = (Name: string, Title: string, Industry: string, Summary = "") =>
  ({
    Name,
    Title,
    Industry,
    Summary,
    Skills: [],
    Starters: [],
    Blocked: "",
  }) satisfies Role

const roles = [
  role("health", "Clinical scribe", "Health", "Drafts SOAP notes"),
  role("hr", "People partner", "HR", "Answers policy questions"),
  role("engineering", "Software engineer", "Engineering", "Fixes bugs"),
]

describe("narrowing the built-in presets", () => {
  it("shows every role for every field and no search", () => {
    expect(narrowRoles(roles, "", "")).toEqual(roles)
  })

  it("keeps the roles of the field chosen", () => {
    expect(narrowRoles(roles, "HR", "").map((each) => each.Name)).toEqual([
      "hr",
    ])
  })

  it("finds a role by its title, its field or its summary, in any case, around spaces", () => {
    expect(narrowRoles(roles, "", "  SOAP ").map((each) => each.Name)).toEqual([
      "health",
    ])
    expect(
      narrowRoles(roles, "", "engineering").map((each) => each.Name)
    ).toEqual(["engineering"])
    expect(narrowRoles(roles, "", "partner").map((each) => each.Name)).toEqual([
      "hr",
    ])
  })

  it("asks for both the field and the search", () => {
    expect(narrowRoles(roles, "Health", "bugs")).toEqual([])
  })
})

describe("the role shown beside the list", () => {
  it("is the one chosen while the list shows it", () => {
    expect(pickedRole(roles, roles, "hr")?.Name).toBe("hr")
  })

  it("is the first the list shows once the one chosen is narrowed away", () => {
    const shown = narrowRoles(roles, "Engineering", "")
    expect(pickedRole(roles, shown, "hr")?.Name).toBe("engineering")
  })

  it("stays the one chosen when the list shows none", () => {
    expect(pickedRole(roles, [], "hr")?.Name).toBe("hr")
  })

  it("is none when there are no roles at all", () => {
    expect(pickedRole([], [], "")).toBeUndefined()
  })
})
