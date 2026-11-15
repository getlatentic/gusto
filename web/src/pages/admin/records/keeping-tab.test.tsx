import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import {
  draw,
  everyControlIsNamed,
  records,
} from "@/pages/admin/records/fixture"
import type { Keeping } from "@/pages/admin/records/types"

const keeping = (change: Partial<Keeping> = {}): Keeping => ({
  Used: "2.6 KB",
  WorkingFolders: "2.0 KB",
  Jobs: 1,
  Policy: [
    { State: "passed", WorkingFolder: 7, Record: 90 },
    { State: "failed", WorkingFolder: 0, Record: 0 },
  ],
  Removes: true,
  HeldTasks: null,
  HeldPeople: null,
  Oldest: "a-week-old",
  OldestAt: "5 Nov 2026",
  Upcoming: [
    {
      job: "a-week-old",
      brief: "brief a-week-old",
      kind: "workspace",
      state: "passed",
      days: 10,
      bytes: 2048,
    },
  ],
  UpcomingFreed: "2.0 KB",
  ...change,
})

const drawKeeping = (change: Partial<Keeping> = {}) =>
  draw(records("keeping", (page) => (page.Keeping = keeping(change))))

const formOf = (button: string) =>
  screen.getByRole("button", { name: button }).closest("form")!

describe("the keeping tab", () => {
  it("says what the disk holds and the policy that removes it", async () => {
    drawKeeping()
    for (const label of ["Stored", "Working folders", "Oldest task"]) {
      expect(await screen.findByText(label)).toBeTruthy()
    }
    expect(screen.getByTitle("by 1 task")).toBeTruthy()
    const oldest = screen.getByTitle("a-week-old")
    expect(oldest.getAttribute("href")).toBe("/jobs/a-week-old")
    const passed = screen.getByText("passed").closest("tr")!
    expect(passed.textContent).toBe("passed7 days90 days")
    expect(screen.getByText("failed").closest("tr")!.textContent).toBe(
      "failedForeverForever"
    )
    everyControlIsNamed()
  })

  it("says when the policy removes nothing, and nothing is due", async () => {
    drawKeeping({
      Removes: false,
      Upcoming: null,
      Oldest: undefined,
      OldestAt: undefined,
    })
    expect(
      (await screen.findByText(/The policy removes nothing\./)).textContent
    ).toBe(
      "The policy removes nothing. Set workspaceDays and recordDays by state in policy.json in GUSTO_RETENTION."
    )
    expect(screen.getByText("Nothing is due.")).toBeTruthy()
    expect(screen.getByText("Oldest task").nextSibling!.textContent).toBe("—")
  })

  it("lists what a sweep now would remove, makes one, and says what the last did", async () => {
    drawKeeping({
      Last: { at: 1, dry: false, actions: [], freed: 4096 },
      LastAt: "14 Nov 2026 10:00 UTC",
      LastFreed: "4.0 KB",
    })
    expect(
      await screen.findByText(
        "A sweep now would remove 1 item, freeing 2.0 KB."
      )
    ).toBeTruthy()
    const due = screen.getByRole("link", { name: "brief a-week-old" })
    expect(due.getAttribute("href")).toBe("/jobs/a-week-old")
    expect(due.closest("tr")!.textContent).toBe(
      "brief a-week-oldworkspace10 days ago2048 bytes"
    )
    const sweep = formOf("Sweep now")
    expect(sweep.method).toBe("post")
    expect(sweep.getAttribute("action")).toBe("/admin/retention/sweep")
    expect(
      screen.getByText(
        "The last sweep was 14 Nov 2026 10:00 UTC: 0 removed, 4.0 KB freed."
      )
    ).toBeTruthy()
  })

  it("holds a task or a person's work, and releases each hold", async () => {
    drawKeeping({
      HeldTasks: { held: { by: "ada@acme.com", at: 1, why: "the audit" } },
      HeldPeople: {
        "amy@acme.test": { by: "ada@acme.com", at: 1, why: "legal" },
      },
    })
    const task = (await screen.findByText("the audit")).closest("tr")!
    expect(task.textContent).toContain("task held")
    expect(task.querySelector("form")!.getAttribute("action")).toBe(
      "/admin/retention/jobs/release"
    )
    expect(
      (task.querySelector("input[name=job]") as HTMLInputElement).value
    ).toBe("held")
    const person = screen
      .getByText("everything asked for by amy@acme.test")
      .closest("tr")!
    expect(person.textContent).toContain("legal")
    expect(person.querySelector("form")!.getAttribute("action")).toBe(
      "/admin/retention/holds/release"
    )
    expect(
      (person.querySelector("input[name=who]") as HTMLInputElement).value
    ).toBe("amy@acme.test")

    const holdTask = formOf("Keep the task")
    expect(holdTask.getAttribute("action")).toBe("/admin/retention/jobs/hold")
    expect(holdTask.method).toBe("post")
    const job = screen.getByLabelText("Task") as HTMLInputElement
    expect([job.name, job.required, job.placeholder]).toEqual([
      "job",
      true,
      "the task's id, as in its address",
    ])
    const holdPerson = formOf("Keep their work")
    expect(holdPerson.getAttribute("action")).toBe("/admin/retention/holds")
    const who = screen.getByLabelText("Person") as HTMLInputElement
    expect([who.name, who.required, who.placeholder]).toEqual([
      "who",
      true,
      "amy@acme.test",
    ])
    for (const form of [holdTask, holdPerson]) {
      const why = form.querySelector("input[name=why]") as HTMLInputElement
      expect(why.placeholder).toBe("for the record")
    }
    expect(screen.getAllByLabelText("Why")).toHaveLength(2)
    everyControlIsNamed()
  })
})
