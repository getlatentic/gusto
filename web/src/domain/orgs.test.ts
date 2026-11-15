import { describe, expect, it } from "vitest"

import { orgLines } from "@/domain/orgs"

describe("the organization switcher", () => {
  it("lists an organization with one workspace as one line, and one with more under its name", () => {
    const lines = orgLines(
      [
        {
          slug: "acme",
          name: "Acme",
          open: "https://cloud/acme",
          workspaces: [
            { id: "w1", name: "Research", open: "https://cloud/acme/w1" },
            { id: "w2", name: "Sales", open: "https://cloud/acme/w2" },
          ],
        },
        { slug: "beta", name: "Beta", open: "https://cloud/beta" },
      ],
      "acme",
      "w2"
    )
    expect(lines).toEqual([
      { kind: "heading", key: "acme heading", text: "Acme" },
      {
        kind: "place",
        key: "acme w1",
        text: "Research",
        href: "/workspace/w1",
        current: false,
        inset: true,
      },
      {
        kind: "place",
        key: "acme w2",
        text: "Sales",
        href: "/workspace/w2",
        current: true,
        inset: true,
      },
      {
        kind: "place",
        key: "beta",
        text: "Beta",
        href: "https://cloud/beta",
        current: false,
        inset: false,
      },
    ])
  })
})
