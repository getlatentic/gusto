import { useRef, useState, type ReactNode } from "react"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { withBase } from "@/lib/base"

// ConfirmForm is a form that asks first: sending it opens the question, and only a yes sends it.
export function ConfirmForm({
  action,
  question,
  yes,
  send,
  testId,
  children,
}: {
  action: string
  question: string
  yes: string
  send: (form: HTMLFormElement) => Promise<void>
  testId?: string
  children: ReactNode
}) {
  const form = useRef<HTMLFormElement>(null)
  const [asking, setAsking] = useState(false)
  return (
    <>
      <form
        ref={form}
        method="post"
        action={withBase(action)}
        className="m-0"
        data-testid={testId}
        onSubmit={(event) => {
          event.preventDefault()
          setAsking(true)
        }}
      >
        {children}
      </form>
      <AlertDialog open={asking} onOpenChange={setAsking}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{question}</AlertDialogTitle>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              data-testid="confirm"
              variant="destructive"
              onClick={() => {
                setAsking(false)
                if (form.current) void send(form.current)
              }}
            >
              {yes}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
