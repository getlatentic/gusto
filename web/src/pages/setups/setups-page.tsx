import { AppLink } from "@/components/app-link"
import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { Card, CardContent } from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { usePageData } from "@/lib/api"
import { holds } from "@/domain/skill-sets"
import type { SetupRow, SetupsData } from "@/pages/setups/types"

const crumbs = [{ label: "New agent", href: "/presets" }]

// SetupsPage is the skill sets anybody may give an agent: instructions and tools.
export function SetupsPage() {
  const loaded = usePageData<SetupsData>()
  return (
    <>
      <PageBar title="Skill sets" crumbs={crumbs} />
      <PageBody>
        <PageSub>Instructions and tools you can give any agent.</PageSub>
        <Loading loaded={loaded}>
          {(page) => (
            <>
              {page.Refused && <Notice>{page.Refused}</Notice>}
              {page.Keeps && <SetupList setups={page.Setups} />}
            </>
          )}
        </Loading>
      </PageBody>
    </>
  )
}

function SetupList({ setups }: { setups: SetupRow[] }) {
  if (setups.length === 0) {
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>No skill sets yet</EmptyTitle>
          <EmptyDescription>
            Admins add them in Admin, under Apps &amp; skills.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  return (
    <ul
      className="m-0 grid list-none grid-cols-[repeat(auto-fill,minmax(min(16rem,100%),1fr))] gap-4 p-0"
      aria-label="Skill sets"
    >
      {setups.map((setup) => (
        <li key={setup.Name}>
          <Card className="relative h-full transition-colors hover:ring-foreground/25">
            <CardContent className="grid content-start gap-1">
              <AppLink
                href={`/setups/${encodeURIComponent(setup.Name)}`}
                className="font-heading text-sm font-semibold text-foreground no-underline after:absolute after:inset-0"
                data-testid="setup-link"
              >
                {setup.Name}
              </AppLink>
              <span className="text-sm text-muted-foreground">
                {holds(setup)}
              </span>
            </CardContent>
          </Card>
        </li>
      ))}
    </ul>
  )
}
