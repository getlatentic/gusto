import { fireEvent, screen, within } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ConversationPage } from "@/pages/conversation/conversation-page"
import { conversationOf, roomOf, turnOf } from "@/pages/conversation/fixture"
import type { ConversationData } from "@/pages/conversation/types"
import { renderPage } from "@/test/render"

beforeEach(() => {
  vi.stubGlobal(
    "EventSource",
    class {
      addEventListener() {}
      close() {}
    }
  )
})

const route = "/instances/:id/threads/:thread"

function draw(page: ConversationData, query = "") {
  const at = `${page.Path}${query}`
  return renderPage(at, route, <ConversationPage />, {
    [at]: page,
    [`${page.Path}/turns`]: {
      Turns: page.Turns,
      Version: page.Version,
      Settings: page.Settings.Mark,
    },
  })
}

describe("a conversation with one agent", () => {
  it("is named in full in its bar, and says what each turn said and replied", async () => {
    const page = conversationOf()
    draw(page)
    const title = await screen.findByRole("heading", {
      level: 1,
      name: page.Named,
    })
    expect(title.getAttribute("title")).toBe(page.Named)
    const turn = document.getElementById("t1")!
    expect(within(turn).getByText("Draft the policy")).toBeTruthy()
    expect(within(turn).getByText("Here it is.")).toBeTruthy()
    expect(turn.textContent).toContain("12s · $0.02")
  })

  it("says where the agent runs beside Files, in words, with the folder it works in", async () => {
    draw(conversationOf())
    const sandbox = await screen.findByRole("button", { name: "Sandboxed" })
    expect(
      screen.getByRole("link", { name: "Files" }).getAttribute("href")
    ).toBe("/instances/a/files?from=%2Finstances%2Fa%2Fthreads%2F1")
    fireEvent.click(sandbox)
    expect(
      (await screen.findByText(/Runs in its own sandbox/)).textContent
    ).toMatch(/model's key never reaches the agent/)
    expect(screen.getByText("docs")).toBeTruthy()
  })

  it("writes to the agent by name, and sends what is written", async () => {
    draw(conversationOf())
    const box = (await screen.findByRole("textbox", {
      name: "Message Scout",
    })) as HTMLTextAreaElement
    expect(box.form?.getAttribute("action")).toBe("/instances/a/threads/1/say")
    const send = screen.getByRole("button", {
      name: "Send",
    }) as HTMLButtonElement
    expect(send.disabled).toBe(true)
    fireEvent.change(box, { target: { value: "Shorter, please" } })
    expect(send.disabled).toBe(false)
  })

  it("shows a reply being written, and offers to stop it", async () => {
    draw(
      conversationOf((page) => ((page.Waiting = true), (page.Latest = "t2")))
    )
    expect(
      await screen.findByRole("region", { name: "The reply so far" })
    ).toBeTruthy()
    expect(screen.getByRole("button", { name: "Stop" })).toBeTruthy()
  })

  it("says once that a reply is being written, inside the agent's own reply", async () => {
    draw(
      conversationOf((page) => {
        page.Waiting = true
        page.Latest = "t2"
        page.Turns = [turnOf("t2", { Done: false, ReplyHTML: "" })]
      })
    )
    const reply = await screen.findByTestId("reply")
    expect(
      within(reply).getByRole("region", { name: "The reply so far" })
    ).toBeTruthy()
    expect(
      screen.getAllByRole("status").filter((s) => s.textContent === "Working")
    ).toHaveLength(1)
    expect(within(reply).getAllByRole("button", { name: "Stop" })).toHaveLength(
      1
    )
  })

  it("greets an empty conversation, and writes a starter in the box", async () => {
    draw(
      conversationOf(
        (page) => (
          (page.Turns = null),
          (page.Starters = ["Summarize the folder"])
        )
      )
    )
    expect(
      await screen.findByRole("heading", { name: "How can I help?" })
    ).toBeTruthy()
    fireEvent.click(
      screen.getByRole("button", { name: "Summarize the folder" })
    )
    expect(
      (
        screen.getByRole("textbox", {
          name: "Message Scout",
        }) as HTMLTextAreaElement
      ).value
    ).toBe("Summarize the folder")
  })
})

describe("a room", () => {
  it("offers agents first when @ is typed, and Enter chooses without sending", async () => {
    draw(roomOf())
    const box = (await screen.findByRole("combobox", {
      name: "Message the room",
    })) as HTMLTextAreaElement
    fireEvent.change(box, { target: { value: "hi @S", selectionStart: 5 } })
    const options = screen
      .getAllByRole("option")
      .map((option) => option.textContent)
    expect(options[0]).toContain("Scout")
    expect(options[1]).toContain("Sam")
    fireEvent.keyDown(box, { key: "Enter" })
    expect(box.value).toBe("hi @Scout ")
    expect(screen.queryByRole("listbox", { name: "Mention" })).toBeNull()
  })

  it("refuses a file with an @ in its name, and says why", async () => {
    draw(roomOf())
    const input = (await screen.findByLabelText(
      "Attach files"
    )) as HTMLInputElement
    fireEvent.change(input, { target: { files: [new File(["x"], "a@b.txt")] } })
    expect((await screen.findByRole("alert")).textContent).toMatch(
      /has an @ in its name/
    )
  })

  it("says whose agent replied, as the person looking reads it, and offers to stop one at work", async () => {
    draw(
      roomOf((page) => {
        page.Turns = [
          turnOf("t1", { Owner: "ada@acme.com", OwnerName: "Ada" }),
          turnOf("t2", {
            Owner: "sam@acme.com",
            OwnerName: "Sam",
            Done: false,
          }),
        ]
      })
    )
    await screen.findByText("your agent")
    expect(
      within(document.getElementById("t1")!).getByText("your agent")
    ).toBeTruthy()
    const working = document.getElementById("t2")!
    expect(within(working).getByText("Sam's agent")).toBeTruthy()
    expect(within(working).getByText("Working")).toBeTruthy()
    expect(
      within(working)
        .getByRole("button", { name: "Stop" })
        .closest("form")!
        .getAttribute("action")
    ).toBe("/jobs/t2/stop")
  })

  it("puts the box away while archived, and offers an admin to bring the room back", async () => {
    draw(roomOf((page) => (page.Settings.Archived = true)))
    expect(await screen.findByText(/This room is archived/)).toBeTruthy()
    expect(
      screen.queryByRole("combobox", { name: "Message the room" })
    ).toBeNull()
    expect(
      screen
        .getByRole("button", { name: "Unarchive" })
        .closest("form")!
        .getAttribute("action")
    ).toBe("/rooms/a/archive")
  })

  it("opens its members from the address, with why a change was refused", async () => {
    draw(
      roomOf((page) => (page.Refused = "A room keeps one admin")),
      "?members=1"
    )
    const dialog = await screen.findByRole("dialog")
    expect(
      within(dialog).getByRole("heading", { name: "Room members" })
    ).toBeTruthy()
    expect(within(dialog).getByRole("alert").textContent).toBe(
      "A room keeps one admin"
    )
    expect(within(dialog).getByText("you")).toBeTruthy()
  })
})
