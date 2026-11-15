// The rules the work pages draw by: the tone a run's state wears, what Activity offers and says when it lists nothing, how a
// comparison's run reads, and how a project's app and routine read.

export type Tone = "ok" | "bad" | "warn" | "plain"

// toneOf is the colour of a state's meaning: done well, done badly, still going, or neither.
export function toneOf(state: string): Tone {
  if (["passed", "finished", "signed", "live"].includes(state)) return "ok"
  if (state === "failed" || state === "interrupted") return "bad"
  if (state === "running" || state === "queued") return "warn"
  return "plain"
}

// isLive is a task still queued or running, whose page follows it as it goes.
export function isLive(state: string): boolean {
  return state === "queued" || state === "running"
}

// showsOutcome is whether a run's line says how it ended: a reply and a passed check are what is expected, and say nothing.
export function showsOutcome(outcome: string): boolean {
  return outcome !== "replied" && outcome !== "check passed"
}

interface Filtered {
  State: string
  Harness: string
  Harnesses: string[] | null
  Days: unknown[] | null
  Index: {
    Query: string
    Person: string
    Building: { building: boolean }
  } | null
}

// everythingHref is the tab that lifts the state filter and keeps the agent type.
export function everythingHref(harness: string): string {
  return harness ? `/jobs?agent_type=${encodeURIComponent(harness)}` : "/jobs"
}

// narrowed is whether anything narrows the list: a state, an agent type, a search or a person.
function narrowed(board: Filtered): boolean {
  return Boolean(
    board.State || board.Harness || board.Index?.Query || board.Index?.Person
  )
}

// showsFilters is whether Activity offers its search and choices: when it lists something, or something narrows it.
export function showsFilters(board: Filtered): boolean {
  return (board.Days?.length ?? 0) > 0 || narrowed(board)
}

// offersAgentTypes is whether there is an agent type to choose between, or one already chosen to lift.
export function offersAgentTypes(board: Filtered): boolean {
  return (board.Harnesses?.length ?? 0) > 1 || Boolean(board.Harness)
}

// whyEmpty is why Activity lists nothing: its filters let nothing through, the index is still being built, or nothing has
// happened yet.
export function whyEmpty(board: Filtered): "filtered" | "building" | "nothing" {
  if (narrowed(board)) return "filtered"
  if (board.Index?.Building.building) return "building"
  return "nothing"
}

// sortChosen is the order the list is in: the one the address names, or newest asked first.
export function sortChosen(sort: string): string {
  return sort || "queued"
}

// searchOf is the address a search form sends to, written as a browser writes a form it sends by GET.
export function searchOf(action: string, fields: [string, string][]): string {
  return `${action}?${new URLSearchParams(fields).toString()}`
}

// SideResult is how one agent's run in a comparison stands.
export type SideResult =
  "holding" | "working" | "passed" | "failed" | "stopped before the tests"

export function sideResult(side: {
  Holding: string
  Done: boolean
  Checked: boolean
  Passed: boolean
}): SideResult {
  if (side.Holding) return "holding"
  if (!side.Done) return "working"
  if (side.Checked && side.Passed) return "passed"
  if (side.Checked) return "failed"
  return "stopped before the tests"
}

// appIsRunning is an app that is live or starting, which is stopped rather than dismissed.
export function appIsRunning(state: string): boolean {
  return state === "live" || state === "starting"
}

// routineLast is how a routine's last slot reads, and in what tone.
export function routineLast(last: string): { words: string; tone: Tone } {
  if (last === "ran") return { words: "Last run ok", tone: "ok" }
  if (last === "not yet") return { words: "Not run yet", tone: "plain" }
  return { words: `Last run ${last}`, tone: "warn" }
}

// streamMost is how much of a running task's words its page keeps: a harness that loops can say any amount, and the whole
// transcript is a file of the record once it ends.
export const streamMost = 512 * 1024

// heard is what the page shows of a running task once it says another line: everything so far, cut from the front to the
// last most characters.
export function heard(so: string, line: string, most = streamMost): string {
  const next = so + line + "\n"
  return next.length > most ? next.slice(next.length - most) : next
}
