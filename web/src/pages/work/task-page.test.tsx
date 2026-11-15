import { act, fireEvent, screen, within } from "@testing-library/react"
import { Route, Routes } from "react-router"
import { describe, expect, it, vi } from "vitest"

import { taskOf } from "@/pages/work/fixture"
import { TaskPage } from "@/pages/work/task-page"
import type { TaskData } from "@/pages/work/types"
import { renderPage } from "@/test/render"

function draw(page: TaskData, extra: Record<string, unknown> = {}) {
  return renderPage("/jobs/t1", "/jobs/:id", <TaskPage />, {
    "/jobs/t1": page,
    ...extra,
  })
}

// Stream is the server's stream of a running task's words, as the page's EventSource.
class Stream {
  static opened: Stream[] = []
  url: string
  closed = false
  onmessage: ((event: MessageEvent<string>) => void) | null = null
  private ended: ((event: MessageEvent<string>) => void)[] = []
  constructor(url: string) {
    this.url = url
    Stream.opened.push(this)
  }
  addEventListener(
    name: string,
    listener: (event: MessageEvent<string>) => void
  ) {
    if (name === "ended") this.ended.push(listener)
  }
  close() {
    this.closed = true
  }
  say(line: string) {
    this.onmessage?.(new MessageEvent("message", { data: line }))
  }
  end(state: string) {
    for (const listener of this.ended)
      listener(new MessageEvent("ended", { data: state }))
  }
}

const running = (change: Partial<TaskData> = {}) =>
  taskOf({
    Job: { ...taskOf().Job, State: "running", Ended: 0 },
    Outcome: "working",
    Took: "",
    Live: true,
    Transcript: undefined,
    Check: undefined,
    Proof: null,
    ...change,
  })

