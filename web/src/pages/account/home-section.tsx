import { Notice } from "@/components/page"
import { ago } from "@/lib/format"
import { ConfirmForm } from "@/pages/account/confirm-form"
import type { KeptHome } from "@/pages/account/types"
import type { AccountForm } from "@/pages/account/use-account-form"

// HomeSection is the home a person's agents keep between tasks: how much of its limit it takes, whether it is full, and the way to
// start it again empty.
export function HomeSection({
  home,
  reset,
  form,
}: {
  home: KeptHome
  reset: boolean
  form: AccountForm
}) {
  return (
    <section
      aria-labelledby="home-title"
      className="mt-8 grid max-w-prose gap-3"
      data-testid="home"
    >
      <h2 id="home-title" className="m-0 text-base font-semibold">
        Your agents' home
      </h2>
      <p className="m-0 text-sm text-muted-foreground">
        Between tasks your agents keep what they install and download for you in
        one home, so the next task starts where the last one stopped. Your tasks
        and conversations are not kept in it.
      </p>
      {reset && (
        <Notice tone="plain">
          Your agents' home is reset. The next task starts with an empty one.
        </Notice>
      )}
      {home.Counted ? (
        <>
          <p className="m-0 text-sm">
            It takes <strong>{home.Used}</strong> of the{" "}
            <strong>{home.Limit}</strong> it may, counted {ago(home.At)}.
          </p>
          {home.Full && (
            <Notice>
              It is full, so a task you ask for is refused until you reset it.
            </Notice>
          )}
          <ConfirmForm
            action="/connections/home/reset"
            label="Reset the home"
            question="Reset your agents' home?"
            consequence="What they kept in it is deleted: caches, downloads and tools they installed."
            send={(sent) => void form.send(sent)}
            testId="reset-home"
          />
        </>
      ) : (
        <p className="m-0 text-sm text-muted-foreground">
          Nothing is counted yet. It is counted when your first task ends.
        </p>
      )}
    </section>
  )
}
