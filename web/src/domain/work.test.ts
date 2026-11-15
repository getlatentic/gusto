import { describe, expect, it } from "vitest"

import {
  appIsRunning,
  everythingHref,
  heard,
  isLive,
  offersAgentTypes,
  routineLast,
  searchOf,
  showsFilters,
  showsOutcome,
  sideResult,
  sortChosen,
  toneOf,
  whyEmpty,
} from "@/domain/work"

const board = (change: Partial<Parameters<typeof showsFilters>[0]> = {}) => ({
  State: "",
  Harness: "",
  Harnesses: null,
  Days: null,
  Index: null,
  ...change,
})

const index = (Query = "", Person = "", building = false) => ({
  Query,
  Person,
  Building: { building },
})

describe("the tone of a state", () => {
  it("is good for what ended well, bad for what did not, and a warning for what is still going", () => {
    expect(["passed", "finished", "signed", "live"].map(toneOf)).toEqual([
      "ok",
      "ok",
      "ok",
      "ok",
    ])
    expect(["failed", "interrupted"].map(toneOf)).toEqual(["bad", "bad"])
    expect(["running", "queued"].map(toneOf)).toEqual(["warn", "warn"])
    expect(toneOf("stopped")).toBe("plain")
  })

  it("says a queued or running task is live", () => {
    expect(isLive("queued") && isLive("running")).toBe(true)
    expect(isLive("passed") || isLive("interrupted")).toBe(false)
  })
})

describe("a run's line", () => {
  it("says how it ended unless it ended as expected", () => {
    expect(showsOutcome("replied")).toBe(false)
    expect(showsOutcome("check passed")).toBe(false)
    expect(
      ["waiting", "working", "failed", "check failed"].map(showsOutcome)
    ).toEqual([true, true, true, true])
  })
})

describe("Activity's filters", () => {
  it("keeps the agent type when every state is shown", () => {
    expect(everythingHref("")).toBe("/jobs")
    expect(everythingHref("claude code")).toBe("/jobs?agent_type=claude%20code")
  })

  it("are offered when something is listed or something narrows the list", () => {
    expect(showsFilters(board())).toBe(false)
    expect(showsFilters(board({ Days: [{}] }))).toBe(true)
    expect(showsFilters(board({ State: "failed" }))).toBe(true)
    expect(showsFilters(board({ Index: index("login") }))).toBe(true)
    expect(showsFilters(board({ Index: index("", "bob@acme.com") }))).toBe(true)
    expect(showsFilters(board({ Index: index() }))).toBe(false)
  })

  it("offer the agent types only when there are two to choose between, or one to lift", () => {
    expect(offersAgentTypes(board({ Harnesses: ["codex"] }))).toBe(false)
    expect(offersAgentTypes(board({ Harnesses: ["codex", "pi"] }))).toBe(true)
    expect(offersAgentTypes(board({ Harness: "pi" }))).toBe(true)
  })

  it("say why nothing is listed", () => {
    expect(whyEmpty(board({ Harness: "pi" }))).toBe("filtered")
    expect(whyEmpty(board({ Index: index("", "", true) }))).toBe("building")
    expect(whyEmpty(board({ Index: index("x", "", true) }))).toBe("filtered")
    expect(whyEmpty(board())).toBe("nothing")
  })

  it("order by newest asked when the address names no order", () => {
    expect(sortChosen("")).toBe("queued")
    expect(sortChosen("seq")).toBe("seq")
  })

  it("send a search as a browser sends a form by GET", () => {
    expect(
      searchOf("/jobs", [
        ["state", "failed"],
        ["q", "100% done"],
        ["agent_type", ""],
      ])
    ).toBe("/jobs?state=failed&q=100%25+done&agent_type=")
  })
})

describe("a comparison's run", () => {
  const side = (change = {}) => ({
    Holding: "",
    Done: true,
    Checked: true,
    Passed: true,
    ...change,
  })

  it("says what it waits for, that it works, or how its tests went", () => {
    expect(sideResult(side({ Holding: "Number 1 in line." }))).toBe("holding")
    expect(sideResult(side({ Done: false }))).toBe("working")
    expect(sideResult(side())).toBe("passed")
    expect(sideResult(side({ Passed: false }))).toBe("failed")
    expect(sideResult(side({ Checked: false, Passed: false }))).toBe(
      "stopped before the tests"
    )
  })
})

describe("a project's apps and routines", () => {
  it("stops an app that runs and dismisses one that does not", () => {
    expect(appIsRunning("live") && appIsRunning("starting")).toBe(true)
    expect(appIsRunning("failed") || appIsRunning("stopped")).toBe(false)
  })

  it("says how a routine's last slot went", () => {
    expect(routineLast("ran")).toEqual({ words: "Last run ok", tone: "ok" })
    expect(routineLast("not yet")).toEqual({
      words: "Not run yet",
      tone: "plain",
    })
    expect(routineLast("skipped: still answering")).toEqual({
      words: "Last run skipped: still answering",
      tone: "warn",
    })
  })
})

describe("a running task's words", () => {
  it("are each line it says, in order", () => {
    expect(heard(heard("", "first"), "second")).toBe("first\nsecond\n")
  })

  it("keep only the last of a harness that goes on", () => {
    expect(heard("abcdef\n", "gh", 6)).toBe("ef\ngh\n")
  })
})
