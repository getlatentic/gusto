import type { RouteObject } from "react-router"

import { listSearch } from "@/domain/members"
import { pagePath } from "@/lib/api"
import { readAhead } from "@/router/read-ahead"
import type { AppStore } from "@/store"

// membersRoute is the members pages: the list is their layout, read once, and the page beside it is nobody, an agent or a person,
// each read at its own address.
export function membersRoute(store: AppStore): RouteObject {
  return {
    id: "members",
    path: "/members",
    lazy: () =>
      import("@/pages/members/members-layout").then((m) => ({
        Component: m.MembersLayout,
      })),
    loader: readAhead(store, (url) =>
      pagePath({ pathname: "/members", search: listSearch(url) })
    ),
    children: [
      {
        index: true,
        lazy: () =>
          import("@/pages/members/members-index").then((m) => ({
            Component: m.MembersIndex,
          })),
      },
      {
        path: "agents/:id",
        lazy: () =>
          import("@/pages/members/agent-view").then((m) => ({
            Component: m.OpenAgent,
          })),
        loader: readAhead(store),
      },
      {
        path: "people/:who",
        lazy: () =>
          import("@/pages/members/person-view").then((m) => ({
            Component: m.OpenPerson,
          })),
        loader: readAhead(store),
      },
    ],
  }
}
