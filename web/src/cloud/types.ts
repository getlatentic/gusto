// A page of gusto cloud as its server gives it, inside the page (internal/cloud/page_view.go). Page names which page it is.

export interface CloudOrg {
  Slug: string
  Name: string
  Admin: string
  Demo: boolean
}

export interface CloudQuota {
  Tasks: number
  Concurrent: number
  Members: number
  Computers: number
  Monthly: boolean
}

export interface CloudMember {
  Email: string
  Role: string
  Computer: string
}

export interface CloudSpace {
  ID: string
  Name: string
  People: { Email: string; Removable: boolean }[] | null
  Others: string[] | null
}

export interface CloudStep {
  Label: string
  Mark: string
}

// The pages internal/cloud renders, by the name it renders them with.
export type CloudPageName =
  | "landing"
  | "signin"
  | "signup"
  | "confirm"
  | "orgs"
  | "starting"
  | "lease"
  | "demo"
  | "demo-wait"
  | "demo-book"
  | "demo-colleague"
  | "demo-stop"
  | "demo-end"
  | "org"

export interface CloudPage {
  Page: CloudPageName
  Title: string

  Error: string
  // Ask is the question the sign-up form asks next when nothing was wrong, only missing.
  Ask: string
  Email: string
  OrgName: string
  Name: string
  Notice: string
  More: boolean
  Taken: boolean
  WorkOnly: boolean
  ForTeam: boolean
  // Google says a person may sign in and sign up with a Google account.
  Google: boolean
  SignedIn: boolean
  Sent: boolean

  Token: string
  Action: string
  Button: string
  Inviter: string

  Org: CloudOrg | null
  Orgs: CloudOrg[] | null
  Several: boolean
  Person: { Email: string; Trial: boolean }
  Address: string
  Domain: string
  Credit: { Left: string; Of: string } | null
  Plan: {
    Words: string
    Offer: string
    Subscribe: boolean
    Manage: boolean
  } | null
  Quota: CloudQuota
  Verified: CloudQuota

  Computers: number
  Server: string
  CanOpen: boolean
  // Watch is set on a page that waits on the organization's server: the state it was drawn in.
  Watch?: { State: string }

  Members: CloudMember[] | null
  Invited: string[] | null
  Admins: number
  IsAdmin: boolean
  CanInvite: boolean
  Teams: CloudSpace[] | null
  Link: string

  DemoLeft: string
  DemoLeftLong: string
  DemoWords: string
  Steps: CloudStep[] | null
  Book: string
  BookFrame: string
  TurnstileSite: string
  Contact: CloudContact | null
  Extendable: boolean
  Ended: boolean
  Ending: boolean

  Lease: { Left: string; Ends: string; Step: string; CanExtend: boolean } | null
}

// CloudContact is the landing page's note to the team (internal/cloud/contact_page.go): what was typed, why it was refused, or
// that it was sent.
export interface CloudContact {
  Error: string
  Email: string
  Company: string
  Note: string
  Sent: boolean
}

// The operator's page (internal/operator/view.go), which the same entry draws on the operator's own server.
export interface OperatorView {
  Page: "operator"
  Title: string
  Notice: string
  Paused: boolean
  LimitsSet: boolean
  Limits: {
    perDay: number
    perPerson: number
    perCompany: number
    maxOrgs: number
  }
  Running:
    { Name: string; Company: string; State: string; Left: string }[] | null
  Leads:
    | {
        Asked: string
        AskedAt: string
        Name: string
        Email: string
        Company: string
        Outcome: string
      }[]
    | null
  Contacts:
    | {
        Sent: string
        SentAt: string
        Email: string
        Company: string
        Note: string
      }[]
    | null
}

export type ServedPage = CloudPage | OperatorView
