import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import type { SignIn } from "@/domain/sign-in"
import { SignInPage } from "@/pages/sign-in/sign-in-page"
import { renderPage } from "@/test/render"

const nobody: SignIn = {
  Wrong: false,
  Local: false,
  Identity: false,
  Provider: false,
  Desktop: false,
  Shared: false,
  Refused: "",
}

function signInAt(at: string, given: Partial<SignIn>) {
  return renderPage(at, "/login", <SignInPage />, {
    [at]: { SignIn: { ...nobody, ...given } },
  })
}

describe("the sign-in page", () => {
  it("leads with the provider, keeps the shared password a link away, and asks for nothing a session gives", async () => {
    const { asked } = signInAt("/login", { Identity: true, Provider: true })
    const provider = await screen.findByTestId("provider-sign-in")
    expect(provider.getAttribute("href")).toBe("/auth/start")
    expect(provider.textContent).toBe("Continue with Google")
    expect(screen.getByTestId("use-shared-password").getAttribute("href")).toBe(
      "/login?shared"
    )
    expect(screen.queryByTestId("shared-password")).toBeNull()
    expect(asked).toEqual(["/app/api/login"])
  })

  it("posts the shared password, and never puts it in an address", async () => {
    signInAt("/login?shared", { Identity: true, Provider: true, Shared: true })
    const password = (await screen.findByTestId(
      "shared-password"
    )) as HTMLInputElement
    expect(password.type).toBe("password")
    expect(password.name).toBe("password")
    expect(password.required).toBe(true)
    expect(password.placeholder).toBe("Shared password")
    const form = password.form!
    expect(form.method).toBe("post")
    expect(form.getAttribute("action")).toBe("/login")
    const button = screen.getByTestId("sign-in") as HTMLButtonElement
    expect(button.type).toBe("submit")
    expect(button.textContent).toBe("Sign in")
    expect(screen.getByTestId("provider-sign-in")).toBeTruthy()
    expect(screen.queryByTestId("use-shared-password")).toBeNull()
  })

  it("opens the shared password at once where there is no provider", async () => {
    signInAt("/login", { Shared: true })
    const password = await screen.findByTestId("shared-password")
    expect(document.activeElement).toBe(password)
    expect(screen.queryByTestId("provider-sign-in")).toBeNull()
    expect(screen.queryByTestId("use-shared-password")).toBeNull()
  })

  it("says in plain words that a password was wrong", async () => {
    signInAt("/login?wrong=1", { Wrong: true, Shared: true })
    expect((await screen.findByRole("alert")).textContent).toBe(
      "That password is not right. Check it and try again."
    )
  })

  it("sends a person whose gusto opens with a link to run gusto open", async () => {
    signInAt("/login?wrong=1", { Wrong: true, Local: true, Shared: true })
    const said = await screen.findByRole("alert")
    expect(said.textContent).toBe(
      "This copy of gusto opens with a link, not a password. Run gusto open for a new one."
    )
    expect(said.querySelector("code")?.textContent).toBe("gusto open")
  })

  it("says the refusal the server sealed", async () => {
    signInAt("/login?refused=x&refused-seal=y", {
      Refused: "that sign-in expired; try again",
      Shared: true,
    })
    expect(
      await screen.findByText("that sign-in expired; try again")
    ).toBeTruthy()
  })

  it("in the desktop app, continues in the browser, and its shared password goes back to the app", async () => {
    signInAt("/login?desktop=1", {
      Desktop: true,
      Identity: true,
      Provider: true,
      Shared: true,
    })
    const browser = await screen.findByText("Continue in your browser")
    expect(browser.getAttribute("href")).toBe("/auth/desktop/begin")
    expect(
      screen.getByText(
        "Your browser opens to sign you in, and brings you back here."
      )
    ).toBeTruthy()
    expect(screen.queryByTestId("provider-sign-in")).toBeNull()
    expect(
      (
        screen.getByTestId("shared-password") as HTMLInputElement
      ).form?.getAttribute("action")
    ).toBe("/login?desktop=1")
  })

  it("behind gusto cloud, offers the desktop app its browser and nothing else", async () => {
    signInAt("/login?desktop", { Desktop: true, Provider: true })
    expect(await screen.findByText("Continue in your browser")).toBeTruthy()
    expect(screen.queryByTestId("shared-password")).toBeNull()
    expect(screen.queryByTestId("use-shared-password")).toBeNull()
  })
})
