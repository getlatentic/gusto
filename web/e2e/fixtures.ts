import { randomBytes } from "node:crypto"
import {
  test as base,
  expect,
  type APIRequestContext,
  type Locator,
  type Page,
} from "@playwright/test"

import { admin, runner, stateOf } from "./values.mjs"

export { expect }

// unique is a name no other test of this run uses, since every test shares one server.
export const unique = (word: string) =>
  `${word}${randomBytes(4).toString("hex")}`

// sideRow is the side list's row for the conversation at path, within scope: the page, or one part of the list.
export function sideRow(scope: Page | Locator, path: string): Locator {
  const page = "goto" in scope ? scope : scope.page()
  return scope.getByTestId("side-row").and(page.locator(`[href="${path}"]`))
}

export interface Room {
  id: string
  // path is the room's conversation.
  path: string
}

// Gusto seeds what a test needs through the server's own forms, as the person the request context is signed in as.
export class Gusto {
  private readonly request: APIRequestContext

  constructor(request: APIRequestContext) {
    this.request = request
  }

  // post sends a form and gives where the server sent the person; a refusal fails the test, with the server's reason.
  async post(
    path: string,
    fields: Record<string, string> | URLSearchParams
  ): Promise<string> {
    const answer = await this.request.post(path, {
      headers: { "content-type": "application/x-www-form-urlencoded" },
      data: new URLSearchParams(fields).toString(),
      maxRedirects: 0,
    })
    expect(answer.status(), `${path}: ${await answer.text()}`).toBe(303)
    const landed = answer.headers().location ?? ""
    const refused = new URL(landed, "http://gusto").searchParams.get("refused")
    expect(refused, `${path} was refused`).toBeNull()
    return landed
  }

  // makeAgent makes one of the person's agents, the canned one, which replies and writes policy.md.
  async makeAgent(name: string): Promise<string> {
    const landed = await this.post("/members/agents", {
      name,
      specialty: "Research",
      persona: "Brief.",
      harness: "opencode",
      model: "stub/stub-model",
    })
    return landed.split("/members/agents/")[1].split("?")[0]
  }

  async makeRoom(name: string): Promise<Room> {
    const landed = await this.post("/rooms", { name })
    const path = landed.split("?")[0]
    return { id: path.split("/")[2], path }
  }

  // addToRoom adds agents and people to a room in one go, as its add dialog does.
  async addToRoom(
    room: Room,
    { agents = [], people = [] }: { agents?: string[]; people?: string[] }
  ) {
    const form = new URLSearchParams({ who: people.join(",") })
    for (const agent of agents) form.append("agent", agent)
    await this.post(`/rooms/${room.id}/members/add`, form)
  }

  // say says something in a conversation and gives the turn it made.
  async say(path: string, words: string): Promise<string> {
    const landed = await this.post(`${path}/say`, { said: words })
    return landed.split("#")[1]
  }

  // replied waits until the turn's reply is written.
  async replied(path: string, turn: string) {
    await expect
      .poll(async () => {
        const answer = await this.request.get(`/app/api${path}/turns`)
        const read = (await answer.json()) as {
          Turns: { ID: string; Done: boolean }[]
        }
        return read.Turns.find((each) => each.ID === turn)?.Done ?? false
      })
      .toBe(true)
  }

  // savePreset keeps the canned agent as one of the person's presets.
  async savePreset(name: string) {
    await this.post("/presets", {
      name,
      harness: "opencode",
      model: "stub/stub-model",
      role: "",
    })
  }
}

// roomWithAgent is a room of the admin's with their canned agent in it, and the runner as a member.
export async function roomWithAgent(gusto: Gusto) {
  const agent = unique("Scout")
  const id = await gusto.makeAgent(agent)
  const room = await gusto.makeRoom(unique("Planning "))
  await gusto.addToRoom(room, { agents: [id], people: [runner.email] })
  return { room, agent }
}

// Each person's Gusto has its own signed-in requests, apart from the page's browser, so a test that signs the page out still
// seeds as them.
export const test = base.extend<{
  gusto: Gusto
  runnerGusto: Gusto
  runnerPage: Page
}>({
  gusto: async ({ playwright, baseURL }, use) => {
    const request = await playwright.request.newContext({
      baseURL,
      storageState: stateOf(admin),
    })
    await use(new Gusto(request))
    await request.dispose()
  },
  runnerGusto: async ({ playwright, baseURL }, use) => {
    const request = await playwright.request.newContext({
      baseURL,
      storageState: stateOf(runner),
    })
    await use(new Gusto(request))
    await request.dispose()
  },
  // runnerPage is a second browser, signed in as the runner.
  runnerPage: async ({ browser }, use) => {
    const context = await browser.newContext({ storageState: stateOf(runner) })
    await use(await context.newPage())
    await context.close()
  },
})
