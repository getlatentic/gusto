import type { ReactNode } from "react"
import {
  BlocksIcon,
  MailIcon,
  ShieldAlertIcon,
  WalletIcon,
  type LucideIcon,
} from "lucide-react"

import { people } from "@/cloud/pages/landing/demo/demo-parts"
import { cn } from "@/lib/utils"

// The admin pages the control scene moves through: people and their budgets, the models agents may use, and the tools, skills
// and MCP servers they may reach.

export interface Control {
  page: "budgets" | "models" | "tools"
  editing: boolean
  budget: string
  saved: boolean
  deepseek: boolean
  blocked: boolean
}

const team = [
  { ...people.sam, role: "Admin", spent: 31, of: 50 },
  { ...people.priya, role: "Member", spent: 18, of: 20 },
  { ...people.jordan, role: "Member", spent: 9, of: 30 },
]

const body =
  "grid min-h-0 flex-1 content-start gap-5 px-8 pb-6 group-data-[compact=true]:px-4"
const list = "m-0 grid list-none rounded-xl border bg-surface p-0"
const row =
  "flex min-h-12 items-center gap-3 border-b px-4 py-2 text-sm last:border-b-0"

// BudgetsPage is what the team has spent this month against what it may, then each person's budget.
export function BudgetsPage({ control }: { control: Control }) {
  const budgets = team.map((person) =>
    person.name === "Priya" && control.saved ? 50 : person.of
  )
  const spent = team.reduce((sum, person) => sum + person.spent, 0)
  const total = budgets.reduce((sum, each) => sum + each, 0)
  return (
    <div className={body}>
      <section
        data-demo-target="total"
        className="grid gap-3 rounded-xl border bg-surface p-4"
      >
        <p className="m-0 flex items-baseline gap-2 text-sm text-muted-foreground">
          <span className="font-numeric text-2xl font-semibold text-foreground tabular-nums">
            ${spent}
          </span>
          spent of ${total} this month
        </p>
        <Bar part={spent / total} />
      </section>
      <ul className={list}>
        {team.map((person, at) => (
          <li key={person.name} className={row}>
            <img className="size-8 rounded-full" src={person.face} alt="" />
            <span className="grid min-w-0 flex-1 leading-tight">
              <span className="font-medium">{person.name}</span>
              <span className="text-xs text-muted-foreground">
                ${person.spent} spent · {person.role}
              </span>
            </span>
            {person.name === "Priya" ? (
              <BudgetField control={control} />
            ) : (
              <Budget>{`$${budgets[at]} / mo`}</Budget>
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}

function Bar({ part }: { part: number }) {
  return (
    <span className="block h-2 overflow-hidden rounded-full bg-sunken">
      <span
        className="bg-solid block h-full rounded-full transition-[width] duration-500"
        style={{ width: `${Math.min(1, part) * 100}%` }}
      />
    </span>
  )
}

function Budget({ children }: { children: string }) {
  return (
    <span className="flex h-8 w-28 items-center gap-1 rounded-lg border px-2.5 font-medium">
      <WalletIcon className="size-3.5 text-subtle" strokeWidth={1.75} />
      {children}
    </span>
  )
}

// BudgetField is Priya's budget, which the admin opens, types a new amount into and saves.
function BudgetField({ control }: { control: Control }) {
  if (!control.editing) {
    return (
      <span data-demo-target="budget">
        <Budget>{control.saved ? "$50 / mo" : "$20 / mo"}</Budget>
      </span>
    )
  }
  return (
    <span className="flex items-center gap-2">
      <span
        data-demo-target="budget"
        className="flex h-8 w-20 items-center rounded-lg border-2 border-ring px-2.5 font-medium"
      >
        {control.budget}
        <span className="ml-px h-4 w-px animate-pulse bg-foreground" />
      </span>
      <span
        data-demo-target="save"
        className="bg-solid text-on-solid flex h-8 items-center rounded-lg px-3 text-xs font-medium"
      >
        Save
      </span>
    </span>
  )
}

// Each model is drawn with its maker's mark; a black mark is turned white on a dark page.
const models = [
  {
    name: "Claude Sonnet 5.5",
    id: "anthropic/claude-sonnet-5-5",
    mark: "/static/agents/claude.svg",
  },
  {
    name: "GPT-5.5",
    id: "openai/gpt-5.5",
    mark: "/static/agents/codex.svg",
    black: true,
  },
  {
    name: "Gemini 3 Pro",
    id: "google/gemini-3-pro",
    mark: "/static/models/gemini.svg",
  },
  {
    name: "Kimi K2",
    id: "moonshotai/kimi-k2",
    mark: "/static/models/kimi.svg",
    black: true,
  },
  {
    name: "Qwen3 Coder",
    id: "qwen/qwen3-coder",
    mark: "/static/models/qwen.svg",
    off: true,
  },
  {
    name: "DeepSeek V4 Pro",
    id: "deepseek/deepseek-v4-pro",
    mark: "/static/agents/dsh.svg",
    deepseek: true,
  },
]

// ModelsPage is the models the workspace's agents may use, each by name and the id it is reached by.
export function ModelsPage({ control }: { control: Control }) {
  return (
    <div className={body}>
      <ul className={list}>
        {models.map((model) => (
          <Allowed
            key={model.id}
            icon={
              <img
                className={cn(
                  "size-4 flex-none object-contain",
                  model.black && "dark:invert"
                )}
                src={model.mark}
                alt=""
              />
            }
            name={model.name}
            detail={model.id}
            mono
            on={model.deepseek ? control.deepseek : !model.off}
            target={model.deepseek ? "deepseek" : undefined}
          />
        ))}
      </ul>
    </div>
  )
}

// ToolsPage is what agents may reach besides models, and an agent stopped at one it may not, told in a toast over the page so
// nothing on it moves.
export function ToolsPage({ control }: { control: Control }) {
  return (
    <div className={body}>
      {control.blocked && (
        <div
          role="status"
          className="absolute right-6 bottom-6 z-10 flex w-88 [animation:demo-in_240ms_ease-out] gap-3 rounded-xl border bg-surface p-4 text-sm shadow-lg group-data-[compact=true]:right-4 group-data-[compact=true]:left-4 group-data-[compact=true]:w-auto"
        >
          <ShieldAlertIcon
            className="mt-0.5 size-4 flex-none text-bad"
            strokeWidth={1.75}
          />
          <span className="grid gap-0.5 leading-snug">
            <span className="font-medium">Blocked: Gmail isn’t allowed</span>
            <span className="text-muted-foreground">
              Hermes tried to send an email. Nothing was sent.
            </span>
          </span>
        </div>
      )}
      <ul className={list}>
        <Allowed
          icon={<Glyph icon={BlocksIcon} />}
          name="GitHub"
          detail="Tool/MCP"
          on
        />
        <Allowed
          icon={<Glyph icon={BlocksIcon} />}
          name="Linear"
          detail="Tool/MCP"
          on
        />
        <Allowed
          icon={<Glyph icon={BlocksIcon} />}
          name="release-notes"
          detail="Skill"
          on
        />
        <Allowed
          icon={<Glyph icon={MailIcon} />}
          name="Gmail"
          detail="Tool/MCP"
          on={false}
        />
      </ul>
    </div>
  )
}

function Glyph({ icon: Icon }: { icon: LucideIcon }) {
  return <Icon className="size-4 flex-none text-subtle" strokeWidth={1.75} />
}

function Allowed({
  icon,
  name,
  detail,
  on,
  target,
  mono,
}: {
  icon: ReactNode
  name: string
  detail: ReactNode
  on: boolean
  target?: string
  // mono writes the detail as code, for an id.
  mono?: boolean
}) {
  return (
    <li className={row}>
      {icon}
      <span className="grid min-w-0 flex-1 leading-tight">
        <span className="font-medium">{name}</span>
        <span
          className={cn(
            "truncate text-xs text-muted-foreground",
            mono && "font-mono"
          )}
        >
          {detail}
        </span>
      </span>
      <span className="text-xs text-muted-foreground">
        {on ? "Allowed" : "Not allowed"}
      </span>
      <span
        data-demo-target={target}
        className={cn(
          "flex h-5 w-9 flex-none items-center rounded-full p-0.5 transition-colors duration-200",
          on ? "bg-solid" : "bg-input"
        )}
      >
        <span
          className={cn(
            "size-4 rounded-full bg-white shadow-sm transition-transform duration-200",
            on && "translate-x-4"
          )}
        />
      </span>
    </li>
  )
}
