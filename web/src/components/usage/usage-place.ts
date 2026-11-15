// The usage is shown in two places: the console's, of everyone, and a person's own. Each has its summary of the month and its
// conversations at its own addresses; what a person may see in them the server decides.
export interface UsagePlace {
  summary: string
  conversations: string
}

export const consoleUsage: UsagePlace = {
  summary: "/admin/spending",
  conversations: "/admin/usage",
}

export const personUsage: UsagePlace = {
  summary: "/spend",
  conversations: "/usage",
}
