import { fireEvent, render, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { cloudPage } from "@/cloud/fixture"
import { Contact } from "@/cloud/pages/landing/contact"
import type { CloudContact } from "@/cloud/types"

const blank: CloudContact = {
  Error: "",
  Email: "",
  Company: "",
  Note: "",
  Sent: false,
}

function landing(contact: Partial<CloudContact>, site = "the-site-key") {
  return cloudPage("landing", {
    Book: "https://cal.example/gusto",
    TurnstileSite: site,
    Contact: { ...blank, ...contact },
  })
}

const script = () =>
  document.head.querySelector(
    'script[src="https://challenges.cloudflare.com/turnstile/v0/api.js"]'
  )

// jsdom draws nothing, so it cannot scroll; each test sees whether the section asked to be scrolled to.
const scrolled = vi.fn()
beforeEach(() => {
  scrolled.mockClear()
  Element.prototype.scrollIntoView = scrolled
})
afterEach(() => script()?.remove())

describe("the note to the team", () => {
  it("is sent to /contact with a work email, a company and a note", () => {
    render(<Contact page={landing({})} />)
    const form = screen.getByTestId("contact-form")
    expect(form.getAttribute("action")).toBe("/contact")
    expect(form.getAttribute("method")).toBe("post")
    for (const [id, name] of [
      ["contact-email", "email"],
      ["contact-company", "company"],
      ["contact-note", "note"],
    ]) {
      expect(screen.getByTestId(id).getAttribute("name")).toBe(name)
    }
    expect(screen.getByRole("link", { name: "Book a call" })).toBeTruthy()
  })

  it("asks nothing of Cloudflare until the visitor types", () => {
    const { container } = render(<Contact page={landing({})} />)
    expect(container.querySelector(".cf-turnstile")).toBeNull()
    expect(script()).toBeNull()
    expect(scrolled).not.toHaveBeenCalled()
    fireEvent.input(screen.getByTestId("contact-email"), {
      target: { value: "m" },
    })
    expect(
      container.querySelector(".cf-turnstile")?.getAttribute("data-sitekey")
    ).toBe("the-site-key")
    expect(script()).not.toBeNull()
  })

  it("shows why a note was refused, with what was typed and the check", () => {
    const { container } = render(
      <Contact
        page={landing({
          Error: "Say what you would like to talk about.",
          Email: "maria@futureacmebank.com",
          Company: "Future Acme Bank",
        })}
      />
    )
    expect(screen.getByTestId("refusal").textContent).toBe(
      "Say what you would like to talk about."
    )
    expect(
      (screen.getByTestId("contact-email") as HTMLInputElement).value
    ).toBe("maria@futureacmebank.com")
    expect(container.querySelector(".cf-turnstile")).not.toBeNull()
    expect(scrolled).toHaveBeenCalledOnce()
  })

  it("thanks the visitor once the note is sent, in place of the form, where they see it", () => {
    render(<Contact page={landing({ Sent: true })} />)
    expect(scrolled).toHaveBeenCalledOnce()
    expect(screen.getByTestId("contact-sent").textContent).toMatch(
      /write back by email/
    )
    expect(screen.queryByTestId("contact-form")).toBeNull()
  })

  it("is not drawn where the server offers no note", () => {
    const { container } = render(
      <Contact page={cloudPage("landing", { Contact: null })} />
    )
    expect(container.textContent).toBe("")
  })
})
