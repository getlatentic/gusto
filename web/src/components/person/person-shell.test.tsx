import { fireEvent, render, screen, within } from "@testing-library/react"
import { Provider } from "react-redux"
import {
  createMemoryRouter,
  createRoutesFromElements,
  Route,
} from "react-router"
import { RouterProvider } from "react-router/dom"
import { describe, expect, it, vi } from "vitest"

import { PersonShell } from "@/components/person/person-shell"
import { TooltipProvider } from "@/components/ui/tooltip"
import type { PersonShell as Shell, RailChat } from "@/domain/side-list"
import { makeStore } from "@/store"

const chat = (
  name: string,
  where: string,
  change: Partial<RailChat> = {}
): RailChat => ({
  Name: name,
  Where: where,
  When: 0,
  Room: true,
  Unread: { Count: 0, ForYou: 0 },
  ID: name,
  Harness: "",
  Mark: "",
  MarkDark: "",
  ...change,
})

const shell: Shell = {
  Rail: {
    Direct: [
      chat("Scout", "/instances/s/threads/1", {
        Harness: "claude",
        Unread: { Count: 2, ForYou: 1 },
      }),
    ],
    Rooms: [chat("Launch plan", "/instances/l/threads/1")],
    Archived: null,
    Latest: [chat("Launch plan", "/instances/l/threads/1")],
    Unread: 1,
    ForYou: 1,
    Recent: [
      {
        ID: "p1",
        Name: "Q4",
        Detail: "",
        Runs: "Codex",
        Chats: [chat("Plan", "/instances/p1/threads/2", { Room: false })],
      },
    ],
    Older: null,
  },
  SignedAs: { Name: "Ada", Initials: "A", Who: "ada@acme.com" },
  Admin: true,
  Connections: false,
  Org: null,
  Team: "",
  Teams: null,
}

function draw(at: string) {
  localStorage.clear()
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(shell)))
  )
  render(
    <Provider store={makeStore()}>
      <TooltipProvider>
        <RouterProvider
          router={createMemoryRouter(
            createRoutesFromElements(
              <>
                <Route element={<PersonShell />}>
                  <Route path="*" element={<p>page</p>} />
                </Route>
              </>
            ),
            { initialEntries: [at] }
          )}
        />
      </TooltipProvider>
    </Provider>
  )
}

describe("a person's shell", () => {
  it("marks the place a page is in, and lists their rooms and projects beside a conversation", async () => {
    draw("/instances/l/threads/1")
    const places = screen.getAllByRole("navigation", { name: "Places" })[0]
    expect(
      within(places)
        .getByRole("link", { name: "Chat" })
        .getAttribute("aria-current")
    ).toBe("page")
    const list = (
      await screen.findAllByRole("navigation", { name: "Conversations" })
    )[0]
    expect(
      within(list)
        .getByRole("link", { name: "Launch plan" })
        .getAttribute("aria-current")
    ).toBe("page")
    expect(
      within(list).getByRole("link", { name: /Scout/ }).textContent
    ).toContain("2")
    expect(within(list).getByRole("link", { name: "Plan" })).toBeTruthy()
    expect(
      within(list)
        .getByRole("link", { name: "New chat in Q4" })
        .getAttribute("href")
    ).toBe("/chat?at=p1")
  })

  it("counts a person's own room with an agent among the unread, and keeps the tab chosen", async () => {
    draw("/chat")
    fireEvent.click((await screen.findAllByRole("tab", { name: /Unread/ }))[0])
    const unread = screen.getAllByRole("navigation", {
      name: "Unread rooms",
    })[0]
    expect(within(unread).getByRole("link", { name: /Scout/ })).toBeTruthy()
    expect(localStorage.getItem("gusto-side-tab")).toBe("unread")
  })

  it("keeps the list of conversations to the pages of conversations", async () => {
    draw("/files")
    await screen.findAllByRole("link", { name: "Files" })
    expect(
      screen.getAllByRole("navigation", { name: "Places" })[0]
    ).toBeTruthy()
    expect(
      screen
        .queryAllByRole("navigation", { name: "Conversations" })
        .filter((nav) => !nav.closest("[role=dialog]"))
    ).toHaveLength(0)
  })
})

describe("a person's account and way round", () => {
  it("starts with a way past the side list, and says who is signed in", async () => {
    draw("/chat")
    const skip = screen.getAllByRole("link")[0]
    expect(skip.textContent).toBe("Skip to the page")
    expect(skip.getAttribute("href")).toBe("#main")
    const account = (
      await screen.findAllByRole("button", {
        name: "Ada, account and settings",
      })
    )[0]
    expect(account.getAttribute("title")).toBe("ada@acme.com")
    expect(account.textContent).toBe("A")
    expect(account.getAttribute("aria-expanded")).toBe("false")
  })

  it("offers an admin the console once, in the account menu", async () => {
    draw("/chat")
    fireEvent.click(
      (
        await screen.findAllByRole("button", {
          name: "Ada, account and settings",
        })
      )[0]
    )
    const admin = await screen.findAllByRole("link", { name: "Admin" })
    expect(admin).toHaveLength(1)
    expect(admin[0].getAttribute("href")).toBe("/admin")
    expect(
      screen.getByRole("link", { name: "Activity" }).getAttribute("href")
    ).toBe("/jobs")
  })

  it("lists direct messages under their own heading, and counts the unread on the tabs", async () => {
    draw("/chat")
    expect(
      (await screen.findAllByRole("heading", { name: "Direct messages" }))
        .length
    ).toBeGreaterThan(0)
    expect(
      screen.getAllByRole("tab", { name: "Unread 1" }).length
    ).toBeGreaterThan(0)
    expect(
      screen.getAllByRole("tab", { name: "@me 1" }).length
    ).toBeGreaterThan(0)
  })
})
