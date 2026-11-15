import { useSearchParams } from "react-router"

// The room's dialogs are opened by the address: ?members=1, ?add=1 or ?guidelines=1. A refused change lands the person on the
// dialog it came from, with why; the dialog shown is read from the address, and closing it takes the word off.
export type RoomDialog = "members" | "add" | "guidelines"

const dialogs: RoomDialog[] = ["members", "add", "guidelines"]

export function useRoomDialog(): [
  RoomDialog | undefined,
  (dialog: RoomDialog | undefined) => void,
] {
  const [params, setParams] = useSearchParams()
  const open = dialogs.find((dialog) => params.has(dialog))
  const show = (dialog: RoomDialog | undefined) =>
    setParams(
      (now) => {
        const next = new URLSearchParams(now)
        for (const each of dialogs) next.delete(each)
        next.delete("refused")
        next.delete("refused-seal")
        if (dialog) next.set(dialog, "1")
        return next
      },
      { replace: true }
    )
  return [open, show]
}
