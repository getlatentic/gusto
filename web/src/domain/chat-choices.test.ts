import { describe, expect, it } from "vitest"

import {
  agentsFor,
  chooseAgent,
  chooseFolder,
  choosePreset,
  fixedByFolder,
  roleOf,
  runsOn,
  startChoices,
  type Offer,
} from "@/domain/chat-choices"

const offer: Offer = {
  agents: [
    {
      value: "claude",
      label: "Claude Code",
      icon: "/static/agents/claude.svg",
    },
    { value: "codex", label: "Codex" },
  ],
  models: {
    claude: [
      { value: "sonnet", label: "Claude Sonnet" },
      { value: "haiku", label: "Claude Haiku" },
    ],
    codex: [{ value: "gpt", label: "GPT" }],
  },
  roles: [
    { value: "", label: "No preset" },
    { kind: "heading", value: "", label: "Your presets" },
    {
      value: "p:1",
      label: "Mine",
      harness: "claude",
      model: "haiku",
      role: "lawyer",
    },
    { kind: "heading", value: "", label: "Built-in presets" },
    { value: "r:eng", label: "Engineer", role: "engineer", fit: ["codex"] },
    { kind: "action", value: "save", label: "Save this setup as a preset…" },
  ],
}

const plain = { preset: "", harness: "claude", model: "sonnet", fixedBy: "" }

describe("what a new chat runs on", () => {
  it("keeps the model when the new agent has it, and takes its first when not", () => {
    expect(chooseAgent(offer, plain, "codex").model).toBe("gpt")
    expect(
      chooseAgent(offer, { ...plain, model: "haiku" }, "claude").model
    ).toBe("haiku")
  })

  it("narrows the agents to a built-in preset's, and moves to one of them", () => {
    expect(agentsFor(offer, "r:eng").map((a) => a.value)).toEqual(["codex"])
    const chosen = choosePreset(offer, plain, "r:eng")
    expect(chosen).toMatchObject({
      preset: "r:eng",
      harness: "codex",
      model: "gpt",
    })
    expect(roleOf(offer, chosen)).toBe("engineer")
  })

  it("sets the agent and model a saved preset holds", () => {
    expect(choosePreset(offer, plain, "p:1")).toMatchObject({
      harness: "claude",
      model: "haiku",
    })
  })

  it("follows and fixes the agent of a folder that has conversations, and frees it for one that has none", () => {
    const fixed = chooseFolder(offer, plain, "codex")
    expect(fixed).toMatchObject({
      harness: "codex",
      model: "gpt",
      fixedBy: fixedByFolder,
    })
    expect(chooseFolder(offer, fixed, "").fixedBy).toBe("")
  })

  it("says in one line what it runs on, naming the preset only when there is one", () => {
    expect(runsOn(offer, plain).words).toBe("Claude Code · Claude Sonnet")
    expect(
      runsOn(
        offer,
        startChoices(offer, { preset: "r:eng", harness: "", model: "" })
      ).words
    ).toBe("Engineer · Codex · GPT")
  })
})
