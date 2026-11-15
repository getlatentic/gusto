import { useLocation } from "react-router"

import { api, type ApiError } from "@/store/api"
import { useAppDispatch } from "@/store"

// A page's data is the same view the server gave its template, as JSON at /app/api followed by the page's own address. The
// server keeps the query in the links it gives, so changing a filter is changing the address, and the address alone says what
// is shown.

export type Loaded<T> =
  | { state: "loading"; data?: T }
  | { state: "ready"; data: T }
  | { state: "failed"; status: number; message: string }

export interface Reading {
  // pollingInterval reads the data again every so many milliseconds while the page is in view.
  pollingInterval?: number
  // skip leaves the data unread, as for a part of a page that is not shown.
  skip?: boolean
}

// useData is the data at a path, read again when the path changes or the data is marked stale; what was shown stays on screen
// while the next one loads.
export function useData<T>(path: string, reading: Reading = {}): Loaded<T> {
  const read = api.endpoints.page.useQuery(path, {
    ...reading,
    skipPollingIfUnfocused: true,
  })
  if (read.isError) {
    const error = read.error as ApiError
    return { state: "failed", status: error.status, message: error.message }
  }
  if (read.currentData !== undefined)
    return { state: "ready", data: read.currentData as T }
  return { state: "loading", data: read.data as T | undefined }
}

// pagePath is where the data of the page the address names is.
export function pagePath(where: { pathname: string; search: string }): string {
  return "/app/api" + where.pathname + where.search
}

// usePageData is the data of the page the address names.
export function usePageData<T>(reading: Reading = {}): Loaded<T> {
  return useData<T>(pagePath(useLocation()), reading)
}

// useRefreshPage marks the page's data stale, so it is read again, as after the page changed something the server keeps.
export function useRefreshPage(): () => void {
  const dispatch = useAppDispatch()
  const path = pagePath(useLocation())
  return () => dispatch(api.util.invalidateTags([{ type: "Page", id: path }]))
}

// useRefreshAll marks every page's data stale, as after a form whose change may show anywhere.
export function useRefreshAll(): () => void {
  const dispatch = useAppDispatch()
  return () => dispatch(api.util.invalidateTags(["Page"]))
}

// dataOf is what data there is to show: the data read, the data still shown while the next is read, or none.
export function dataOf<T>(loaded: Loaded<T>): T | undefined {
  return loaded.state === "failed" ? undefined : loaded.data
}
