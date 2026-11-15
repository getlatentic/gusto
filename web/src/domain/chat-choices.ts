// What a new chat runs on: a preset (a role, or a saved setup), the agent, and the model. Choosing one narrows the others, and
// a folder that already has conversations fixes the agent. Nothing here draws; the chat page holds a Choices and asks these
// functions what changes.

// Option is one agent or model a picker offers.
export interface Option {
  value: string
  label: string
  detail?: string
  icon?: string
}

// RoleChoice is one entry of the preset menu. A heading or separator is not a choice; "save" keeps the current setup as a preset.
export interface RoleChoice {
  kind?: "heading" | "separator" | "action"
  value: string
  label: string
  detail?: string
  harness?: string
  model?: string
  role?: string
  fit?: string[]
}

// Offer is everything the server says a chat may run on.
export interface Offer {
  agents: Option[]
  models: Record<string, Option[]>
  roles: RoleChoice[]
}

export interface Choices {
  preset: string
  harness: string
  model: string
  // fixedBy is why the agent and preset cannot change, or nothing when they can.
  fixedBy: string
}

export const saveChoice = "save"

export const fixedByFolder =
  "Set by this folder, which already has conversations. Choose another folder to use another agent."

export function presetOf(offer: Offer, value: string): RoleChoice | undefined {
  return offer.roles.find(
    (each) =>
      each.value === value &&
      each.kind !== "heading" &&
      each.kind !== "separator"
  )
}

// agentsFor are the agents a preset may run on: a built-in preset names those that can take it; any other preset, all of them.
export function agentsFor(offer: Offer, preset: string): Option[] {
  const fit = presetOf(offer, preset)?.fit
  return fit?.length
    ? offer.agents.filter((agent) => fit.includes(agent.value))
    : offer.agents
}

export function modelsFor(offer: Offer, harness: string): Option[] {
  return offer.models[harness] ?? []
}

// keepOrFirst keeps what was chosen when it is still offered, and takes the first offered when it is not.
export function keepOrFirst(value: string, offered: Option[]): string {
  return offered.some((each) => each.value === value)
    ? value
    : (offered[0]?.value ?? "")
}

export function chooseAgent(
  offer: Offer,
  choices: Choices,
  harness: string
): Choices {
  return {
    ...choices,
    harness,
    model: keepOrFirst(choices.model, modelsFor(offer, harness)),
  }
}

export function chooseModel(choices: Choices, model: string): Choices {
  return { ...choices, model }
}

// choosePreset narrows the agents to the preset's, and a saved preset also chooses its agent and model.
export function choosePreset(
  offer: Offer,
  choices: Choices,
  value: string
): Choices {
  const preset = presetOf(offer, value)
  const harness = keepOrFirst(
    preset?.harness ?? choices.harness,
    agentsFor(offer, value)
  )
  const next = chooseAgent(offer, { ...choices, preset: value }, harness)
  return preset?.model &&
    modelsFor(offer, harness).some((m) => m.value === preset.model)
    ? { ...next, model: preset.model }
    : next
}

// chooseFolder follows the agent a folder already belongs to, and fixes it; a folder with none frees the choice again.
export function chooseFolder(
  offer: Offer,
  choices: Choices,
  harness: string
): Choices {
  if (!harness) return { ...choices, fixedBy: "" }
  return { ...chooseAgent(offer, choices, harness), fixedBy: fixedByFolder }
}

// startChoices are the choices a chat page opens with.
export function startChoices(
  offer: Offer,
  start: { preset: string; harness: string; model: string }
): Choices {
  return choosePreset(
    offer,
    {
      preset: start.preset,
      harness: start.harness,
      model: start.model,
      fixedBy: "",
    },
    start.preset
  )
}

// roleOf is the role a chat starts with, which a preset carries.
export function roleOf(offer: Offer, choices: Choices): string {
  return presetOf(offer, choices.preset)?.role ?? ""
}

export interface RunsOn {
  preset: string
  agent: Option | undefined
  model: string
  // words is the whole choice in one line, for the button's title and a screen reader.
  words: string
}

// runsOn is what a chat will run on, as the box says it.
export function runsOn(offer: Offer, choices: Choices): RunsOn {
  const preset = choices.preset
    ? (presetOf(offer, choices.preset)?.label ?? "")
    : ""
  const agent = offer.agents.find((each) => each.value === choices.harness)
  const model =
    modelsFor(offer, choices.harness).find(
      (each) => each.value === choices.model
    )?.label ?? choices.model
  const words = [preset, agent?.label ?? choices.harness, model]
    .filter(Boolean)
    .join(" · ")
  return { preset, agent, model, words }
}
