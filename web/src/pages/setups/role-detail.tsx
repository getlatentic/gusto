import { AppLink } from "@/components/app-link"
import { Notice } from "@/components/page"
import { buttonVariants } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import type { Role } from "@/domain/roles"

// RoleDetail is the built-in preset picked: what it is for, what it can do, what to try asking, and the way to use it in a new
// chat, or why it cannot be used here.
export function RoleDetail({ role }: { role: Role }) {
  return (
    <section aria-label={role.Title} data-testid="role-detail">
      <Card className="flex flex-col gap-5 px-8 py-7 max-md:px-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0 flex-[1_1_17rem]">
            <span className="text-xs font-medium text-muted-foreground">
              {role.Industry}
            </span>
            <h2 className="mt-1.5 mb-2 text-xl">{role.Title}</h2>
            <p className="m-0 max-w-[35rem] text-base text-muted-foreground">
              {role.Summary}
            </p>
          </div>
          <div className="flex flex-none items-center gap-4">
            <AppLink
              href={`/templates/${encodeURIComponent(role.Name)}`}
              className="text-sm text-muted-foreground hover:text-foreground"
              data-testid="role-details"
            >
              Details
            </AppLink>
            {role.Blocked ? (
              <span
                className={buttonVariants({ className: "opacity-50" })}
                aria-disabled="true"
              >
                Use in a new chat
              </span>
            ) : (
              <AppLink
                href={`/chat?role=${encodeURIComponent(role.Name)}`}
                className={buttonVariants()}
              >
                Use in a new chat
              </AppLink>
            )}
          </div>
        </div>
        {role.Blocked && <Notice tone="warn">{role.Blocked}</Notice>}
        <div>
          <h3 className="m-0 mb-1 text-xs font-semibold text-muted-foreground">
            What it can do
          </h3>
          <dl className="m-0 divide-y border-y">
            {role.Skills.map((skill) => (
              <div
                key={skill.Title}
                className="grid grid-cols-[12rem_minmax(0,1fr)] gap-4 py-3 max-md:grid-cols-[minmax(0,1fr)] max-md:gap-1"
              >
                <dt className="font-medium">{skill.Title}</dt>
                <dd className="m-0 text-sm text-muted-foreground">
                  {skill.Blurb}
                </dd>
              </div>
            ))}
          </dl>
        </div>
        <div>
          <h3 className="m-0 mb-2 text-xs font-semibold text-muted-foreground">
            Try asking
          </h3>
          <ul className="m-0 grid list-none gap-2 p-0">
            {role.Starters.map((starter) => (
              <li
                key={starter}
                className="rounded-sm bg-muted px-3 py-2 text-sm text-muted-foreground"
              >
                {starter}
              </li>
            ))}
          </ul>
        </div>
      </Card>
    </section>
  )
}
