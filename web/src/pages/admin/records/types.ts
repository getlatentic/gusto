// The Records page's data as the server gives it: the record's health on every tab, and the chosen tab's own part.

export interface RecordsTab {
  Key: string
  Label: string
  Href: string
  Count: number
  On: boolean
}

// Health is the record in one line: whether the activity log's chain holds and is signed, and whether the newest finished runs
// still match what was recorded. LogBad and RunsBad say which half is the trouble.
export interface Health {
  Logged: boolean
  Broken: boolean
  Log: string
  Runs: string
  LogBad: boolean
  RunsBad: boolean
  Head: string
}

export interface ActivityRow {
  At: string
  Full: string
  Who: string
  Said: string
  From: string
  Times: number
  Since: string
  Kind: string
}

export interface Activity {
  Rows: ActivityRow[] | null
  LeftOut: number
  WithServer: boolean
}

export interface CheckedRun {
  ID: string
  Brief: string
  Who: string
  When: string
  State: string
  Href: string
}

export interface RunChecks {
  Rows: CheckedRun[] | null
  Verified: number
  Unsigned: number
  Unchecked: number
  Failing: number
}

export interface PolicyRow {
  State: string
  WorkingFolder: number
  Record: number
}

export interface Hold {
  by: string
  at: number
  why?: string
}

// SweepAction is one thing a sweep removes: a task's working folder, or its whole record.
export interface SweepAction {
  job: string
  brief: string
  kind: string
  state: string
  days: number
  bytes: number
  failed?: string
}

export interface SweepReport {
  at: number
  dry: boolean
  actions: SweepAction[] | null
  freed: number
}

export interface Keeping {
  Used: string
  WorkingFolders: string
  Jobs: number
  Policy: PolicyRow[] | null
  Removes: boolean
  HeldTasks: Record<string, Hold> | null
  HeldPeople: Record<string, Hold> | null
  Oldest?: string
  OldestAt?: string
  Last?: SweepReport
  LastAt?: string
  LastFreed?: string
  Upcoming: SweepAction[] | null
  UpcomingFreed: string
}

export interface RecordsData {
  Tabs: RecordsTab[]
  Tab: string
  Refused: string
  Health: Health
  Logged: boolean
  Activity?: Activity
  Checks?: RunChecks
  Keeping?: Keeping
}
