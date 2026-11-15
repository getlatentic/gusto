import { fireEvent, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { ChatPage } from "@/pages/chat/chat-page"
import type { ChatData } from "@/pages/chat/types"
import { renderPage } from "@/test/render"

function chat(change: (page: ChatData) => void = () => {}): ChatData {
  const page: ChatData = {
    Ready: true,
    Harness: "claude",
    Model: "sonnet",
    Agents: [
      { value: "claude", label: "Claude Code" },
      { value: "codex", label: "Codex" },
    ],
    Models: {
      claude: [{ value: "sonnet", label: "Claude Sonnet" }],
      codex: [{ value: "gpt", label: "GPT" }],
    },
    Refused: "",
    Start: { At: "", Name: "", Harness: "" },
    Roles: [
      { value: "", label: "No preset" },
      { kind: "heading", value: "", label: "Built-in presets" },
      { value: "r:eng", label: "Engineer", role: "engineer", fit: ["codex"] },
    ],
    Role: "",
    NeedsModel: false,
    Presets: [
      {
        Title: "Anthropic",
        Address: "https://api.anthropic.com",
        Auth: "anthropic",
      },
    ],
    Missing: [],
    HandInstall: false,
    FoundKeys: null,
    Lacking: null,
    Wall: null,
  }
  change(page)
  return page
}

const draw = (page: ChatData, at = "/chat") =>
  renderPage(at, "/chat", <ChatPage />, { [at]: page })

const field = (name: string) =>
  document.querySelector(`input[name="${name}"]`) as HTMLInputElement

describe("a new chat", () => {
  it("asks what we are working on, and sends only what is written", async () => {
    draw(chat())
    expect(
      await screen.findByRole("heading", { name: "What are we working on?" })
    ).toBeTruthy()
    const box = screen.getByRole("textbox", {
      name: "Message",
    }) as HTMLTextAreaElement
    const send = screen.getByRole("button", {
      name: "Send",
    }) as HTMLButtonElement
    expect(send.disabled).toBe(true)
    fireEvent.input(box, { target: { value: "Hello" } })
    expect(send.disabled).toBe(false)
    expect(box.form?.getAttribute("action")).toBe("/chat")
  })

  it("writes a starter in the box, and sends nothing", async () => {
    draw(chat())
    fireEvent.click(
      await screen.findByRole("button", { name: "Summarize this folder" })
    )
    expect(
      (screen.getByRole("textbox", { name: "Message" }) as HTMLTextAreaElement)
        .value
    ).toBe("Summarize this folder")
  })

  it("says in one line what it runs on, and carries the choice with the message", async () => {
    draw(chat())
    expect(
      await screen.findByRole("button", {
        name: "Runs on Claude Code · Claude Sonnet. Change",
      })
    ).toBeTruthy()
    expect(field("harness").value).toBe("claude")
    expect(field("model").value).toBe("sonnet")
    expect(field("choice").value).toBe("")
  })

  it("opens in a project's folder, which keeps the agent it has", async () => {
    draw(
      chat(
        (page) => (page.Start = { At: "p1", Name: "Launch", Harness: "codex" })
      ),
      "/chat?at=p1"
    )
    expect(await screen.findByRole("button", { name: "Launch" })).toBeTruthy()
    expect(field("at").value).toBe("p1")
    expect(field("harness").value).toBe("codex")
    fireEvent.click(screen.getByRole("button", { name: /^Runs on/ }))
    expect(
      (screen.getByRole("combobox", { name: /^Agent/ }) as HTMLButtonElement)
        .title
    ).toMatch(/Set by this folder/)
    fireEvent.click(screen.getByRole("button", { name: "Do not use a folder" }))
    expect(field("at").value).toBe("")
  })

  it("starts with what the address suggests, and says why the last message was refused", async () => {
    draw(
      chat((page) => (page.Refused = "That agent is busy")),
      "/chat?prompt=Plan%20Q4"
    )
    const alert = await screen.findByRole("alert")
    expect(alert.textContent).toBe("That agent is busy")
    const box = screen.getByRole("textbox", {
      name: "Message",
    }) as HTMLTextAreaElement
    expect(box.value).toBe("Plan Q4")
    expect(box.getAttribute("aria-describedby")).toBe("refused")
  })

  it("asks for the first model before anything, with the provider filling in where it answers", async () => {
    draw(chat((page) => (page.NeedsModel = true)))
    expect(
      await screen.findByRole("heading", { name: "Add the model you use" })
    ).toBeTruthy()
    expect(screen.queryByLabelText("Address")).toBeNull()
    expect(field("address").value).toBe("https://api.anthropic.com")
    expect(field("auth").value).toBe("anthropic")
    fireEvent.change(screen.getByLabelText("Provider"), {
      target: { value: "another" },
    })
    expect(screen.getByLabelText("Address")).toBeTruthy()
    expect(screen.getByLabelText("Answers like")).toBeTruthy()
  })

  it("asks to agree before agents run with no sandbox", async () => {
    draw(
      chat(
        (page) =>
          (page.Wall = {
            Short: "no sandbox",
            Words: "Nothing contains them.",
            Missing: ["No VM"],
            Unconfirmed: true,
          })
      )
    )
    expect(
      await screen.findByRole("heading", {
        name: "Agents will run with no sandbox",
      })
    ).toBeTruthy()
    expect(
      screen
        .getByRole("button", {
          name: "I understand, start agents with no sandbox",
        })
        .closest("form")!
        .getAttribute("action")
    ).toBe("/setup/no-sandbox")
    expect(screen.queryByRole("textbox", { name: "Message" })).toBeNull()
  })
})

describe("what a new chat says about where it runs", () => {
  it("says what the computer lacks", async () => {
    draw(chat((page) => (page.Lacking = ["gusto needs Git to keep a record."])))
    expect(
      await screen.findByText("gusto needs Git to keep a record.")
    ).toBeTruthy()
  })

  it("says what agents run in, and why nothing stronger", async () => {
    draw(
      chat(
        (page) =>
          (page.Wall = {
            Short: "no sandbox",
            Words: "w",
            Missing: ["neither Docker nor Podman is installed"],
            Unconfirmed: false,
          })
      )
    )
    expect(await screen.findByText("Agents run in no sandbox.")).toBeTruthy()
    expect(screen.getByText("Why not stronger?")).toBeTruthy()
    expect(
      screen.getByText("neither Docker nor Podman is installed.")
    ).toBeTruthy()
  })

  it("offers to install an agent it can, and says the command for one it cannot", async () => {
    draw(
      chat((page) => {
        page.NeedsModel = true
        page.FoundKeys = ["ANTHROPIC_API_KEY"]
        page.Missing = [
          { Name: "claude", Title: "Claude Code", Why: "", Installable: true },
          { Name: "pi", Title: "Pi", Why: "npm i -g pi", Installable: false },
        ]
      })
    )
    const install = await screen.findByRole("button", { name: "Install" })
    const form = document.getElementById(
      install.getAttribute("form")!
    ) as HTMLFormElement
    expect(form.getAttribute("action")).toBe("/agents/claude/install")
    expect((form.elements.namedItem("back") as HTMLInputElement).value).toBe(
      "/chat"
    )
    expect(screen.getByText("npm i -g pi")).toBeTruthy()
    expect(screen.getByText("gusto local use ANTHROPIC_API_KEY")).toBeTruthy()
    expect(screen.queryByRole("textbox", { name: "Message" })).toBeNull()
  })
})
