// The places down the rail of a person's own pages, and which one an address is in.

export type Place = "chat" | "files" | "members" | "usage" | "connections"

const placeOf: [RegExp, Place][] = [
  [/^\/(chat|instances\/[^/]+\/threads|rooms)(\/|$)/, "chat"],
  [/^\/files(\/|$)/, "files"],
  [/^\/(members|presets|harnesses)(\/|$)/, "members"],
  [/^\/(usage|spend)(\/|$)/, "usage"],
  [/^\/connections(\/|$)/, "connections"],
]

export function placeFor(path: string): Place | undefined {
  return placeOf.find(([pattern]) => pattern.test(path))?.[1]
}

// showsConversations is whether the page keeps the list of conversations beside it.
export function showsConversations(path: string): boolean {
  return placeFor(path) === "chat"
}
