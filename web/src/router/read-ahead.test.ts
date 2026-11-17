import { describe, expect, it, vi } from "vitest"

import { readAhead } from "@/router/read-ahead"
import { makeStore } from "@/store"

describe("reading a page's data ahead", () => {
  it("starts reading the address the router moves to, with its query, and does not wait for it", async () => {
    const asked: string[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string) => {
        asked.push(path)
        return new Response("{}", {
          headers: { "Content-Type": "application/json" },
        })
      })
    )
    const load = readAhead(makeStore())
    const request = new Request("http://gusto.test/jobs?state=running")
    expect(await load({ request, params: {}, context: {} } as never)).toBe(null)
    await vi.waitFor(() =>
      expect(asked).toEqual(["/app/api/jobs?state=running"])
    )
  })
})
