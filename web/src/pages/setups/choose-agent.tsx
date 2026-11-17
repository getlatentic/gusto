import { FormSelect } from "@/components/form-select"
import { Button } from "@/components/ui/button"
import type { AgentType } from "@/pages/setups/types"
import { withBase } from "@/lib/base"

// ChooseAgent starts a template on an agent other than the one it starts on first.
export function ChooseAgent({
  name,
  agents,
}: {
  name: string
  agents: AgentType[]
}) {
  return (
    <form
      className="grid gap-3"
      method="post"
      action={withBase(`/templates/${encodeURIComponent(name)}`)}
    >
      <FormSelect
        name="harness"
        label="Agent"
        value={agents[0].Name}
        groups={[
          {
            choices: agents.map((agent) => ({
              value: agent.Name,
              label: agent.Title,
            })),
          },
        ]}
      />
      <div>
        <Button type="submit" variant="outline">
          Start with this agent
        </Button>
      </div>
    </form>
  )
}
