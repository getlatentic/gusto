// The built-in presets page: a list of roles narrowed by a search and a field of work, and the one picked beside it.

export interface Role {
  Name: string
  Title: string
  Industry: string
  Summary: string
  Skills: { Title: string; Blurb: string }[]
  Starters: string[]
  // Blocked is why the role cannot be started here, or empty when it can.
  Blocked: string
}

// narrowRoles is the roles in the field chosen ("" for every field) whose title, field or summary holds the search.
export function narrowRoles(roles: Role[], field: string, search: string) {
  const needle = search.trim().toLowerCase()
  return roles.filter(
    (role) =>
      (!field || role.Industry === field) &&
      (!needle ||
        [role.Title, role.Industry, role.Summary]
          .join(" ")
          .toLowerCase()
          .includes(needle))
  )
}

// pickedRole is the role shown: the one chosen while the list still shows it, else the first the list shows, else the one chosen.
export function pickedRole(
  roles: Role[],
  shown: Role[],
  chosen: string
): Role | undefined {
  return (
    shown.find((role) => role.Name === chosen) ??
    shown[0] ??
    roles.find((role) => role.Name === chosen)
  )
}
