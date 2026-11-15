import { useSearchParams } from "react-router"

// The new agent dialog is opened by the address, ?new=1, so a refused agent lands the person back in it with why. Closing it
// takes the word and the refusal off.
export function useNewAgentDialog(): [boolean, (open: boolean) => void] {
  const [params, setParams] = useSearchParams()
  const show = (open: boolean) =>
    setParams(
      (now) => {
        const next = new URLSearchParams(now)
        next.delete("new")
        next.delete("refused")
        next.delete("refused-seal")
        if (open) next.set("new", "1")
        return next
      },
      { replace: true }
    )
  return [params.has("new"), show]
}
