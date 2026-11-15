import { useRef, useState, type ReactNode } from "react"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"

// ConfirmForm is a form whose one button does what cannot be undone, so it asks first, and is sent only once the person says
// yes. send is how it goes: by the page, or as the browser sends it, for a form that leads away from the app.
export function ConfirmForm({
  action,
  label,
  question,
  consequence,
  send,
  testId,
}: {
  action: string
  label: string
  question: string
  consequence: ReactNode
  send: (form: HTMLFormElement) => void
  testId?: string
}) {
  const form = useRef<HTMLFormElement>(null)
  const [asking, setAsking] = useState(false)
  return (
    <>
      <form
        ref={form}
        method="post"
        action={action}
        className="m-0"
        data-testid={testId}
        onSubmit={(event) => {
          event.preventDefault()
          setAsking(true)
        }}
      >
        <Button type="submit" variant="destructive" size="sm">
          {label}
        </Button>
      </form>
      <AlertDialog open={asking} onOpenChange={setAsking}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{question}</AlertDialogTitle>
            <AlertDialogDescription>{consequence}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              data-testid={testId && `${testId}-confirm`}
              onClick={() => {
                setAsking(false)
                if (form.current) send(form.current)
              }}
            >
              {label}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
