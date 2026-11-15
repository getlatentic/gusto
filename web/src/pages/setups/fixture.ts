import type { Role } from "@/domain/roles"
import type {
  BlankData,
  PresetsData,
  SetUpData,
  SetupData,
  TemplateData,
} from "@/pages/setups/types"

// What the server answers for the pages a person starts an agent from, as internal/web builds it.

const role = (given: Partial<Role> & { Name: string }): Role => ({
  Title: given.Name,
  Industry: "Engineering",
  Summary: "",
  Skills: [],
  Starters: [],
  Blocked: "",
  ...given,
})

export const presets: PresetsData = {
  Roles: [
    role({
      Name: "engineering",
      Title: "Software engineer",
      Summary: "Fixes bugs and reviews changes.",
      Skills: [{ Title: "Fix bugs", Blurb: "Finds the cause and fixes it." }],
      Starters: ["Fix the failing test"],
    }),
    role({
      Name: "health",
      Title: "Clinical scribe",
      Industry: "Health",
      Summary: "Drafts SOAP notes from visit notes.",
      Skills: [{ Title: "SOAP notes", Blurb: "Writes the note." }],
      Starters: ["Draft a SOAP note from the visit notes"],
    }),
    role({
      Name: "defence",
      Title: "Requirements tracer",
      Industry: "Defense",
      Summary: "Traces requirements to tests.",
      Blocked:
        "This work only goes to models in your own network, and this deployment has none.",
    }),
  ],
  Fields: ["Engineering", "Health", "Defense"],
  Presets: [
    {
      ID: "p1",
      Name: "Reviewer",
      Line: "Software engineer on OpenCode with Stub Model",
    },
  ],
  Refused: "",
}

export const setup: SetupData = {
  Set: {
    Name: "reviewer",
    Off: false,
    Skills: [
      {
        Name: "review",
        Description: "Use when asked to review.",
        Instructions: "<p>Read the diff first.</p>",
      },
    ],
  },
  Servers: ["memory"],
  Offers: true,
  Uses: [{ Name: "linear", Skills: 2, Servers: 1, Held: null, On: true }],
  StartWith: [
    {
      Name: "opencode",
      Href: "/instances/new?harness=opencode&setup=reviewer",
      Unfit: "",
    },
    { Name: "pi", Href: "", Unfit: "it takes no MCP servers" },
  ],
  Refused: "",
}

export const template: TemplateData = {
  Template: {
    Name: "health",
    Title: "Clinical scribe",
    Industry: "Health",
    Summary: "Drafts SOAP notes from visit notes.",
    Restricted: false,
    Skills: [
      {
        Title: "SOAP notes",
        Blurb: "Writes the note.",
        Instructions: "<p>Start with the subjective.</p>",
      },
    ],
    Starters: ["Draft a SOAP note from the visit notes"],
    Files: ["visit-notes.txt"],
  },
  Harnesses: [
    { Name: "opencode", Title: "OpenCode" },
    { Name: "claude", Title: "Claude Code" },
  ],
  InBuilding: false,
  Refused: "",
}

export const blank: BlankData = {
  Engines: [
    { Name: "opencode", Title: "OpenCode", What: "A canned agent." },
    { Name: "claude", Title: "Claude Code", What: "Anthropic's agent." },
  ],
  Setups: ["reviewer"],
  Refused: "",
}

export const setUp: SetUpData = {
  Harness: {
    Name: "opencode",
    Title: "OpenCode",
    What: "A canned agent.",
    Mark: "/static/agents/opencode.svg",
  },
  Models: [
    {
      value: "stub/stub-model",
      label: "stub/stub-model · $1 in, $1 out per million tokens",
    },
    {
      value: "stub/other",
      label: "stub/other · did not finish the one-line fix",
    },
  ],
  Setups: [
    { value: "reviewer", label: "reviewer · 1 skill, 1 MCP" },
    { value: "health", label: "health · 3 skills" },
  ],
  Chosen: "health",
  Then: "",
  Refused: "",
}
