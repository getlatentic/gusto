import { fireEvent, render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { cloudPage } from "@/cloud/fixture"
import { DemoBookPage } from "@/cloud/pages/demo-book-page"
import { DemoFormPage } from "@/cloud/pages/demo-form-page"
import {
  DemoColleaguePage,
  DemoEndPage,
  DemoWaitPage,
} from "@/cloud/pages/demo-pages"

const org = {
  Slug: "acme-demo",
  Name: "Acme",
  Admin: "ada@acme.com",
  Demo: true,
}

afterEach(() => vi.unstubAllGlobals())

describe("the demo form", () => {
  it("asks for a node once, when the visitor first types", () => {
    const fetch = vi.fn(() => Promise.resolve(new Response()))
    vi.stubGlobal("fetch", fetch)
    render(
      <DemoFormPage page={cloudPage("demo", { DemoLeft: "30 minutes" })} />
    )
    fireEvent.input(screen.getByTestId("demo-name"), { target: { value: "A" } })
    fireEvent.input(screen.getByTestId("demo-name"), {
      target: { value: "Ad" },
    })
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch).toHaveBeenCalledWith("/demo/warm", {
      method: "POST",
      credentials: "same-origin",
    })
  })

  it("keeps what was typed when it is refused, and carries the bot check", () => {
    const { container } = render(
      <DemoFormPage
        page={cloudPage("demo", {
          Name: "Ada",
          Email: "ada@gmail.com",
          OrgName: "Acme",
          WorkOnly: true,
          Error: "Use your work email.",
          TurnstileSite: "0x4AAA",
        })}
      />
    )
    expect((screen.getByTestId("demo-email") as HTMLInputElement).value).toBe(
      "ada@gmail.com"
    )
    expect(screen.getByLabelText("Work email")).toBeTruthy()
    expect(screen.getByTestId("refusal").textContent).toBe(
      "Use your work email."
    )
    expect(
      container.querySelector(".cf-turnstile")?.getAttribute("data-sitekey")
    ).toBe("0x4AAA")
    expect(
      document.head.querySelector(
        'script[src="https://challenges.cloudflare.com/turnstile/v0/api.js"]'
      )
    ).toBeTruthy()
  })
})

describe("a demo's own page", () => {
  it("says how far it is from ready while it starts", () => {
    render(
      <DemoWaitPage
        page={cloudPage("demo-wait", {
          DemoWords: "Your demo is being set up.",
          Steps: [
            { Label: "Getting a computer ready", Mark: "done" },
            { Label: "Setting up your demo", Mark: "now" },
          ],
        })}
      />
    )
    expect(screen.getByTestId("demo-state").textContent).toBe(
      "Your demo is being set up."
    )
    expect(
      screen.getByText("Setting up your demo").getAttribute("aria-current")
    ).toBe("step")
  })

  it("then gives the time left, the way in and ten more minutes", () => {
    render(
      <DemoWaitPage
        page={cloudPage("demo-wait", {
          Org: org,
          DemoLeft: "25 minutes",
          CanOpen: true,
          Extendable: true,
        })}
      />
    )
    expect(screen.getByTestId("demo-left").textContent).toMatch(
      /You have 25 minutes left/
    )
    expect(screen.getByTestId("open-demo").getAttribute("href")).toBe(
      "/open?org=acme-demo"
    )
    expect(
      screen.getByTestId("extend-demo").closest("form")?.getAttribute("action")
    ).toBe("/demo/extend")
  })
})

describe("booking a demo", () => {
  it("frames the booking page where it may be framed", () => {
    const { container } = render(
      <DemoBookPage
        page={cloudPage("demo-book", {
          Book: "https://cal.example/gusto",
          BookFrame: "https://cal.example/gusto?embed=1",
        })}
      />
    )
    expect(container.querySelector("iframe")?.getAttribute("src")).toBe(
      "https://cal.example/gusto?embed=1"
    )
    expect(
      screen
        .getByRole("link", { name: "Open it in a new tab" })
        .getAttribute("href")
    ).toBe("https://cal.example/gusto")
  })

  it("links to it where it may not be framed", () => {
    const { container } = render(
      <DemoBookPage
        page={cloudPage("demo-book", { Book: "https://cal.example/gusto" })}
      />
    )
    expect(container.querySelector("iframe")).toBeNull()
    expect(
      screen.getByRole("link", { name: "Book a demo" }).getAttribute("href")
    ).toBe("https://cal.example/gusto")
  })
})

describe("bringing a colleague", () => {
  it("lets the person who started the demo invite one", () => {
    render(
      <DemoColleaguePage
        page={cloudPage("demo-colleague", {
          Org: org,
          IsAdmin: true,
          DemoLeft: "20 minutes",
        })}
      />
    )
    expect(
      screen
        .getByTestId("colleague-email")
        .closest("form")
        ?.getAttribute("action")
    ).toBe("/demo/colleague")
  })

  it("shows the link that works once", () => {
    render(
      <DemoColleaguePage
        page={cloudPage("demo-colleague", {
          Org: org,
          IsAdmin: true,
          Email: "bob@acme.com",
          Link: "https://cloud.test/invite?t=x",
        })}
      />
    )
    expect(
      (screen.getByTestId("colleague-link") as HTMLInputElement).value
    ).toBe("https://cloud.test/invite?t=x")
  })

  it("tells anyone else only the person who started it can", () => {
    render(
      <DemoColleaguePage page={cloudPage("demo-colleague", { Org: org })} />
    )
    expect(
      screen.getByText(/Only the person who started this demo/)
    ).toBeTruthy()
    expect(screen.queryByTestId("colleague-email")).toBeNull()
  })
})

describe("the end of a demo", () => {
  it("says it is being deleted while it is", () => {
    render(
      <DemoEndPage
        page={cloudPage("demo-end", {
          Ending: true,
          Book: "https://cal.example",
        })}
      />
    )
    expect(screen.getByRole("status").textContent).toMatch(
      /Deleting your computers/
    )
    expect(screen.queryByRole("link", { name: "Book a call" })).toBeNull()
  })

  it("thanks the visitor and offers a call once it has ended", () => {
    render(
      <DemoEndPage
        page={cloudPage("demo-end", {
          Ended: true,
          Name: "Ada",
          Book: "https://cal.example",
        })}
      />
    )
    expect(screen.getByTestId("demo-ended").textContent).toBe(
      "Thanks, Ada. Everything in it has been deleted."
    )
    expect(
      screen.getByText(/We keep your name, work email and company/)
    ).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "Book a call" }).getAttribute("href")
    ).toBe("https://cal.example")
  })
})
