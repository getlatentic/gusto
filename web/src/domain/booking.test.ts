import { describe, expect, it } from "vitest"

import { calendarHeight, framedAt } from "@/domain/booking"

describe("a framed booking calendar", () => {
  it("tells Calendly the domain it is framed on, and leaves other services alone", () => {
    expect(
      framedAt(
        "https://calendly.com/tosin?embed_type=Inline",
        "gusto.example.com"
      )
    ).toBe(
      "https://calendly.com/tosin?embed_type=Inline&embed_domain=gusto.example.com"
    )
    expect(
      framedAt("https://calendar.google.com/x?gv=true", "gusto.example.com")
    ).toBe("https://calendar.google.com/x?gv=true")
  })

  it("reads the height Calendly says it is, and nothing from anyone else", () => {
    const said = { event: "calendly.page_height", payload: { height: "612px" } }
    expect(calendarHeight("https://calendly.com", said)).toBe(612)
    expect(calendarHeight("https://evil.example", said)).toBeNull()
    expect(
      calendarHeight("https://calendly.com", {
        event: "calendly.date_and_time_selected",
      })
    ).toBeNull()
    expect(
      calendarHeight("https://calendly.com", {
        event: "calendly.page_height",
        payload: { height: "big" },
      })
    ).toBeNull()
    expect(
      calendarHeight("https://calendly.com", "calendly.page_height")
    ).toBeNull()
  })
})
