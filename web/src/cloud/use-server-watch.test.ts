import { afterEach, describe, expect, it, vi } from "vitest"

import { watchServer } from "@/cloud/use-server-watch"

function answer(state: string, redirected = false): Response {
  const response = new Response(JSON.stringify({ State: state }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
  Object.defineProperty(response, "redirected", { value: redirected })
  return response
}

afterEach(() => vi.unstubAllGlobals())

describe("watchServer", () => {
  it("asks again while the state is the one drawn, and draws the page again once it changes", async () => {
    const fetched = vi
      .fn()
      .mockResolvedValueOnce(answer("waiting"))
      .mockResolvedValueOnce(answer("starting"))
    vi.stubGlobal("fetch", fetched)
    const changed = vi.fn()
    await watchServer(
      "waiting",
      "acme-com",
      new AbortController().signal,
      changed
    )
    expect(changed).toHaveBeenCalledOnce()
    expect(fetched).toHaveBeenCalledTimes(2)
    expect(fetched.mock.calls[0][0]).toBe(
      "/org/server?seen=waiting&org=acme-com"
    )
  })

  it("asks again after a pause when gusto cloud could not be asked", async () => {
    const fetched = vi
      .fn()
      .mockRejectedValueOnce(new TypeError("offline"))
      .mockResolvedValueOnce(new Response("", { status: 502 }))
      .mockResolvedValueOnce(answer("ready"))
    vi.stubGlobal("fetch", fetched)
    const changed = vi.fn()
    await watchServer("", undefined, new AbortController().signal, changed, 1)
    expect(changed).toHaveBeenCalledOnce()
    expect(fetched).toHaveBeenCalledTimes(3)
    expect(fetched.mock.calls[0][0]).toBe("/org/server?seen=")
  })

  it("draws the page again when the answer is another page, as after the session ended", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(answer("waiting", true)))
    const changed = vi.fn()
    await watchServer(
      "waiting",
      "acme-com",
      new AbortController().signal,
      changed
    )
    expect(changed).toHaveBeenCalledOnce()
  })

  it("stops when the page goes", async () => {
    const stop = new AbortController()
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(() => {
        stop.abort()
        return Promise.reject(new DOMException("aborted", "AbortError"))
      })
    )
    const changed = vi.fn()
    await watchServer("waiting", "acme-com", stop.signal, changed, 1)
    expect(changed).not.toHaveBeenCalled()
  })
})
