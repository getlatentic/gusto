import { Button } from "@/components/ui/button"
import { PluginName } from "@/pages/admin/apps/add-skill-forms"
import { Choices } from "@/pages/admin/apps/field"
import { Section } from "@/components/section"
import type { SkillSetData } from "@/pages/admin/apps/types"
import { NoServers, UseLine } from "@/components/skill-set/set-gives"

// SkillSetSide is what the set is given besides its skills: the deployment's MCP servers, and imported plugins.
export function SkillSetSide({ page }: { page: SkillSetData }) {
  return (
    <aside className="grid min-w-0 gap-6" aria-label="About this skill set">
      <Section title="MCP servers">
        {page.Servers.length > 0 ? <ServersForm page={page} /> : <NoServers />}
      </Section>
      {(page.Imported.length > 0 || page.Uses.length > 0) && (
        <Section title="Integrations">
          {page.Imported.length > 0 && <IntegrationsForm page={page} />}
          {page.Uses.map((use) => (
            <UseLine key={use.Name} use={use} />
          ))}
        </Section>
      )}
    </aside>
  )
}

// A skill set is saved whole, so each of its two forms carries the other's choices as they are.
function Kept({ name, values }: { name: string; values: string[] }) {
  return values.map((value) => (
    <input key={value} type="hidden" name={name} value={value} />
  ))
}

function ServersForm({ page }: { page: SkillSetData }) {
  const plugins = page.Imported.filter((each) => each.Chosen)
  return (
    <form className="grid gap-3" method="post" action="/admin/skill-sets">
      <input type="hidden" name="name" value={page.Set.Name} />
      <Kept name="integration" values={plugins.map((each) => each.Name)} />
      <Choices
        name="server"
        choices={page.Servers.map((server) => ({
          value: server.Name,
          checked: server.Chosen,
          label: (
            <>
              {server.Name}{" "}
              <span className="text-muted-foreground">{server.Where}</span>
            </>
          ),
        }))}
      />
      <div>
        <Button variant="outline" size="sm" type="submit">
          Save
        </Button>
      </div>
    </form>
  )
}

function IntegrationsForm({ page }: { page: SkillSetData }) {
  const servers = page.Servers.filter((each) => each.Chosen)
  return (
    <form className="mb-3 grid gap-3" method="post" action="/admin/skill-sets">
      <input type="hidden" name="name" value={page.Set.Name} />
      <Kept name="server" values={servers.map((each) => each.Name)} />
      <Choices
        name="integration"
        choices={page.Imported.map((plugin) => ({
          value: plugin.Name,
          checked: plugin.Chosen,
          label: <PluginName plugin={plugin} />,
        }))}
      />
      <div>
        <Button variant="outline" size="sm" type="submit">
          Save
        </Button>
      </div>
    </form>
  )
}
