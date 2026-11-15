// The folders of a person's cloud computer, as /chat/folders gives them, and the state of the window that chooses one for a chat.
// Nothing here draws.

export interface Crumb {
  name: string
  at: string
}

export interface ShownFolders {
  at: string
  crumbs: Crumb[]
  folders:
    { name: string; at: string; harness?: string; modified: number }[] | null
  files: { name: string; size: number; modified: number }[] | null
  harness?: string
}

export interface Explorer {
  at: string
  // back is where the person was before, the newest last, for the Back button.
  back: string[]
  selected: string
  filter: string
  making: boolean
  problem: string
}

export type ExplorerMove =
  | { type: "open"; at: string }
  | { type: "back" }
  | { type: "select"; at: string }
  | { type: "filter"; text: string }
  | { type: "make" }
  | { type: "stopMaking" }
  | { type: "made"; at: string }
  | { type: "failed"; problem: string }

export function startExplorer(at: string): Explorer {
  return { at, back: [], selected: "", filter: "", making: false, problem: "" }
}

// explore is the window after a move. Opening a place remembers where the person was, and starts that place afresh.
export function explore(state: Explorer, move: ExplorerMove): Explorer {
  switch (move.type) {
    case "open":
      if (move.at === state.at) return state
      return { ...startExplorer(move.at), back: [...state.back, state.at] }
    case "back": {
      if (state.back.length === 0) return state
      const back = state.back.slice(0, -1)
      return { ...startExplorer(state.back[state.back.length - 1]), back }
    }
    case "select":
      return { ...state, selected: move.at }
    case "filter":
      return { ...state, filter: move.text }
    case "make":
      return { ...state, making: true, problem: "" }
    case "stopMaking":
      return { ...state, making: false, problem: "" }
    case "made":
      return { ...state, making: false, problem: "", selected: move.at }
    case "failed":
      return { ...state, problem: move.problem }
  }
}

// parentOf is the place one folder up, or the top.
export function parentOf(shown: ShownFolders): string {
  const crumbs = shown.crumbs
  return crumbs.length > 1 ? crumbs[crumbs.length - 2].at : ""
}

// chosenPlace is what Choose would choose: the folder selected, or the one open.
export function chosenPlace(state: Explorer): string {
  return state.selected || state.at
}

// matching are the folders and files whose name holds the filter, in any case.
export function matching<T extends { name: string }>(
  entries: T[] | null,
  filter: string
): T[] {
  const needle = filter.trim().toLowerCase()
  return (entries ?? []).filter((each) =>
    each.name.toLowerCase().includes(needle)
  )
}

// nameAt is what a place is called: a folder listed here, or the folder open.
export function nameAt(shown: ShownFolders, at: string): string {
  return (
    shown.folders?.find((folder) => folder.at === at)?.name ??
    shown.crumbs[shown.crumbs.length - 1]?.name ??
    ""
  )
}

// step moves the selection one folder down or up the list, staying within it.
export function step(
  shown: ShownFolders,
  filter: string,
  selected: string,
  by: 1 | -1
): string {
  const folders = matching(shown.folders, filter).map((folder) => folder.at)
  if (folders.length === 0) return ""
  const at = folders.indexOf(selected)
  const next = at < 0 ? (by > 0 ? 0 : folders.length - 1) : at + by
  return folders[Math.max(0, Math.min(folders.length - 1, next))]
}

// emptyWords is what an empty list says.
export function emptyWords(shown: ShownFolders, filter: string): string {
  if (filter.trim()) return "Nothing here matches."
  return shown.at
    ? "This folder is empty."
    : "No projects yet. Choose New Project to start one."
}
