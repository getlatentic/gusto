import { useMutation, useQuery } from "@tanstack/react-query"

import { Button } from "@/components/ui/button"
import { askCloud, tellCloud } from "@/lib/cloud"

interface Account {
  confirmed: boolean
  email: string
  jobs: number
}

// sentWords is what to tell the person after asking for the link again.
function sentWords(status: number): string {
  if (status >= 200 && status < 300) return "Sent. Check your inbox."
  if (status === 429)
    return "That address has been mailed too often. Try again later."
  return "The link could not be sent. Try again."
}

// EmailReminder asks a person who has not confirmed their address to, and mails the link again. Only gusto cloud knows
// whether they have.
export function EmailReminder({ from, send }: { from: string; send: string }) {
  const account = useQuery({
    queryKey: [from],
    queryFn: ({ signal }) => askCloud<Account>(from, signal),
    staleTime: 60_000,
  })
  const again = useMutation({ mutationFn: () => tellCloud(send) })
  if (!account.data || account.data.confirmed) return null
  // Once the link is on its way there is nothing more to send.
  const sent = again.isSuccess && again.data < 300
  const said = again.isError
    ? "The link could not be sent. Try again."
    : again.data !== undefined
      ? sentWords(again.data)
      : ""
  return (
    <aside
      aria-label="Confirm your email address"
      className="grid gap-2 rounded-md border bg-surface p-3 text-sm"
    >
      <p className="m-0 font-medium">Confirm your email address</p>
      <p className="m-0 text-muted-foreground">
        It lifts the trial to {account.data.jobs} tasks a month and lets you
        invite colleagues. We mailed a link to{" "}
        <span className="wrap-anywhere text-foreground">
          {account.data.email}
        </span>
        .
      </p>
      {said && (
        <p className="m-0 text-muted-foreground" role="status">
          {said}
        </p>
      )}
      <Button
        variant="outline"
        size="sm"
        disabled={again.isPending || sent}
        onClick={() => again.mutate()}
      >
        Send it again
      </Button>
    </aside>
  )
}
