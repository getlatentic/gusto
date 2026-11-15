import type { Cost } from "@/pages/work/types"

// CostText is what a run cost, with what qualifies the figure as its title: the agent's own claim, or calls with no price.
export function CostText({ cost }: { cost: Cost }) {
  return cost.Title ? <span title={cost.Title}>{cost.Text}</span> : cost.Text
}
