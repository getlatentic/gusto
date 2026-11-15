import { describe, expect, it } from "vitest"

import { isAppPage } from "@/components/app-link"
import { placeFor, showsConversations } from "@/components/person/places"

describe("the work pages' places", () => {
  it("put a project and how each agent type did among the members, and Activity and comparing in no place", () => {
    expect(placeFor("/instances/atlas-1")).toBe("members")
    expect(placeFor("/engines")).toBe("members")
    expect(placeFor("/instances/atlas-1/threads/1")).toBe("chat")
    expect(placeFor("/jobs")).toBeUndefined()
    expect(placeFor("/compare/l1")).toBeUndefined()
  })

  it("keep the list of conversations beside a conversation alone", () => {
    expect(
      ["/jobs", "/jobs/t1", "/instances/atlas-1", "/compare"].some(
        showsConversations
      )
    ).toBe(false)
  })

  it("are the app's pages, and the page that starts an agent and a task's files are the server's", () => {
    for (const page of [
      "/jobs",
      "/jobs?state=failed",
      "/jobs/t1",
      "/instances/atlas-1#routines",
      "/engines",
      "/compare",
      "/compare/l1",
    ])
      expect(isAppPage(page)).toBe(true)
    for (const page of [
      "/instances/new",
      "/instances/atlas-1/files",
      "/jobs/t1/files/transcript.log",
      "/jobs/t1/bundle",
    ])
      expect(isAppPage(page)).toBe(false)
  })
})
