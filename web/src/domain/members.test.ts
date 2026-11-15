import { describe, expect, it } from "vitest"

import {
  agentTabOf,
  agentTabs,
  appChoices,
  emailWords,
  kindNotes,
  memberTabOf,
  shownIn,
  skillGroups,
} from "@/domain/members"

const kind = {
  Ready: true,
  What: "Writes code",
  Measured: "12s on big-1",
  Failed: false,
  Installable: false,
  Why: "",
}

describe("the members list's tabs", () => {
  it("opens on the tab kept last, and on All when none or another word was kept", () => {
    expect(memberTabOf("agents")).toBe("agents")
    expect(memberTabOf("people")).toBe("people")
    expect(memberTabOf(null)).toBe("all")
    expect(memberTabOf("robots")).toBe("all")
  })

  it("lists everybody on All, and only people or only agents on theirs", () => {
    expect(shownIn("all")).toEqual({ people: true, agents: true })
    expect(shownIn("people")).toEqual({ people: true, agents: false })
    expect(shownIn("agents")).toEqual({ people: false, agents: true })
  })
})

describe("an agent's tabs", () => {
  it("offer Apps only where the server offers apps", () => {
    expect(agentTabs(false).map((tab) => tab.label)).toEqual([
      "Profile",
      "Access",
      "Skills",
    ])
    expect(agentTabs(true).map((tab) => tab.key)).toContain("apps")
  })

  it("open the tab asked for when there is one, and the profile otherwise", () => {
    expect(agentTabOf("access", false)).toBe("access")
    expect(agentTabOf("skills", true)).toBe("skills")
    expect(agentTabOf("apps", true)).toBe("apps")
    expect(agentTabOf("apps", false)).toBe("profile")
    expect(agentTabOf(null, true)).toBe("profile")
    expect(agentTabOf("elsewhere", true)).toBe("profile")
  })
})

describe("what the new agent dialog says of an agent type", () => {
  it("says what one that can run is, and how it did here, warning when it finished on no model offered", () => {
    expect(kindNotes(kind)).toEqual([
      { text: "Writes code", warn: false },
      { text: "12s on big-1", warn: false },
    ])
    expect(
      kindNotes({ ...kind, What: "", Measured: "Did not finish", Failed: true })
    ).toEqual([{ text: "Did not finish", warn: true }])
  })

  it("says one gusto can install is not on this computer yet", () => {
    expect(kindNotes({ ...kind, Ready: false, Installable: true })).toEqual([
      { text: "Not on this computer yet", warn: false },
    ])
  })

  it("says why one that cannot run is not on this server", () => {
    expect(
      kindNotes({ ...kind, Ready: false, Why: "npm install -g claude" })
    ).toEqual([
      {
        text: "Not on this server: npm install -g claude",
        warn: false,
        title: "npm install -g claude",
      },
    ])
  })
})

describe("words of the members pages", () => {
  it("gives a person's email, and says the shared sign-in signs in with the server's password", () => {
    expect(emailWords({ Who: "ada@acme.com", SharedSignIn: false })).toBe(
      "ada@acme.com"
    )
    expect(emailWords({ Who: "the shared sign-in", SharedSignIn: true })).toBe(
      "Signs in with this server's password"
    )
  })

  it("names what inherit follows in an app's choices", () => {
    expect(appChoices(false).map((choice) => choice.label)).toEqual([
      "Inherit (off)",
      "Allow",
      "Don’t allow",
    ])
    expect(appChoices(true)[0].label).toBe("Inherit (on)")
  })

  it("splits the skills an agent can be given into ready-made roles and skill sets", () => {
    const groups = skillGroups([
      { Name: "health", Role: true },
      { Name: "launch", Role: false },
    ])
    expect(groups.roles.map((each) => each.Name)).toEqual(["health"])
    expect(groups.sets.map((each) => each.Name)).toEqual(["launch"])
  })
})
