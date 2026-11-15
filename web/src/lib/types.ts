// The views the server answers at /app/api, as internal/web builds them. Field names are the Go struct fields'.

export interface ConsoleShell {
  sections: { key: string; href: string; label: string }[]
  orgName?: string
  orgPage?: string
}

export interface UsageTabs {
  Summary: string
  Tasks: string
  Current: "summary" | "tasks"
}

// Overview: this month's spend, against the budget.
export interface SpendLine {
  Label: string
  Sub: string
  Href: string
  USD: number
  Calls: number
  Unpriced: boolean
  Budget: number
  Percent: number
  More: number
}

export interface OverviewData {
  MonthName: string
  Budget: {
    Set: boolean
    Limit: number
    Passed: boolean
    Over: number
    Err: string
  }
  BudgetsPage: boolean
  PeopleAll: string
  Percent?: number
  Credit?: { Used: number; Limit: number }
  Spent: {
    USD: number
    Unmeasured: number
    Split: boolean
    Building: boolean
    People: SpendLine[]
    Engines: SpendLine[]
    AllUsage: string
  }
}

// Spending: the month's ledger, day by day.
export interface Tally {
  name?: string
  calls: number
  usd: number
  unmeasured: number
  uncounted: number
  inputTokens: number
  cachedTokens: number
  outputTokens: number
  refused: number
}

export interface SpendingData {
  Base: string
  Tabs: UsageTabs
  Ledger: {
    month: string
    total: Tally
    Months: string[] | null
    Measure: "money" | "tokens"
    Daily:
      { Date: string; Label: string; Height: number; Says: string }[] | null
    DayLabels: string[] | null
    Tops: { Title: string; Name: string; Says: string }[] | null
  }
}

// Usage: the conversations, filtered and sorted.
export interface UsageRow {
  ID: string
  Title: string
  Href: string
  Record: string
  Last: string
  LastFull: string
  Agent: string
  Engine: string
  Harness: string
  Mark: string
  MarkDark: string
  Asked: string
  Owner: string
  Models: string
  Cost: string
  Unpriced: string
  Turns: number
}

export interface UsageView {
  Values: Record<string, string[]>
  Everyone: boolean
  Facets: {
    people: string[] | null
    agentTypes: string[] | null
    agents: string[] | null
    providers: string[] | null
    models: string[] | null
  }
  Building: { building: boolean; done: number; total: number }
  Figures: {
    Spent: string
    SpentCaption: string
    In: string
    Out: string
    Cache: string
    Unpriced: string
    Calls: number
    Refused: number
  }
  Rows: UsageRow[] | null
  Columns: { Label: string; Href: string; Sorted: string; Num: boolean }[]
  States: { Label: string; Href: string; Count: string; Current: boolean }[]
  ShowWho: boolean
  Filtered: boolean
  Clear: string
  Newest: string
  Next: string
  Export: string
}

export interface UsageData {
  Base: string
  Tabs: UsageTabs
  Usage?: UsageView
}

export interface SessionData {
  Base: string
  Session: {
    ID: string
    Person: string
    Agent: string
    Engine: string
    Search: string
    Calls: {
      At: string
      AtFull: string
      Task: string
      Record: string
      Model: string
      Provider: string
      Key: string
      In: string
      Out: string
      Cache: string
      Cost: string
      Refused: string
      Translated: string
      Status: number
      Unpriced: boolean
    }[]
    Next: string
    Newest: string
  }
}

// People: who is in the organization.
export interface PersonRow {
  Email: string
  Name: string
  Role: string
  LastSignIn: number
  Budget: { Set: boolean; Limit: number; Passed: boolean }
  Spent: { calls: number; usd: number; unmeasured: number } | null
}

export interface PeopleData {
  Refused: string
  Manage?: string
  RoleAction?: string
  Back?: string
  Unreadable?: string
  SpendUnreadable?: string
  Tab?: string
  Tabs?: {
    Key: string
    Label: string
    Href: string
    Count: number
    On: boolean
  }[]
  People?: PersonRow[]
  Pages?: { Shown: string; Previous: string; Next: string }
  Invited?: { Email: string; By: string; Expires: number; Expired: boolean }[]
  Admins?: number
  Tokens?: boolean
  Budgets?: boolean
}
