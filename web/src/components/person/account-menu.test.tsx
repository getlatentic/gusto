import { fireEvent, render, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it } from "vitest"

import lookScript from "../../../../internal/web/static/look.js?raw"
import { AccountMenu } from "@/components/person/account-menu"
import type { PersonShell } from "@/domain/side-list"

const shell = {
  SignedAs: { Name: "Ada Lovelace", Initials: "AL", Who: "ada@acme.com" },
  Admin: false,
} as PersonShell

// The menu changes the look through the script the page head loads, so the test runs that script as the page would.
beforeEach(() => {
  localStorage.clear()
  new Function(lookScript)()
})

function open() {
  render(<AccountMenu shell={shell} />)
  fireEvent.click(
    screen.getByRole("button", { name: "Ada Lovelace, account and settings" })
  )
}

describe("the account menu", () => {
  it("says who is signed in", () => {
    render(<AccountMenu shell={shell} />)
    const who = screen.getByRole("button", {
      name: "Ada Lovelace, account and settings",
    })
    expect(who.getAttribute("title")).toBe("ada@acme.com")
    expect(who.textContent).toBe("AL")
  })

  it("sets the theme, the accent and the text size on the page and keeps them", async () => {
    open()
    fireEvent.click(await screen.findByRole("button", { name: "Dark" }))
    fireEvent.click(screen.getByRole("button", { name: "Blue" }))
    fireEvent.click(screen.getByRole("button", { name: "Larger" }))
    const page = document.documentElement
    expect(page.dataset.theme).toBe("dark")
    expect(page.dataset.accent).toBe("blue")
    expect(page.style.fontSize).toBe("110%")
    expect(screen.getByRole("button", { name: "Blue" }).ariaPressed).toBe(
      "true"
    )
    expect(localStorage.getItem("gusto-theme")).toBe("dark")
    expect(localStorage.getItem("gusto-accent")).toBe("blue")
  })
})
