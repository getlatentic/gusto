import { useQuery } from "@tanstack/react-query"

import { getJSON } from "@/lib/api"
import type { PersonShell } from "@/domain/side-list"

const shellPath = "/app/api/shell"

// useShell is the side list and who is signed in. Every page of a person's own reads the same copy; it is asked again every
// 20 seconds while the page is in view, so unread counts move without a reload, and at once after the page changes a room.
export function useShell() {
  return useQuery({
    queryKey: [shellPath],
    queryFn: ({ signal }) => getJSON<PersonShell>(shellPath, signal),
    refetchInterval: 20_000,
  })
}

export const shellKey = [shellPath]
