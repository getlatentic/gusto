import type {
  ActivityData,
  Board,
  CompareData,
  LineupData,
  ProjectData,
  Run,
  Side,
  TaskData,
} from "@/pages/work/types"

export function runOf(change: Partial<Run> = {}): Run {
  return {
    ID: "t1",
    Brief: "count the files",
    State: "passed",
    Outcome: "check passed",
    On: {
      Named: "Atlas · Counting",
      Project: "Atlas",
      Why: "atlas-1",
      Link: "/instances/atlas-1",
    },
    Brain: "big-1",
    AskedBy: "",
    Waiting: "",
    When: "15 Nov 10:00",
    Hour: "10:00",
    Cost: { Text: "$0.0100", Title: "" },
    ...change,
  }
}

export function boardOf(change: Partial<Board> = {}): Board {
  return {
    Figures: [
      {
        Label: "running",
        Count: 0,
        Href: "/jobs?state=running",
        Current: false,
      },
      { Label: "queued", Count: 0, Href: "/jobs?state=queued", Current: false },
      { Label: "passed", Count: 2, Href: "/jobs?state=passed", Current: false },
      { Label: "failed", Count: 1, Href: "/jobs?state=failed", Current: false },
    ],
    State: "",
    Harness: "",
    Harnesses: ["codex"],
    Days: [{ Named: "Today", Runs: [runOf()] }],
    Shown: "3 tasks",
    Newer: "",
    Older: "",
    Index: null,
    ...change,
  }
}

export const activityOf = (change: Partial<Board> = {}): ActivityData => ({
  Board: boardOf(change),
})

export function taskOf(change: Partial<TaskData> = {}): TaskData {
  return {
    Job: {
      ID: "t1",
      Brief: "fix the adder",
      State: "passed",
      Harness: "codex",
      Repo: "",
      Model: "acme/big-1",
      Started: 100,
      Ended: 160,
    },
    Outcome: "check passed",
    Took: "1m0s",
    Waiting: "",
    Brain: "big-1",
    On: {
      Named: "Atlas",
      Project: "Atlas",
      Why: "atlas-1",
      Link: "/instances/atlas-1",
    },
    Admin: true,
    Agent: { ID: "atlas-1", Name: "Atlas", Brain: "big-1" },
    Steps: [{ What: "Read", Detail: "adder.sh" }],
    Calls: null,
    Files: [
      { Name: "transcript.log", SHA256: "abc123" },
      { Name: "manifest.json", SHA256: "" },
    ],
    Proof: {
      Holds: true,
      Signed: true,
      Files: 1,
      Wrong: null,
      Signer: "acme",
      Unchecked: "",
      Key: "/keys/acme.pub",
      Bundle: "/jobs/t1/bundle",
      Command: "tar -xzf t1.tar.gz && GUSTO_KEYRING=. gusto verify t1 -signed",
    },
    Home: null,
    Transcript: {
      Empty: false,
      HTML: "<p>I fixed it.</p>",
      Cut: false,
      Name: "transcript.log",
      Raw: "/jobs/t1/files/transcript.log",
      Whole: "14 B",
      Shown: "14 B",
    },
    Check: {
      Verdict: { Passed: true, Code: 0 },
      Command: "sh test.sh",
      Log: null,
      Explains: false,
    },
    ...change,
  }
}

export function projectOf(change: Partial<ProjectData> = {}): ProjectData {
  return {
    Instance: {
      ID: "atlas-1",
      Model: "acme/big-1",
      Sensitivity: "internal",
      Setup: "",
      Repo: "",
      Started: "15 Nov 09:00",
    },
    Agent: {
      Name: "Atlas",
      Harness: "codex",
      Field: "",
      Engine: "Codex",
      Brain: "big-1",
      Mark: "/static/agents/codex.svg",
      MarkDark: "",
    },
    Conversations: [
      {
        Path: "/instances/atlas-1/threads/2",
        Name: "The budget",
        Latest: "what did we spend?",
        LatestQueued: 0,
      },
      {
        Path: "/instances/atlas-1/threads/1",
        Name: "New thread",
        Latest: "",
        LatestQueued: 0,
      },
    ],
    Models: [
      {
        value: "acme/big-1",
        label: "big-1 · $1.00 in, $2.00 out per million tokens",
      },
      { value: "acme/small-1", label: "small-1" },
    ],
    Sharing: false,
    Runs: false,
    Apps: [],
    Refresh: false,
    Tasks: [],
    Routines: [],
    Unread: "",
    Refused: "",
    ...change,
  }
}

export function compareOf(change: Partial<CompareData> = {}): CompareData {
  return {
    Refused: "",
    Lineups: [],
    Template: {
      Title: "Engineering",
      Starter: "Run the tests and fix what fails",
      Check: "node --test",
    },
    Engines: [
      {
        Name: "codex",
        Title: "Codex",
        Mark: "/static/agents/codex.svg",
        MarkDark: "",
      },
      { Name: "pi", Title: "Pi", Mark: "", MarkDark: "" },
    ],
    Models: ["big-1", "small-1"],
    ...change,
  }
}

export function sideOf(change: Partial<Side> = {}): Side {
  return {
    Engine: "Codex",
    Harness: "codex",
    Mark: "",
    MarkDark: "",
    Model: "big-1",
    Task: "run-1",
    Holding: "",
    Done: true,
    Checked: true,
    Passed: true,
    Seconds: 40,
    Metered: true,
    Money: "$0.7500",
    Calls: 3,
    Files: 1,
    Added: 2,
    Removed: 1,
    Fastest: true,
    Cheapest: false,
    ...change,
  }
}

export function lineupOf(change: Partial<LineupData> = {}): LineupData {
  return {
    Lineup: { ID: "two-runs", Brief: "fix it", Model: "big-1" },
    Running: false,
    Passed: 1,
    Reading: { Headline: "Codex was the only one to pass the check." },
    Sides: [
      sideOf(),
      sideOf({
        Engine: "Pi",
        Harness: "pi",
        Task: "run-2",
        Passed: false,
        Fastest: false,
        Files: 0,
      }),
    ],
    ...change,
  }
}
