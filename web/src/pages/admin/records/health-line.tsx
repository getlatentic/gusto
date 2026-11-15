import { Notice } from "@/components/page"
import type { Health } from "@/pages/admin/records/types"

// HealthLine is the record in one line at the top of every tab: a quiet line with a dot when all holds, and a notice when not.
export function HealthLine({ health }: { health: Health }) {
  if (health.Broken || health.LogBad || health.RunsBad) {
    return (
      <Notice>
        <strong>
          {health.Broken
            ? "The record is broken."
            : "Something does not check out."}
        </strong>
        {health.Logged && ` ${health.Log}.`} {health.Runs}.
      </Notice>
    )
  }
  return (
    <p
      role="status"
      className="m-0 mb-3 flex items-start gap-2 text-sm text-muted-foreground"
    >
      <span
        className="mt-1.5 size-2 flex-none rounded-full bg-ok"
        aria-hidden="true"
      />
      <span>
        Intact{health.Logged && ` · ${health.Log}`} · {health.Runs}
      </span>
    </p>
  )
}
