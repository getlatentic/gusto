import { BlocksIcon, CpuIcon, GaugeIcon, UsersRoundIcon } from "lucide-react"

import {
  BudgetsPage,
  ModelsPage,
  ToolsPage,
  type Control,
} from "@/cloud/pages/landing/demo/control-pages"
import {
  PageHeader,
  Shell,
  SideRow,
  SideTitle,
} from "@/cloud/pages/landing/demo/demo-parts"
import type { Frame, Scene } from "@/cloud/pages/landing/demo/script"

// controlScene: an admin raises Priya's budget, turns on one more model, and on the tools page sees an agent stopped at a tool
// the workspace does not allow.
export const controlScene: Scene<Control> = {
  length: 12500,
  start: {
    page: "budgets",
    editing: false,
    budget: "",
    saved: false,
    deepseek: false,
    blocked: false,
  },
  steps: [
    { at: 0, cursor: "total" },
    { at: 500, cursor: "budget", label: "Change Priya’s budget" },
    { at: 1300, click: true, set: { editing: true } },
    { at: 1600, type: { key: "budget", text: "$50", perChar: 160 } },
    { at: 2500, cursor: "save", label: "Save" },
    { at: 3200, click: true, set: { editing: false, saved: true } },
    { at: 4000, cursor: "models", label: "Models" },
    { at: 4700, click: true, set: { page: "models" } },
    { at: 5200, cursor: "deepseek", label: "Allow DeepSeek V4 Pro" },
    { at: 6000, click: true, set: { deepseek: true } },
    { at: 6800, cursor: "tools", label: "Tools/MCP and skills" },
    { at: 7500, click: true, set: { page: "tools" } },
    { at: 8400, set: { blocked: true } },
  ],
}

const titles: Record<Control["page"], string> = {
  budgets: "People and budgets",
  models: "Models",
  tools: "Tools/MCP and skills",
}

export function ControlScene({ frame }: { frame: Frame<Control> }) {
  const control = frame.state
  return (
    <Shell
      place="admin"
      side={
        <>
          <SideTitle>Admin</SideTitle>
          <SideRow icon={GaugeIcon}>Overview</SideRow>
          <SideRow icon={UsersRoundIcon} current={control.page === "budgets"}>
            {titles.budgets}
          </SideRow>
          <SideRow
            icon={CpuIcon}
            current={control.page === "models"}
            target="models"
          >
            {titles.models}
          </SideRow>
          <SideRow
            icon={BlocksIcon}
            current={control.page === "tools"}
            target="tools"
          >
            {titles.tools}
          </SideRow>
        </>
      }
    >
      <PageHeader title={titles[control.page]} />
      {control.page === "budgets" && <BudgetsPage control={control} />}
      {control.page === "models" && <ModelsPage control={control} />}
      {control.page === "tools" && <ToolsPage control={control} />}
    </Shell>
  )
}
