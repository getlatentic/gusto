import type { ComponentType } from "react"
import type { RouteObject } from "react-router"

import { ConsoleShell } from "@/components/console-shell"
import { PersonShell } from "@/components/person/person-shell"
import { shellPath } from "@/lib/shell"
import { membersRoute } from "@/pages/members/members-routes"
import { readAhead } from "@/router/read-ahead"
import { RouteError } from "@/router/route-error"
import type { AppStore } from "@/store"

type PageCode = () => Promise<{ Component: ComponentType }>

// appRoutes are the pages the gusto server hands to this app. Each address is also a route on the server, which serves this app
// for it behind the same sign-in; any other address is a page the server draws. Each page's code is fetched when it is first
// opened, so the first page drawn does not wait for the others', and its data is read alongside its code.
export function appRoutes(store: AppStore): RouteObject[] {
  const page = (at: { path: string } | { index: true }, code: PageCode) =>
    ({ ...at, lazy: code, loader: readAhead(store) }) as RouteObject
  return [
    {
      errorElement: <RouteError />,
      HydrateFallback: Nothing,
      children: [
        {
          id: "person",
          Component: PersonShell,
          loader: readAhead(store, () => shellPath),
          children: [
            page({ path: "/chat" }, () =>
              import("@/pages/chat/chat-page").then((m) => ({
                Component: m.ChatPage,
              }))
            ),
            page({ path: "/rooms/new" }, () =>
              import("@/pages/rooms/new-room-page").then((m) => ({
                Component: m.NewRoomPage,
              }))
            ),
            page({ path: "/instances/:id/threads/:thread" }, () =>
              import("@/pages/conversation/conversation-page").then((m) => ({
                Component: m.ConversationPage,
              }))
            ),
            membersRoute(store),
            page({ path: "/files" }, () =>
              import("@/pages/files/files-page").then((m) => ({
                Component: m.FilesPage,
              }))
            ),
            page({ path: "/instances/:id/file" }, () =>
              import("@/pages/files/file-page").then((m) => ({
                Component: m.FilePage,
              }))
            ),
            page({ path: "/jobs/:id/files/:name" }, () =>
              import("@/pages/files/file-page").then((m) => ({
                Component: m.FilePage,
              }))
            ),
            page({ path: "/setups" }, () =>
              import("@/pages/setups/setups-page").then((m) => ({
                Component: m.SetupsPage,
              }))
            ),
            page({ path: "/setups/:name" }, () =>
              import("@/pages/setups/setup-page").then((m) => ({
                Component: m.SetupPage,
              }))
            ),
            page({ path: "/presets" }, () =>
              import("@/pages/setups/presets-page").then((m) => ({
                Component: m.PresetsPage,
              }))
            ),
            page({ path: "/templates/:name" }, () =>
              import("@/pages/setups/template-page").then((m) => ({
                Component: m.TemplatePage,
              }))
            ),
            page({ path: "/new/blank" }, () =>
              import("@/pages/setups/blank-page").then((m) => ({
                Component: m.BlankPage,
              }))
            ),
            page({ path: "/instances/new" }, () =>
              import("@/pages/setups/set-up-page").then((m) => ({
                Component: m.SetUpPage,
              }))
            ),
            page({ path: "/usage" }, () =>
              import("@/pages/usage/usage-pages").then((m) => ({
                Component: m.MyUsagePage,
              }))
            ),
            page({ path: "/usage/:session" }, () =>
              import("@/pages/usage/usage-pages").then((m) => ({
                Component: m.MySessionPage,
              }))
            ),
            page({ path: "/spend" }, () =>
              import("@/pages/usage/usage-pages").then((m) => ({
                Component: m.MySpendPage,
              }))
            ),
            page({ path: "/connections" }, () =>
              import("@/pages/account/connections-page").then((m) => ({
                Component: m.ConnectionsPage,
              }))
            ),
            page({ path: "/device" }, () =>
              import("@/pages/account/device-page").then((m) => ({
                Component: m.DevicePage,
              }))
            ),
            // The work pages: Activity, a task, a project, how each agent type did, and comparing agents.
            page({ path: "/jobs" }, () =>
              import("@/pages/work/activity-page").then((m) => ({
                Component: m.ActivityPage,
              }))
            ),
            page({ path: "/jobs/:id" }, () =>
              import("@/pages/work/task-page").then((m) => ({
                Component: m.TaskPage,
              }))
            ),
            page({ path: "/instances/:id" }, () =>
              import("@/pages/work/project-page").then((m) => ({
                Component: m.ProjectPage,
              }))
            ),
            page({ path: "/engines" }, () =>
              import("@/pages/work/engines-page").then((m) => ({
                Component: m.EnginesPage,
              }))
            ),
            page({ path: "/compare" }, () =>
              import("@/pages/work/compare-page").then((m) => ({
                Component: m.ComparePage,
              }))
            ),
            page({ path: "/compare/:id" }, () =>
              import("@/pages/work/lineup-page").then((m) => ({
                Component: m.LineupPage,
              }))
            ),
          ],
        },
        page({ path: "/login" }, () =>
          import("@/pages/sign-in/sign-in-page").then((m) => ({
            Component: m.SignInPage,
          }))
        ),
        {
          path: "/auth/desktop/handoff",
          lazy: () =>
            import("@/pages/sign-in/handoff-page").then((m) => ({
              Component: m.HandoffPage,
            })),
        },
        {
          id: "admin",
          path: "/admin",
          Component: ConsoleShell,
          loader: readAhead(store, () => "/app/api/console"),
          children: [
            page({ index: true }, () =>
              import("@/pages/admin/overview").then((m) => ({
                Component: m.OverviewPage,
              }))
            ),
            page({ path: "spending" }, () =>
              import("@/pages/admin/spending").then((m) => ({
                Component: m.SpendingPage,
              }))
            ),
            page({ path: "usage" }, () =>
              import("@/pages/admin/usage").then((m) => ({
                Component: m.UsagePage,
              }))
            ),
            page({ path: "usage/:session" }, () =>
              import("@/pages/admin/usage-session").then((m) => ({
                Component: m.UsageSessionPage,
              }))
            ),
            page({ path: "people" }, () =>
              import("@/pages/admin/people").then((m) => ({
                Component: m.PeoplePage,
              }))
            ),
            page({ path: "budgets" }, () =>
              import("@/pages/admin/budgets").then((m) => ({
                Component: m.BudgetsPage,
              }))
            ),
            page({ path: "tokens" }, () =>
              import("@/pages/admin/tokens").then((m) => ({
                Component: m.TokensPage,
              }))
            ),
            page({ path: "records" }, () =>
              import("@/pages/admin/records/records-page").then((m) => ({
                Component: m.RecordsPage,
              }))
            ),
            page({ path: "models" }, () =>
              import("@/pages/admin/models/models-page").then((m) => ({
                Component: m.ModelsPage,
              }))
            ),
            page({ path: "apps" }, () =>
              import("@/pages/admin/apps/apps-page").then((m) => ({
                Component: m.AppsPage,
              }))
            ),
            page({ path: "skill-sets/:name" }, () =>
              import("@/pages/admin/apps/skill-set-page").then((m) => ({
                Component: m.SkillSetPage,
              }))
            ),
            page({ path: "integrations/:name" }, () =>
              import("@/pages/admin/apps/integration-page").then((m) => ({
                Component: m.IntegrationPage,
              }))
            ),
          ],
        },
      ],
    },
  ]
}

// Nothing is what is drawn while the first page's code comes, which is quick enough that a blank page reads better than a flash.
function Nothing() {
  return null
}
