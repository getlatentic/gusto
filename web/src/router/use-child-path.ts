import { useMatches } from "react-router"

// useChildPath is the address of the page a layout route draws in its outlet: where the route below the one named parent
// matched. A layout below it that keeps its own page while its children change, as the members list does, is one page here.
export function useChildPath(parent: string): string {
  const matches = useMatches()
  const at = matches.findIndex((match) => match.id === parent)
  return at < 0 ? "" : (matches[at + 1]?.pathname ?? "")
}
