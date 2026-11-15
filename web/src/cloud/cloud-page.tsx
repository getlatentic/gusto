import { lazy, Suspense, type ComponentType } from "react"

import { LandingPage } from "@/cloud/pages/landing/landing-page"
import type { CloudPage, CloudPageName, ServedPage } from "@/cloud/types"

type PageComponent = ComponentType<{ page: CloudPage }>

// The landing page is a visitor's first page, so it comes with this code. Each other kind of page is fetched when it is first
// drawn, so the landing page does not carry them.
const signIn = lazy(() =>
  import("@/cloud/pages/sign-in-page").then((m) => ({ default: m.SignInPage }))
)
const signUp = lazy(() =>
  import("@/cloud/pages/sign-up-page").then((m) => ({ default: m.SignUpPage }))
)
const account = (
  name: "ConfirmPage" | "OrgsPage" | "StartingPage" | "LeasePage"
) =>
  lazy(() =>
    import("@/cloud/pages/account-pages").then((m) => ({ default: m[name] }))
  )
const demo = (
  name:
    | "DemoWaitPage"
    | "DemoBookPage"
    | "DemoColleaguePage"
    | "DemoStopPage"
    | "DemoEndPage"
) =>
  lazy(() =>
    import("@/cloud/pages/demo-pages").then((m) => ({ default: m[name] }))
  )
const demoForm = lazy(() =>
  import("@/cloud/pages/demo-form-page").then((m) => ({
    default: m.DemoFormPage,
  }))
)
const org = lazy(() =>
  import("@/cloud/pages/org/org-page").then((m) => ({ default: m.OrgPage }))
)

// pages is each page the server names (internal/cloud, the name it renders), and what draws it.
export const pages: Record<CloudPageName, PageComponent> = {
  landing: LandingPage,
  signin: signIn,
  signup: signUp,
  confirm: account("ConfirmPage"),
  orgs: account("OrgsPage"),
  starting: account("StartingPage"),
  lease: account("LeasePage"),
  demo: demoForm,
  "demo-wait": demo("DemoWaitPage"),
  "demo-book": demo("DemoBookPage"),
  "demo-colleague": demo("DemoColleaguePage"),
  "demo-stop": demo("DemoStopPage"),
  "demo-end": demo("DemoEndPage"),
  org,
}

const OperatorPage = lazy(() =>
  import("@/cloud/pages/operator-page").then((m) => ({
    default: m.OperatorPage,
  }))
)

export function CloudPageView({ page }: { page: ServedPage }) {
  if (page.Page === "operator") {
    return (
      <Suspense fallback={null}>
        <OperatorPage page={page} />
      </Suspense>
    )
  }
  const Page = pages[page.Page]
  if (!Page) {
    return (
      <p className="p-8">This page is not one this app draws: {page.Page}.</p>
    )
  }
  return (
    <Suspense fallback={null}>
      <Page page={page} />
    </Suspense>
  )
}
