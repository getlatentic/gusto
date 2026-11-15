import { createApi, type BaseQueryFn } from "@reduxjs/toolkit/query/react"

import { Failure, getJSON } from "@/lib/http"
import { askCloud } from "@/lib/cloud"

// ApiError is why something could not be read: the server's status, and what it said.
export interface ApiError {
  status: number
  message: string
}

const asError = (err: unknown): ApiError => ({
  status: err instanceof Failure ? err.status : 0,
  message: err instanceof Error ? err.message : String(err),
})

// appQuery reads a page's data at an address of this server.
const appQuery: BaseQueryFn<string, unknown, ApiError> = async (
  path,
  { signal }
) => {
  try {
    return { data: await getJSON(path, signal) }
  } catch (err) {
    return { error: asError(err) }
  }
}

// api is everything the app reads from the server, kept by address. A page's data is tagged with its address, so a page that
// changed something marks its own data, or every page's, as stale. The room's live turns are added to it in room-live.ts.
export const api = createApi({
  reducerPath: "api",
  baseQuery: appQuery,
  tagTypes: ["Page"],
  // A page's data is read again when the person comes back to the window, as they expect after working elsewhere.
  refetchOnFocus: true,
  endpoints: (build) => ({
    page: build.query<unknown, string>({
      query: (path) => path,
      providesTags: (_data, _error, path) => [{ type: "Page", id: path }],
    }),
    // cloud is what gusto cloud knows of the person, read with their gusto cloud session.
    cloud: build.query<unknown, string>({
      queryFn: async (address, { signal }) => {
        try {
          return { data: await askCloud(address, signal) }
        } catch (err) {
          return { error: asError(err) }
        }
      },
    }),
  }),
})
