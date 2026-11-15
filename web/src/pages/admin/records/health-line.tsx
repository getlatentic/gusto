import { Notice } from "@/components/page"
import type { Health } from "@/pages/admin/records/types"

// HealthLine is the record's verdict at the top of every tab: one quiet word when all holds, with what was checked for a
// screen reader and on hover, and a notice saying what is wrong when not.
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
  const detail = [health.Logged && health.Log, health.Runs]
    .filter(Boolean)
    .join(", ")
  return (
    <p
      role="status"
      className="m-0 mb-3 flex items-start gap-2 text-sm text-muted-foreground"
    >
      <span
        className="mt-1.5 size-2 flex-none rounded-full bg-ok"
        aria-hidden="true"
      />
      <span title={detail}>
        Intact<span className="sr-only">: {detail}</span>
      </span>
    </p>
  )
}
