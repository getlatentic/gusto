import type { CloudPage, CloudPageName, OperatorView } from "@/cloud/types"

const quota = {
  Tasks: 20,
  Concurrent: 1,
  Members: 1,
  Computers: 1,
  Monthly: false,
}

// cloudPage is a page as the server gives it, with nothing set but its name and what a test sets.
export function cloudPage(
  name: CloudPageName,
  set: Partial<CloudPage> = {}
): CloudPage {
  return {
    Page: name,
    Title: "gusto",
    Error: "",
    Ask: "",
    Email: "",
    OrgName: "",
    Name: "",
    Notice: "",
    More: false,
    Taken: false,
    WorkOnly: false,
    ForTeam: false,
    Google: false,
    SignedIn: false,
    Sent: false,
    Token: "",
    Action: "",
    Button: "",
    Inviter: "",
    Org: null,
    Orgs: null,
    Several: false,
    Person: { Email: "", Trial: false },
    Address: "",
    Domain: "",
    Credit: null,
    Plan: null,
    Quota: quota,
    Verified: { ...quota, Tasks: 200, Monthly: true },
    Computers: 0,
    Server: "",
    CanOpen: false,
    Members: null,
    Invited: null,
    Admins: 0,
    IsAdmin: false,
    CanInvite: false,
    Teams: null,
    Link: "",
    DemoLeft: "",
    DemoLeftLong: "",
    DemoWords: "",
    Steps: null,
    Book: "",
    BookFrame: "",
    TurnstileSite: "",
    Contact: null,
    Extendable: false,
    Ended: false,
    Ending: false,
    Lease: null,
    ...set,
  }
}

export function operatorPage(set: Partial<OperatorView> = {}): OperatorView {
  return {
    Page: "operator",
    Title: "gusto operator",
    Notice: "",
    Paused: false,
    LimitsSet: false,
    Limits: { perDay: 20, perPerson: 0, perCompany: 3, maxOrgs: 1 },
    Running: null,
    Leads: null,
    Contacts: null,
    ...set,
  }
}
