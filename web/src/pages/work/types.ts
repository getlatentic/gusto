// The work pages' data, as /app/api gives it for Activity, a task, a project, how each agent type did, and comparing agents.
// The server builds these in Go (activity_view.go, task_view.go, project_view.go, compare_view.go); the two must stay the
// same shape.

import type { Option } from "@/domain/chat-choices"

// Origin is where a run came from. Link is empty when there is nothing to go to; Named is then a short word, and Why the whole
// reason. Project is the agent alone, where Named adds the conversation to it.
export interface Origin {
  Named: string
  Project: string
  Why: string
  Link: string
}

// Cost is what a run cost as a line says it: Title qualifies a figure that is not the provider's own count of money.
export interface Cost {
  Text: string
  Title: string
}

// Run is one line of Activity. Outcome is how it ended in the words of a log; Waiting, for a queued run, where it stands, and
// AskedBy who asked for it.
export interface Run {
  ID: string
  Brief: string
  State: string
  Outcome: string
  On: Origin
  Brain: string
  AskedBy: string
  Waiting: string
  When: string
  Hour: string
  Cost: Cost | null
}

export interface Figure {
  Label: string
  Count: number
  Href: string
  Current: boolean
}

// Index is what Activity adds when it reads the index of tasks: the search, the person (for an admin, Everyone), the order,
// how far the index has caught up, and the way back to the newest page.
export interface Index {
  Query: string
  Person: string
  Everyone: boolean
  Sort: string
  Sorts: { Name: string; Label: string }[]
  Building: { building: boolean; done: number; total: number }
  Newest: string
}

export interface Board {
  Figures: Figure[] | null
  State: string
  Harness: string
  Harnesses: string[] | null
  Days: { Named: string; Runs: Run[] }[] | null
  Shown: string
  Newer: string
  Older: string
  Index: Index | null
}

export interface ActivityData {
  Board: Board
}

// Cut says the page draws only the start of a record file, and where the whole of it is.
export interface Cut {
  Cut: boolean
  Name: string
  Raw: string
  Whole: string
  Shown: string
}

export interface DiffLine {
  Kind: "added" | "removed" | "context" | "note"
  Old: number
  New: number
  Text: string
}

export interface DiffFile {
  Name: string
  Meta: string[] | null
  Binary: boolean
  Deleted: boolean
  Hunks: { Header: string; Lines: DiffLine[] | null }[] | null
  Added: number
  Removed: number
}

export interface Proof {
  Holds: boolean
  Signed: boolean
  Files: number
  Wrong: string[] | null
  Signer: string
  // Unchecked is why a signature whose files match could not be tied to a signer.
  Unchecked: string
  Key: string
  Bundle: string
  Command: string
}

export interface Step {
  What: string
  Detail: string
}

// TaskData is a task's page. A turn of a conversation, for somebody who is not an admin, is read in its conversation, and its
// data is only MovedTo, where.
export interface TaskData {
  MovedTo?: string
  Job: {
    ID: string
    Brief: string
    State: string
    Harness: string
    Repo: string
    Model: string
    Started: number
    Ended: number
  }
  Outcome: string
  Took: string
  Waiting: string
  Brain: string
  Build?: string
  ThreadPath?: string
  On: Origin
  Admin: boolean
  // Live is a task still queued or running, whose transcript is its stream.
  Live?: boolean
  Cost?: Cost | null
  Agent?: { ID: string; Name: string; Brain: string }
  Talk?: {
    Name: string
    Place: number
    Of: number
    Before: string
    After: string
  }
  Steps: Step[] | null
  Calls: {
    InputTokens: string
    OutputTokens: string
    Denied: number
    Sites: { Host: string; Times: number }[] | null
  } | null
  Files: { Name: string; SHA256: string }[] | null
  Proof: Proof | null
  StoppedByCap?: { Cap: string; Reason: string }
  Home?: { Outcome: string; Used: string; Limit: string } | null
  Patch?: Cut & { Empty: boolean; Files: DiffFile[] | null }
  Transcript?: Cut & { Empty: boolean; HTML: string }
  Check?: {
    Verdict: { Passed: boolean; Code: number } | null
    Command: string
    Log: (Cut & { Text: string }) | null
    // Explains is a check that did not pass, whose output is the answer the reader came for.
    Explains: boolean
  }
}

export interface Routine {
  ID: string
  Title: string
  Prompt: string
  Thread: string
  When: string
  // Last is how its last slot went: "ran", "not yet", or why it was skipped.
  Last: string
  Paused: boolean
}

export interface ProjectApp {
  ID: string
  State: "live" | "starting" | "failed" | "stopped" | string
  Link: string
  Folder: string
  Command: string
  Note: string
  Port: number
}

export interface ProjectData {
  Instance: {
    ID: string
    Model: string
    Sensitivity: string
    Setup: string
    Repo: string
    Started: string
  }
  // Field is the kind of work the agent was set up for, drawn when its type has no mark.
  Agent: {
    Name: string
    Harness: string
    Field: string
    Engine: string
    Brain: string
    Mark: string
    MarkDark: string
  }
  Conversations: {
    Path: string
    Name: string
    Latest: string
    LatestQueued: number
  }[]
  Models: Option[]
  Sharing: boolean
  Runs: boolean
  Apps: ProjectApp[]
  // Refresh says an app is still starting, so the page reads itself again until it answers.
  Refresh: boolean
  Tasks: { ID: string; Brief: string; State: string; Queued: number }[]
  Routines: Routine[]
  // Unread is why the routines could not be read, when they could not.
  Unread: string
  Refused: string
}

export interface MeasuredCell {
  Tried: boolean
  Finished: boolean
  Took: string
  Cost: string
}

export interface EnginesData {
  Measured: {
    Models: string[] | null
    Rows:
      | {
          Name: string
          Title: string
          Mark: string
          MarkDark: string
          Cells: MeasuredCell[]
        }[]
      | null
    Finished: number
    Tried: number
  }
}

export interface EngineChoice {
  Name: string
  Title: string
  Mark: string
  MarkDark: string
}

export interface CompareData {
  Refused: string
  Lineups: {
    ID: string
    Brief: string
    Model: string
    Agents: number
    At: number
    When: string
  }[]
  // Template is the sample project a comparison runs on, when this server has one with a check.
  Template?: { Title: string; Starter: string; Check: string }
  Engines?: EngineChoice[]
  Models?: string[] | null
}

// Side is one agent's run in a comparison.
export interface Side {
  Engine: string
  Harness: string
  Mark: string
  MarkDark: string
  Model: string
  Task: string
  Holding: string
  Done: boolean
  Checked: boolean
  Passed: boolean
  Seconds: number
  Metered: boolean
  Money: string
  Calls: number
  Files: number
  Added: number
  Removed: number
  Fastest: boolean
  Cheapest: boolean
}

export interface LineupData {
  Lineup: { ID: string; Brief: string; Model: string }
  Running: boolean
  Passed: number
  Reading: { Headline: string } | null
  Sides: Side[]
}
