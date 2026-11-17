import { fireEvent, render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { cloudPage } from "@/cloud/fixture"
import {
  ConfirmPage,
  LeasePage,
  OrgsPage,
  StartingPage,
} from "@/cloud/pages/account-pages"
import { SignInPage } from "@/cloud/pages/sign-in-page"
import { SignUpPage } from "@/cloud/pages/sign-up-page"

const field = (testid: string) => screen.getByTestId(testid) as HTMLInputElement

describe("signing in", () => {
  it("asks for an email address, and keeps it when it is refused", () => {
    render(
      <SignInPage
        page={cloudPage("signin", {
          Email: "ada@",
          Error: "Enter an email address, such as you@company.com.",
        })}
      />
    )
    expect(field("email").value).toBe("ada@")
    expect(screen.getByTestId("refusal").textContent).toMatch(
      /Enter an email address/
    )
    expect(field("email").closest("form")?.getAttribute("action")).toBe(
      "/signin"
    )
  })

  it("takes the code once it is mailed, for the address it was mailed to", () => {
    render(
      <SignInPage
        page={cloudPage("signin", { Sent: true, Email: "ada@acme.com" })}
      />
    )
    expect(
      screen.getByRole("heading", { name: "Check your email" })
    ).toBeTruthy()
    const form = field("code").closest("form")!
    expect(form.getAttribute("action")).toBe("/signin/code")
    expect(
      (form.querySelector("input[name=email]") as HTMLInputElement).value
    ).toBe("ada@acme.com")
    expect(field("code").autocomplete).toBe("one-time-code")
  })
})

describe("signing up", () => {
  it("asks for a work email only where only one will do", () => {
    render(<SignUpPage page={cloudPage("signup", { WorkOnly: true })} />)
    expect(screen.getByLabelText("Work email")).toBeTruthy()
    expect(screen.queryByTestId("org-name")).toBeNull()
  })

  it("asks for the organization's name and keeps what was typed when it is refused", () => {
    render(
      <SignUpPage
        page={cloudPage("signup", {
          More: true,
          Email: "ada@gmail.com",
          OrgName: "Acme",
          Slug: "acme",
          Error: "That address is taken.",
        })}
      />
    )
    expect(field("email").value).toBe("ada@gmail.com")
    expect(field("org-name").value).toBe("Acme")
    expect(field("org-slug").value).toBe("acme")
    expect(screen.getByTestId("refusal").textContent).toBe(
      "That address is taken."
    )
  })

  it("asks for a name as the next step, without calling it a refusal", () => {
    render(
      <SignUpPage
        page={cloudPage("signup", {
          More: true,
          Email: "ada@gmail.com",
          Ask: "What should your organization be called? Your team sees this name.",
        })}
      />
    )
    expect(screen.getByTestId("sign-up-ask").textContent).toMatch(
      /^What should your organization be called\?/
    )
    expect(screen.queryByTestId("refusal")).toBeNull()
    expect(document.activeElement).toBe(screen.getByTestId("org-name"))
    expect(screen.getByLabelText("Web address, optional")).toBeTruthy()
  })

  it("offers a sign-in, and no second form, to an address that has an account", () => {
    render(
      <SignUpPage
        page={cloudPage("signup", { Taken: true, Email: "ada@acme.com" })}
      />
    )
    expect(screen.getByRole("heading", { name: "Welcome back" })).toBeTruthy()
    expect(
      screen.getByTestId("send-code").closest("form")?.getAttribute("action")
    ).toBe("/signin")
    expect(screen.queryByTestId("sign-up")).toBeNull()
  })
})

describe("Google", () => {
  it("is offered above the email form on both ways in where gusto cloud has it", () => {
    for (const page of [
      <SignInPage key="in" page={cloudPage("signin", { Google: true })} />,
      <SignUpPage key="up" page={cloudPage("signup", { Google: true })} />,
    ]) {
      const { unmount } = render(page)
      expect(screen.getByTestId("google").getAttribute("href")).toBe(
        "/signin/google"
      )
      expect(screen.getByTestId("google").textContent).toBe(
        "Continue with Google"
      )
      unmount()
    }
  })

  it("is not offered where gusto cloud has no client at Google, or once the sign-up asks for the organization", () => {
    render(<SignInPage page={cloudPage("signin")} />)
    expect(screen.queryByTestId("google")).toBeNull()
    render(
      <SignUpPage
        page={cloudPage("signup", {
          Google: true,
          More: true,
          Email: "ada@gmail.com",
        })}
      />
    )
    expect(screen.queryByTestId("google")).toBeNull()
  })
})

describe("confirming a link", () => {
  it("sends its form once, and says it is working", () => {
    render(
      <ConfirmPage
        page={cloudPage("confirm", {
          Title: "Join Acme",
          Token: "secret",
          Action: "/invite",
          Button: "Join",
          Inviter: "ada@acme.com",
        })}
      />
    )
    expect(screen.getByText("ada@acme.com invited you.")).toBeTruthy()
    const button = screen.getByTestId("confirm")
    const form = button.closest("form")!
    expect(form.getAttribute("action")).toBe("/invite")
    expect(
      (form.querySelector("input[name=t]") as HTMLInputElement).value
    ).toBe("secret")
    expect(fireEvent.submit(form)).toBe(true)
    expect(button.textContent).toBe("Just a moment…")
    expect(button.getAttribute("aria-disabled")).toBe("true")
    expect(fireEvent.submit(form)).toBe(false)
  })
})

describe("a person's organizations", () => {
  it("are each a way in and a way to their settings", () => {
    render(
      <OrgsPage
        page={cloudPage("orgs", {
          Email: "ada@acme.com",
          Address: "cloud.test",
          Orgs: [
            {
              Slug: "acme-com",
              Name: "Acme",
              Admin: "ada@acme.com",
              Demo: false,
            },
            { Slug: "beta", Name: "Beta", Admin: "bob@beta.com", Demo: false },
          ],
        })}
      />
    )
    expect(screen.getByText("ada@acme.com is in 2 organizations.")).toBeTruthy()
    expect(
      screen
        .getByRole("link", { name: /acme-com\.cloud\.test/ })
        .getAttribute("href")
    ).toBe("/open?org=acme-com")
    expect(
      screen.getByRole("link", { name: "Acme settings" }).getAttribute("href")
    ).toBe("/org?org=acme-com")
  })
})

describe("waiting and the lease", () => {
  it("says the server is starting, with the settings a click away", () => {
    render(
      <StartingPage
        page={cloudPage("starting", {
          Title: "Acme",
          Notice: "Starting your server",
        })}
      />
    )
    expect(screen.getByRole("status").textContent).toBe("Starting your server")
    expect(
      screen
        .getByRole("link", { name: "Organization settings" })
        .getAttribute("href")
    ).toBe("/org")
  })

  it("says how long the host has and offers to keep it", () => {
    render(
      <LeasePage
        page={cloudPage("lease", {
          Title: "This demo host",
          Lease: {
            Left: "20 minutes",
            Ends: "10:30 UTC",
            Step: "30 minutes",
            CanExtend: true,
          },
        })}
      />
    )
    expect(screen.getByTestId("lease-left").textContent).toBe(
      "It removes itself in 20 minutes, at 10:30 UTC."
    )
    expect(screen.getByTestId("extend").textContent).toBe(
      "Keep it for 30 minutes more"
    )
  })

  it("says when it cannot be kept longer", () => {
    render(
      <LeasePage
        page={cloudPage("lease", {
          Lease: { Left: "", Ends: "", Step: "", CanExtend: false },
        })}
      />
    )
    expect(screen.getByText("It is about to remove itself.")).toBeTruthy()
    expect(
      screen.getByText("It cannot be kept any longer than this.")
    ).toBeTruthy()
  })
})
