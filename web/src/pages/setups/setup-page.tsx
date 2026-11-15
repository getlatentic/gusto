import { useParams } from "react-router"

import { AppLink } from "@/components/app-link"
import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody } from "@/components/page"
import { Section } from "@/components/section"
import { NoServers, UseLine } from "@/components/skill-set/set-gives"
import { SetSkills } from "@/components/skill-set/set-skills"
import { serversLine } from "@/domain/skill-sets"
import { usePageData } from "@/lib/api"
import type { SetupData, SetupStarter } from "@/pages/setups/types"

const crumbs = [{ label: "Start an agent", href: "/presets" }]

// SetupPage is one skill set as anybody reads it: its skills and its servers, and the agents that can start with it. Changing it
// is the console's.
export function SetupPage() {
  const name = useParams().name ?? ""
  const loaded = usePageData<SetupData>()
  return (
    <>
      <PageBar title={name} crumbs={crumbs} />
      <PageBody>
        <Loading loaded={loaded}>
          {(page) => <SetupParts page={page} />}
        </Loading>
      </PageBody>
    </>
  )
}

function SetupParts({ page }: { page: SetupData }) {
  return (
    <>
      {page.Refused && <Notice>{page.Refused}</Notice>}
      <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,20rem)] items-start gap-6 max-lg:grid-cols-1">
        <SetSkills skills={page.Set.Skills} />
        <aside className="grid min-w-0 gap-6" aria-label="About this skill set">
          <Section title="MCP servers">
            {page.Offers ? (
              <p className="m-0 text-sm text-muted-foreground">
                {serversLine(page.Servers)}
              </p>
            ) : (
              <NoServers />
            )}
          </Section>
          {page.Uses.length > 0 && (
            <Section title="Integrations">
              {page.Uses.map((use) => (
                <UseLine key={use.Name} use={use} />
              ))}
            </Section>
          )}
          <Section title="Start with it">
            <ul className="m-0 grid list-none gap-2 p-0 text-sm">
              {page.StartWith.map((starter) => (
                <li key={starter.Name}>
                  <StartWith starter={starter} />
                </li>
              ))}
            </ul>
          </Section>
        </aside>
      </div>
    </>
  )
}

// StartWith is an agent type to start with the skill set, or one that cannot take it, saying why.
function StartWith({ starter }: { starter: SetupStarter }) {
  if (starter.Unfit) {
    return (
      <span className="text-muted-foreground" title={starter.Unfit}>
        {starter.Name}
      </span>
    )
  }
  return (
    <AppLink
      href={starter.Href}
      className="font-medium text-foreground"
      data-testid="start-with"
    >
      {starter.Name}
    </AppLink>
  )
}
