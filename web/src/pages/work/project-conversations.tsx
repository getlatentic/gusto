import { AppLink } from "@/components/app-link"
import { Card } from "@/components/ui/card"
import { ago } from "@/lib/format"
import type { ProjectData } from "@/pages/work/types"

// ProjectConversations is every conversation held with the agent, newest first, each with the last thing said in it.
export function ProjectConversations({ page }: { page: ProjectData }) {
  return (
    <section aria-labelledby="conversations">
      <h2 id="conversations" className="m-0 mb-3 text-sm font-semibold">
        Conversations
      </h2>
      <Card className="py-0">
        <ol className="m-0 list-none divide-y p-0" data-testid="conversations">
          {page.Conversations.map((each) => (
            <li key={each.Path}>
              <AppLink
                href={each.Path}
                data-testid="conversation"
                className="grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-0.5 px-4 py-3 text-foreground no-underline hover:bg-muted/50"
              >
                <span className="truncate font-medium" title={each.Name}>
                  {each.Name}
                </span>
                <span
                  className="col-start-1 row-start-2 truncate text-sm text-muted-foreground"
                  title={each.Latest || undefined}
                >
                  {each.Latest || "Nothing said yet"}
                </span>
                <span className="col-start-2 row-start-1 text-xs text-subtle">
                  {each.LatestQueued ? ago(each.LatestQueued) : ""}
                </span>
              </AppLink>
            </li>
          ))}
        </ol>
      </Card>
    </section>
  )
}
