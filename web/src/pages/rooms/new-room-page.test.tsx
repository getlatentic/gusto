import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { NewRoomPage } from "@/pages/rooms/new-room-page"
import { renderPage } from "@/test/render"

describe("making a room", () => {
  it("asks for its name, and posts it to make the room", async () => {
    renderPage("/rooms/new", "/rooms/new", <NewRoomPage />, {
      "/rooms/new": { Refused: "" },
    })
    const name = (await screen.findByLabelText("Name it")) as HTMLInputElement
    expect(name.required).toBe(true)
    expect(name.maxLength).toBe(60)
    expect(name.form?.getAttribute("action")).toBe("/rooms")
    expect(screen.queryByRole("alert")).toBeNull()
  })

  it("says why the last name was refused", async () => {
    renderPage(
      "/rooms/new?refused=x&refused-seal=y",
      "/rooms/new",
      <NewRoomPage />,
      {
        "/rooms/new?refused=x&refused-seal=y": {
          Refused: "A room needs a name",
        },
      }
    )
    expect((await screen.findByRole("alert")).textContent).toBe(
      "A room needs a name"
    )
  })
})