describe("a task's page", () => {
  it("says what was asked, how it ended, what it took and ran on, and where it is", async () => {
    draw(taskOf({ Cost: { Text: "$0.0200", Title: "" }, Build: "codex@1.2.3" }))
    expect((await screen.findByTestId("task-brief")).textContent).toBe(
      "fix the adder"
    )
    expect(screen.getByLabelText("Summary").textContent).toBe(
      "check passed1m0s · $0.0200"
    )
    const details = screen.getByLabelText("Details")
    expect(
      within(details).getByRole("link", { name: "Atlas" }).getAttribute("href")
    ).toBe("/instances/atlas-1")
    expect(details.textContent).toContain("Buildcodex@1.2.3")
    expect(details.textContent).toContain("Modelbig-1")
    expect(details.textContent).toContain("Checkpassed")
    expect(details.textContent).not.toContain("Came from")
    const crumbs = screen.getByRole("navigation", { name: "Where this is" })
    expect(
      within(crumbs)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href"))
    ).toEqual(["/files", "/instances/atlas-1"])
    expect(screen.getByTestId("page-title").textContent).toBe("Run details")
  })

  it("says where a task that ran on no agent came from", async () => {
    draw(
      taskOf({
        Agent: undefined,
        On: {
          Named: "API",
          Project: "",
          Why: "none: sent to POST /api/jobs, from the command line",
          Link: "",
        },
      })
    )
    const came = (await screen.findByText("Came from")).closest("div")!
    expect(within(came).getByText("API").getAttribute("title")).toBe(
      "none: sent to POST /api/jobs, from the command line"
    )
    expect(came.parentElement!.textContent).toContain("Agentcodex")
    expect(
      within(screen.getByRole("navigation", { name: "Where this is" }))
        .getByRole("link")
        .getAttribute("href")
    ).toBe("/jobs")
  })

  it("places a turn in its conversation, with the messages either side and the way back", async () => {
    draw(
      taskOf({
        ThreadPath: "/instances/atlas-1/threads/1",
        Talk: {
          Name: "The budget",
          Place: 2,
          Of: 3,
          Before: "t0",
          After: "t2",
        },
        Check: undefined,
      })
    )
    expect(await screen.findByText("Message 2 of 3")).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "Previous" }).getAttribute("href")
    ).toBe("/jobs/t0")
    expect(
      screen.getByRole("link", { name: "Next" }).getAttribute("href")
    ).toBe("/jobs/t2")
    expect(
      screen
        .getByRole("link", { name: "Open the conversation" })
        .getAttribute("href")
    ).toBe("/instances/atlas-1/threads/1")
    expect(screen.queryByTestId("task-transcript")).toBeNull()
    expect(screen.queryByTestId("task-check")).toBeNull()
  })

  it("draws the transcript the server made safe, and says when a file is cut", async () => {
    draw(
      taskOf({
        Transcript: {
          ...taskOf().Transcript!,
          HTML: "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>",
          Cut: true,
          Whole: "512.0 KiB",
          Shown: "256.0 KiB",
        },
      })
    )
    const said = await screen.findByTestId("task-transcript")
    expect(said.querySelector("script")).toBeNull()
    expect(said.textContent).toContain("<script>alert(1)</script>")
    expect(said.textContent).toContain(
      "transcript.log is 512.0 KiB, and the page draws the first 256.0 KiB. Read the whole file."
    )
    expect(
      within(said)
        .getByRole("link", { name: "Read the whole file" })
        .getAttribute("href")
    ).toBe("/jobs/t1/files/transcript.log")
  })

  it("draws the patch file by file, as text, line by line", async () => {
    draw(
      taskOf({
        Patch: {
          Empty: false,
          Cut: false,
          Name: "diff.patch",
          Raw: "",
          Whole: "",
          Shown: "",
          Files: [
            {
              Name: "<b>x</b>.html",
              Meta: ["new file mode 100644"],
              Binary: false,
              Deleted: false,
              Added: 1,
              Removed: 1,
              Hunks: [
                {
                  Header: "@@ -1 +1 @@",
                  Lines: [
                    { Kind: "removed", Old: 1, New: 0, Text: "<p>old</p>" },
                    {
                      Kind: "added",
                      Old: 0,
                      New: 1,
                      Text: "<script>alert('diff')</script>",
                    },
                  ],
                },
              ],
            },
            {
              Name: "logo.png",
              Meta: null,
              Binary: true,
              Deleted: false,
              Added: 0,
              Removed: 0,
              Hunks: null,
            },
          ],
        },
      })
    )
    const patch = await screen.findByTestId("task-patch")
    expect(within(patch).getByText("2 files.")).toBeTruthy()
    const [html, logo] = within(patch).getAllByTestId("patch-file")
    fireEvent.click(within(html).getByRole("button"))
    expect(html.querySelector("b, p, script")).toBeNull()
    const rows = [...html.querySelectorAll("tr")].map((row) =>
      [...row.cells].map((cell) => cell.textContent)
    )
    expect(rows).toEqual([
      ["", "", "@@ -1 +1 @@"],
      ["1", "", "<p>old</p>"],
      ["", "1", "<script>alert('diff')</script>"],
    ])
    expect(html.querySelector('[data-kind="removed"]')!.className).toContain(
      "bg-bad-wash"
    )
    expect(html.textContent).toContain("+1−1")
    fireEvent.click(within(logo).getByRole("button"))
    expect(logo.textContent).toContain(
      "A binary file. The patch names it and does not carry its contents."
    )
  })

  it("says nothing was recorded when the run left no patch, and nothing of one that changed nothing", async () => {
    draw(taskOf())
    expect((await screen.findByTestId("task-no-patch")).textContent).toBe(
      "Nothing was recorded."
    )
  })

  it("lists what the run did, and each site it was refused", async () => {
    draw(
      taskOf({
        Calls: {
          InputTokens: "1.2k",
          OutputTokens: "300",
          Denied: 3,
          Sites: [{ Host: "example.com", Times: 3 }],
        },
      })
    )
    const steps = await screen.findByTestId("task-steps")
    expect(within(steps).getByRole("button", { name: "1 step" })).toBeTruthy()
    expect(within(steps).getByRole("listitem").textContent).toBe("Readadder.sh")
    const network = screen.getByTestId("task-network")
    expect(network.textContent).toContain(
      "The agent asked for 3 addresses outside the model, and each was refused."
    )
    expect(
      within(network).getByRole("cell", { name: "example.com" })
    ).toBeTruthy()
    expect(screen.getByText("1.2k in · 300 out")).toBeTruthy()
  })

  it("shows what a check that did not pass said, and folds away what a pass printed", async () => {
    draw(
      taskOf({
        Job: { ...taskOf().Job, State: "failed" },
        Outcome: "check failed",
        Check: {
          Verdict: { Passed: false, Code: 3 },
          Command: "sh -c 'go test ./... && exit 3'",
          Log: {
            Text: "<img src=x onerror=alert('gate')>\n",
            Cut: false,
            Name: "gate.log",
            Raw: "",
            Whole: "38 B",
            Shown: "",
          },
          Explains: true,
        },
      })
    )
    const check = await screen.findByTestId("task-check")
    expect(within(check).getByText("failed")).toBeTruthy()
    expect(
      within(check).getByText("sh -c 'go test ./... && exit 3'")
    ).toBeTruthy()
    expect(within(check).getByText("exited 3")).toBeTruthy()
    expect(
      within(check).getByRole("heading", { name: "What the check said" })
    ).toBeTruthy()
    expect(check.querySelector("img")).toBeNull()
    expect(check.querySelector("pre")!.textContent).toBe(
      "<img src=x onerror=alert('gate')>\n"
    )
  })

  it("folds what a passing check printed under how much it was", async () => {
    draw(
      taskOf({
        Check: {
          Verdict: { Passed: true, Code: 0 },
          Command: "true",
          Log: {
            Text: "ok\n",
            Cut: false,
            Name: "gate.log",
            Raw: "",
            Whole: "3 B",
            Shown: "",
          },
          Explains: false,
        },
      })
    )
    const check = await screen.findByTestId("task-check")
    expect(
      within(check).queryByRole("heading", { name: "What the check said" })
    ).toBeNull()
    expect(
      within(check).getByRole("button", {
        name: "What the check printed (3 B)",
      })
    ).toBeTruthy()
  })

  it("says the run stopped before its check answered", async () => {
    draw(
      taskOf({
        Check: { Verdict: null, Command: "", Log: null, Explains: true },
      })
    )
    expect(
      await screen.findByText("The run stopped before its check answered.")
    ).toBeTruthy()
    expect(screen.getByText("no answer")).toBeTruthy()
  })

  it("gives an admin the record, the key and the command that verify the run, and each file's fingerprint", async () => {
    const copied = vi.fn(async () => {})
    vi.stubGlobal("navigator", { clipboard: { writeText: copied } })
    draw(taskOf())
    const proof = await screen.findByTestId("task-proof")
    fireEvent.click(
      within(proof).getByRole("button", { name: "Verify this run" })
    )
    expect(proof.textContent).toContain(
      "Verified: signed by acme, and all 1 files match."
    )
    expect(proof.textContent).toContain(
      "Check it on your own machine, without gusto."
    )
    expect(
      within(proof)
        .getByRole("link", { name: "The record" })
        .getAttribute("href")
    ).toBe("/jobs/t1/bundle")
    expect(
      within(proof)
        .getByRole("link", { name: "The signing key" })
        .getAttribute("href")
    ).toBe("/keys/acme.pub")
    expect(proof.querySelector("#verify")!.textContent).toBe(
      "tar -xzf t1.tar.gz && GUSTO_KEYRING=. gusto verify t1 -signed"
    )
    fireEvent.click(within(proof).getByRole("button", { name: "Copy" }))
    expect(
      await within(proof).findByRole("button", { name: "Copied" })
    ).toBeTruthy()
    expect(copied).toHaveBeenCalledWith(
      "tar -xzf t1.tar.gz && GUSTO_KEYRING=. gusto verify t1 -signed"
    )
    fireEvent.click(
      within(proof).getByRole("button", {
        name: "Each file and its fingerprint",
      })
    )
    const files = within(proof).getAllByRole("link", { name: "transcript.log" })
    expect(files.map((file) => file.getAttribute("href"))).toEqual([
      "/jobs/t1/files/transcript.log",
    ])
    expect(within(proof).getByText("abc123")).toBeTruthy()
    expect(within(proof).queryByText("manifest.json")).toBeNull()
  })

  it("says how a record was or was not signed", async () => {
    const proof = taskOf().Proof!
    for (const [change, words] of [
      [
        { Signed: false, Signer: "", Key: "" },
        "All 1 files match. Not signed: this server had no signing key when it ran.",
      ],
      [
        {
          Signed: false,
          Signer: "",
          Unchecked: "its key, stranger, is not in the keyring",
        },
        "All 1 files match; its key, stranger, is not in the keyring.",
      ],
    ] as const) {
      const { unmount } = draw(taskOf({ Proof: { ...proof, ...change } }))
      const shown = await screen.findByTestId("task-proof")
      fireEvent.click(
        within(shown).getByRole("button", { name: "Verify this run" })
      )
      expect(shown.textContent).toContain(words)
      unmount()
    }
  })

  it("shows anybody a record that does not check out, open, and nobody else's proof that does", async () => {
    draw(
      taskOf({
        Admin: false,
        Proof: {
          ...taskOf().Proof!,
          Holds: false,
          Wrong: ["answer.txt does not match"],
        },
      })
    )
    const proof = await screen.findByTestId("task-proof")
    expect(proof.textContent).toContain("Verify this run(does not check out)")
    expect(proof.textContent).toContain("This record does not check out.")
    expect(within(proof).getByText("answer.txt does not match")).toBeTruthy()
    expect(
      screen
        .getByRole("link", { name: "does not check out" })
        .getAttribute("href")
    ).toBe("#proof")
  })

  it("leaves a record that checks out to an admin", async () => {
    draw(taskOf({ Admin: false }))
    await screen.findByTestId("task-brief")
    expect(screen.queryByTestId("task-proof")).toBeNull()
  })

  it("says whose home was full and that the person who asked can reset it, and what cap stopped a run", async () => {
    draw(
      taskOf({
        Home: { Outcome: "refused", Used: "2.0 GiB", Limit: "2.0 GiB" },
        StoppedByCap: { Cap: "task", Reason: "the task spent its $1.00" },
      })
    )
    expect((await screen.findByRole("alert")).textContent).toBe(
      "The home their agents keep is full: it takes 2.0 GiB of the 2.0 GiB it may, so this turn was refused. The person who asked can reset it on Connections."
    )
    expect(screen.queryByRole("link", { name: "Connections" })).toBeNull()
    expect(screen.getByText("stopped by the task cap")).toBeTruthy()
    expect(screen.getByText("the task spent its $1.00")).toBeTruthy()
  })

  it("follows a running task's words as it says them, and reads the record again once it ends", async () => {
    Stream.opened = []
    vi.stubGlobal("EventSource", Stream)
    const { asked } = draw(running({ Waiting: "Number 1 in line." }))
    const stream = await screen.findByTestId("task-stream")
    expect(stream.textContent).toBe("waiting…")
    expect(screen.getByText("Number 1 in line.").id).toBe("why")
    const watching = Stream.opened.find((each) => !each.closed)!
    expect(watching.url).toBe("/watch/t1")
    act(() => {
      watching.say("first line")
      watching.say("second line")
    })
    expect(stream.textContent).toBe("first line\nsecond line\n")
    const read = asked.length
    act(() => watching.end("passed"))
    expect(screen.getByTestId("task-state").textContent).toBe("passed")
    expect(watching.closed).toBe(true)
    await vi.waitFor(() => expect(asked.length).toBeGreaterThan(read))
  })

  it("offers to stop a running task, which comes back to the task", async () => {
    vi.stubGlobal("EventSource", Stream)
    draw(running())
    const stop = await screen.findByTestId("stop-task")
    expect(stop.getAttribute("action")).toBe("/jobs/t1/stop")
    expect(
      (stop.querySelector('[name="back"]') as HTMLInputElement).value
    ).toBe("/jobs/t1")
    expect(within(stop).getByRole("button", { name: "Stop" })).toBeTruthy()
  })

  it("offers no Stop on a task that is over", async () => {
    draw(taskOf())
    await screen.findByTestId("task-brief")
    expect(screen.queryByTestId("stop-task")).toBeNull()
  })

  it("goes to the conversation for a turn read there", async () => {
    renderPage(
      "/jobs/t1",
      "/*",
      <Routes>
        <Route path="/jobs/:id" element={<TaskPage />} />
        <Route
          path="/instances/:id/threads/:thread"
          element={<p>the conversation</p>}
        />
      </Routes>,
      { "/jobs/t1": { MovedTo: "/instances/atlas-1/threads/1#t1" } }
    )
    expect(await screen.findByText("the conversation")).toBeTruthy()
  })
})
