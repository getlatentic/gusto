import { describe, expect, it, vi } from "vitest"

import { sendForm } from "@/lib/forms"

function form(html: string): HTMLFormElement {
  document.body.innerHTML = html
  return document.querySelector("form")!
}

describe("sending a form without leaving the page", () => {
  it("sends its fields and gives where the server sent the person", async () => {
    const answered = vi.fn(async (_: string, init: RequestInit) => {
      expect(String(init.body)).toBe("model=fast&back=%2Fx")
      return Object.defineProperty(new Response("page"), "url", {
        value: `${location.origin}/x?refused=no&refused-seal=s`,
      })
    })
    vi.stubGlobal("fetch", answered)
    const where = await sendForm(
      form(
        '<form method="post" action="/instances/a/model"><input name="model" value="fast"><input name="back" value="/x"></form>'
      )
    )
    expect(where).toBe("/x?refused=no&refused-seal=s")
  })

  it("says why the server refused outright", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("not yours", { status: 403 }))
    )
    await expect(
      sendForm(form('<form method="post" action="/rooms/a/mute"></form>'))
    ).rejects.toThrow("not yours")
  })
})
