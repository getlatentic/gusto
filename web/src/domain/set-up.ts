// What the page that starts one agent type opens on.

// chosenSetup is the skill set the link named, when the agent type can take it; else none.
export function chosenSetup(
  setups: { value: string }[],
  chosen: string
): string {
  return setups.some((each) => each.value === chosen) ? chosen : ""
}
