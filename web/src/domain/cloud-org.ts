// What an organization's page lets the person looking do to each member and team. Only an admin changes anything; the last
// admin is not offered to be made a runner, and nobody is offered to remove themselves.

export interface MemberLike {
  Email: string
  Role: string
}

export interface OrgViewer {
  IsAdmin: boolean
  Admins: number
  Person: { Email: string }
}

export interface MemberActions {
  makeRunner: boolean
  makeAdmin: boolean
  remove: boolean
}

export function memberActions(
  member: MemberLike,
  viewer: OrgViewer
): MemberActions {
  if (!viewer.IsAdmin) {
    return { makeRunner: false, makeAdmin: false, remove: false }
  }
  const admin = member.Role === "admin"
  return {
    makeRunner: admin && viewer.Admins > 1,
    makeAdmin: !admin,
    remove: member.Email !== viewer.Person.Email,
  }
}

// showsTeams says the page has a teams section: an admin may change them, or there are several to tell apart.
export const showsTeams = (viewer: OrgViewer, teams: number) =>
  viewer.IsAdmin || teams > 1

// currentStep is the stage a demo is at while it is made ready, or none once it is.
export function currentStep<T extends { Mark: string }>(
  steps: T[] | null
): T | undefined {
  return steps?.find((step) => step.Mark === "now")
}
