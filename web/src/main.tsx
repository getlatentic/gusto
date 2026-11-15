import { lazy, StrictMode, Suspense } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter, Route, Routes } from "react-router"

import "./index.css"
import { ConsoleShell } from "@/components/console-shell"
import { TooltipProvider } from "@/components/ui/tooltip"

// Each page is fetched when it is first opened, so the first page drawn does not wait for the others' code.
const OverviewPage = lazy(() =>
  import("@/pages/admin/overview").then((m) => ({ default: m.OverviewPage }))
)
const SpendingPage = lazy(() =>
  import("@/pages/admin/spending").then((m) => ({ default: m.SpendingPage }))
)
const UsagePage = lazy(() =>
  import("@/pages/admin/usage").then((m) => ({ default: m.UsagePage }))
)
const UsageSessionPage = lazy(() =>
  import("@/pages/admin/usage-session").then((m) => ({
    default: m.UsageSessionPage,
  }))
)
const PeoplePage = lazy(() =>
  import("@/pages/admin/people").then((m) => ({ default: m.PeoplePage }))
)
const BudgetsPage = lazy(() =>
  import("@/pages/admin/budgets").then((m) => ({ default: m.BudgetsPage }))
)
const TokensPage = lazy(() =>
  import("@/pages/admin/tokens").then((m) => ({ default: m.TokensPage }))
)

// The pages the gusto server hands to this app. Each address is also a route on the server, which serves this app for it
// behind the same sign-in; any other address is a page the server draws.
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <TooltipProvider>
      <BrowserRouter>
        <Suspense>
          <Routes>
            <Route path="/admin" element={<ConsoleShell />}>
              <Route index element={<OverviewPage />} />
              <Route path="spending" element={<SpendingPage />} />
              <Route path="usage" element={<UsagePage />} />
              <Route path="usage/:session" element={<UsageSessionPage />} />
              <Route path="people" element={<PeoplePage />} />
              <Route path="budgets" element={<BudgetsPage />} />
              <Route path="tokens" element={<TokensPage />} />
            </Route>
          </Routes>
        </Suspense>
      </BrowserRouter>
    </TooltipProvider>
  </StrictMode>
)
