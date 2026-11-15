import { fireEvent, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import type { Connection } from "@/domain/connections"
import { ConnectionsPage } from "@/pages/account/connections-page"
import type { ConnectionsData } from "@/pages/account/types"
import { renderPage } from "@/test/render"

const linear: Connection = {
  ID: "linear",
  Name: "Linear",
  What: "Issues, projects and cycles in your Linear workspace",
  Offered: true,
  Connected: false,
  SignInAgain: false,
  ForAgents: false,
  Since: 0,
}

function page(change: (page: ConnectionsData) => void = () => {}) {
  const data: ConnectionsData = {
    Person: true,
    Connectors: [linear],
    Refused: "",
    Done: "",
    Home: null,
    HomeReset: false,
  }
  change(data)
  return data
}

function draw(data: ConnectionsData, at = "/connections") {
  return renderPage(at, "/connections", <ConnectionsPage />, { [at]: data })
}

const rowOf = async (id: string) =>
  (await screen.findAllByTestId("connection")).find(
    (row) => row.getAttribute("data-app") === id
  )!

describe("Connections", () => {
  it("offers to connect an app an admin turned on, by a link to the app's sign-in", async () => {
    draw(page())
    const row = await rowOf("linear")
    expect(within(row).getAllByText(linear.What).length).toBeGreaterThan(0)
    expect(within(row).getByText("not yet")).toBeTruthy()
    expect(
      within(row).getByRole("link", { name: "Connect" }).getAttribute("href")
    ).toBe("/connections/linear/connect")
    expect(within(row).queryByRole("button", { name: "Disconnect" })).toBeNull()
    expect(within(row).queryByRole("combobox")).toBeNull()
    for (const heading of [
      "App",
      "What your agents reach",
      "Connected",
      "For my agents",
    ]) {
      expect(screen.getByRole("columnheader", { name: heading })).toBeTruthy()
    }
  })

  it("says the person is signed in when the app sends them back", async () => {
    draw(
      page((data) => {
        data.Done = "Linear"
        data.Connectors = [{ ...linear, Connected: true, Since: 1 }]
      })
    )
    expect(
      await screen.findByText(
        "You are signed in to Linear. Your agents use it from their next task."
      )
    ).toBeTruthy()
    const row = await rowOf("linear")
    expect(within(row).getByTestId("connected-since")).toBeTruthy()
    const disconnect = within(row).getByRole("button", { name: "Disconnect" })
    expect(disconnect.closest("form")?.getAttribute("action")).toBe(
      "/connections/linear/disconnect"
    )
    expect(within(row).queryByRole("link", { name: "Connect" })).toBeNull()
  })

  it("sends whether the person's agents may use an app as soon as it is chosen", async () => {
    draw(page((data) => (data.Connectors = [{ ...linear, Connected: true }])))
    const choice = within(await rowOf("linear")).getByRole("combobox", {
      name: "Whether your agents may use Linear, unless you choose otherwise for one",
    })
    expect(choice.textContent).toContain("Off")
    const form = choice.closest("form")!
    expect(form.getAttribute("action")).toBe("/connections/linear/agents")
    const sent = vi
      .spyOn(HTMLFormElement.prototype, "requestSubmit")
      .mockImplementation(() => {})
    fireEvent.click(choice)
    await userEvent.click(await screen.findByRole("option", { name: "On" }))
    expect(sent).toHaveBeenCalledTimes(1)
    expect((form.querySelector('[name="on"]') as HTMLInputElement).value).toBe(
      "on"
    )
  })

  it("asks the person to sign in again to an app that stopped taking their sign-in, or to end it", async () => {
    draw(page((data) => (data.Connectors = [{ ...linear, SignInAgain: true }])))
    const row = await rowOf("linear")
    expect(within(row).getByText("sign in again")).toBeTruthy()
    expect(
      within(row)
        .getByRole("link", { name: "Sign in again" })
        .getAttribute("href")
    ).toBe("/connections/linear/connect")
    expect(within(row).getByRole("button", { name: "Disconnect" })).toBeTruthy()
    expect(within(row).queryByRole("combobox")).toBeNull()
  })

  it("lets the person end a sign-in to an app an admin turned off, and nothing else", async () => {
    draw(
      page(
        (data) =>
          (data.Connectors = [{ ...linear, Offered: false, Connected: true }])
      )
    )
    const row = await rowOf("linear")
    expect(within(row).getByText("turned off")).toBeTruthy()
    expect(within(row).getByRole("button", { name: "Disconnect" })).toBeTruthy()
    expect(within(row).queryByRole("link")).toBeNull()
    expect(within(row).queryByRole("combobox")).toBeNull()
  })

  it("says when no app is turned on", async () => {
    draw(page((data) => (data.Connectors = [])))
    expect((await screen.findByTestId("no-connections")).textContent).toBe(
      "No app is turned on yet. An admin turns them on under Admin, Connectors."
    )
  })

  it("tells the shared sign-in to sign in as a person, and why the last thing was refused", async () => {
    draw(
      page((data) => {
        data.Person = false
        data.Refused =
          "Sign in as yourself to connect your own accounts: the shared sign-in has none."
      })
    )
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Sign in as yourself to connect your own accounts: the shared sign-in has none."
    )
    expect(
      screen.getByText(
        "You are signed in with the shared password, which is nobody's own. Sign in as yourself to connect your accounts."
      )
    ).toBeTruthy()
  })
})

describe("the agents' home", () => {
  const counted = {
    Counted: true,
    Used: "6.0 GiB",
    Limit: "5.0 GiB",
    Full: true,
    At: Math.floor(Date.now() / 1000) - 120,
  }

  it("says how much of its limit it takes, and when it is full", async () => {
    draw(page((data) => (data.Home = counted)))
    const home = await screen.findByTestId("home")
    expect(
      within(home).getByRole("heading", { name: "Your agents' home" })
    ).toBeTruthy()
    expect(home.textContent).toContain(
      "It takes 6.0 GiB of the 5.0 GiB it may, counted 2m ago."
    )
    expect(within(home).getByRole("alert").textContent).toBe(
      "It is full, so a task you ask for is refused until you reset it."
    )
  })

  it("asks before resetting, and says what resetting deletes", async () => {
    draw(page((data) => (data.Home = { ...counted, Full: false })))
    const home = await screen.findByTestId("home")
    expect(within(home).queryByRole("alert")).toBeNull()
    const sent = vi.spyOn(globalThis, "fetch")
    fireEvent.click(
      within(home).getByRole("button", { name: "Reset the home" })
    )
    const asking = await screen.findByRole("alertdialog")
    expect(within(asking).getByText("Reset your agents' home?")).toBeTruthy()
    expect(
      within(asking).getByText(
        "What they kept in it is deleted: caches, downloads and tools they installed."
      )
    ).toBeTruthy()
    expect(sent.mock.calls.some(([, init]) => init?.method === "POST")).toBe(
      false
    )
    expect(screen.getByTestId("reset-home").getAttribute("action")).toBe(
      "/connections/home/reset"
    )
  })

  it("says when it is reset, and when nothing is counted yet", async () => {
    draw(
      page((data) => {
        data.Home = { Counted: false, Used: "", Limit: "", Full: false, At: 0 }
        data.HomeReset = true
      })
    )
    expect(
      await screen.findByText(
        "Your agents' home is reset. The next task starts with an empty one."
      )
    ).toBeTruthy()
    expect(
      screen.getByText(
        "Nothing is counted yet. It is counted when your first task ends."
      )
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Reset the home" })).toBeNull()
  })
})
