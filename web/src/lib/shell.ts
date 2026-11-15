import type { PersonShell } from "@/domain/side-list"
import { api } from "@/store/api"

export const shellPath = "/app/api/shell"

// useShell is the side list and who is signed in. Every page of a person's own reads the same copy; it is read again every 20
// seconds while the page is in view, so unread counts move without a reload, and at once after the page changes a room.
export function useShell() {
  const read = api.endpoints.page.useQuery(shellPath, {
    pollingInterval: 20_000,
    skipPollingIfUnfocused: true,
  })
  return { data: read.data as PersonShell | undefined }
}
