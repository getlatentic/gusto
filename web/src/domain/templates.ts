// Whether a template can be started here, and how.

export interface Startable {
  Restricted: boolean
}

// keptOut is restricted work where no model is inside the deployment's own network, which gusto will not send anywhere else.
export function keptOut(template: Startable, inBuilding: boolean): boolean {
  return template.Restricted && !inBuilding
}

// canStart is whether the template's start button starts it: it is not kept out, and some agent here can take it.
export function canStart(
  template: Startable,
  inBuilding: boolean,
  agents: number
): boolean {
  return !keptOut(template, inBuilding) && agents > 0
}

// choosesAgent is whether the page offers a choice of agent: more than one can take the template, and it is not kept out.
export function choosesAgent(
  template: Startable,
  inBuilding: boolean,
  agents: number
): boolean {
  return !keptOut(template, inBuilding) && agents > 1
}
