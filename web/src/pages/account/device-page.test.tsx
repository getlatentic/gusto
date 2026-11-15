import { cleanup, fireEvent, screen, within } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { DevicePage } from "@/pages/account/device-page"
import type { DeviceData } from "@/pages/account/types"
import { renderPage } from "@/test/render"

function page(change: (page: DeviceData) => void = () => {}) {
  const data: DeviceData = {
    Code: "",
    Refused: "",
    Approved: "",
    Shared: false,
    Owner: false,
    Tokens: [
      {
        ID: "t1",
        Name: "laptop",
        Made: "15 Nov 2026 17:03 UTC",
        LastUsed: "",
        Ended: "",
        Live: true,
      },
      {
        ID: "t2",
        Name: "old-desktop",
        Made: "1 Oct 2026 09:00 UTC",
        LastUsed: "2 Oct 2026 10:00 UTC",
        Ended: "3 Oct 2026 11:00 UTC by ada@acme.com",
        Live: false,
      },
    ],
  }
  change(data)
  return data
}

function draw(data: DeviceData, at = "/device") {
  return renderPage(at, "/device", <DevicePage />, { [at]: data })
}

describe("your computers", () => {
  it("approves a computer by the code it shows, filled in when the computer sent the person here", async () => {
    draw(
      page((data) => (data.Code = "BCDF-GHJK")),
      "/device?code=BCDF-GHJK"
    )
    const form = await screen.findByTestId("approve-form")
    expect(form.getAttribute("action")).toBe("/device")
    expect(form.getAttribute("method")).toBe("post")
    const code = within(form).getByLabelText(
      "The code your computer shows"
    ) as HTMLInputElement
    expect(code.name).toBe("code")
    expect(code.value).toBe("BCDF-GHJK")
    expect(code.required).toBe(true)
    expect(within(form).getByRole("button", { name: "Approve" })).toBeTruthy()
  })

  it("says which computer was approved, and why a code was refused", async () => {
    draw(page((data) => (data.Approved = "laptop")))
    expect((await screen.findByTestId("device-approved")).textContent).toBe(
      "Approved laptop. Go back to the computer: it signs in on its own."
    )
    cleanup()
    draw(
      page(
        (data) =>
          (data.Refused =
            "That code is not waiting. Start the sign-in again on your computer and use the new code.")
      ),
      "/device?refused=x"
    )
    expect((await screen.findByRole("alert")).textContent).toBe(
      "That code is not waiting. Start the sign-in again on your computer and use the new code."
    )
  })

  it("lists the computers approved, ends a live one, and says when an ended one was ended", async () => {
    draw(page())
    const [live, ended] = await screen.findAllByTestId("computer")
    expect(within(live).getByText("never")).toBeTruthy()
    expect(
      within(live)
        .getByRole("button", { name: "End" })
        .closest("form")
        ?.getAttribute("action")
    ).toBe("/device/t1/revoke")
    expect(within(ended).queryByRole("button", { name: "End" })).toBeNull()
    expect(within(ended).getByText(/^ended 3 Oct 2026/)).toBeTruthy()
  })

  it("says none is approved yet", async () => {
    draw(page((data) => (data.Tokens = [])))
    expect(await screen.findByText("None yet.")).toBeTruthy()
  })

  it("asks before signing out everywhere, and says what it ends", async () => {
    draw(page())
    const sent = vi.spyOn(globalThis, "fetch")
    const submitted = vi
      .spyOn(HTMLFormElement.prototype, "submit")
      .mockImplementation(() => {})
    fireEvent.click(
      await screen.findByRole("button", { name: "Sign out everywhere" })
    )
    const asking = await screen.findByRole("alertdialog")
    expect(
      within(asking).getByText(
        "Every browser and the app sign in again, and every computer you approved is ended."
      )
    ).toBeTruthy()
    expect(submitted).not.toHaveBeenCalled()
    fireEvent.click(screen.getByTestId("sign-out-everywhere-confirm"))
    expect(submitted).toHaveBeenCalledTimes(1)
    expect(sent.mock.calls.some(([, init]) => init?.method === "POST")).toBe(
      false
    )
    expect(
      screen.getByTestId("sign-out-everywhere").getAttribute("action")
    ).toBe("/device/sign-out-everywhere")
  })

  it("gives the shared sign-in no computers, and says the owner's computer acts as the owner", async () => {
    draw(page((data) => (data.Shared = true)))
    expect(
      await screen.findByText(
        "The shared sign-in is not one person, so it has no computers. Sign in with your own account."
      )
    ).toBeTruthy()
    expect(screen.queryByTestId("approve-form")).toBeNull()
    cleanup()
    draw(
      page((data) => (data.Owner = true)),
      "/device?owner"
    )
    expect(
      await screen.findByText(
        "This server has no sign-in provider, so the shared password is its one owner. A computer you approve acts as that owner."
      )
    ).toBeTruthy()
  })
})
