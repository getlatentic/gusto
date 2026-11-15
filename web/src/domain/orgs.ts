// The organizations and workspaces a person is in, as gusto cloud lists them, made into the switcher's lines. An organization
// the person is in two or more workspaces of is a heading over one line per workspace; one with a single workspace is one line.

export interface CloudOrg {
  slug: string
  name: string
  open: string
  workspaces?: { id: string; name: string; open: string }[]
}

export type OrgLine =
  | { kind: "heading"; key: string; text: string }
  | {
      kind: "place"
      key: string
      text: string
      href: string
      current: boolean
      inset: boolean
    }

export function orgLines(
  orgs: CloudOrg[],
  current: string,
  workspace: string
): OrgLine[] {
  return orgs.flatMap((org): OrgLine[] => {
    const here = org.slug === current
    const spaces = org.workspaces ?? []
    if (spaces.length < 2) {
      return [
        {
          kind: "place",
          key: org.slug,
          text: org.name,
          href: org.open,
          current: here,
          inset: false,
        },
      ]
    }
    return [
      { kind: "heading", key: `${org.slug} heading`, text: org.name },
      ...spaces.map((space): OrgLine => ({
        kind: "place",
        key: `${org.slug} ${space.id}`,
        text: space.name,
        // A workspace of this organization is switched to here; another organization's is opened at its own address.
        href: here ? `/workspace/${encodeURIComponent(space.id)}` : space.open,
        current: here && space.id === workspace,
        inset: true,
      })),
    ]
  })
}
