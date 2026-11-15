import { configureStore } from "@reduxjs/toolkit"
import { setupListeners } from "@reduxjs/toolkit/query"
import { useDispatch, useSelector } from "react-redux"

import { api } from "@/store/api"
import { rooms } from "@/store/rooms"

// makeStore is the app's one store: what it reads from the server, and the live state of the rooms open. A test makes its own.
export function makeStore() {
  const store = configureStore({
    reducer: { [api.reducerPath]: api.reducer, rooms: rooms.reducer },
    middleware: (defaults) => defaults().concat(api.middleware),
  })
  setupListeners(store.dispatch)
  return store
}

export type AppStore = ReturnType<typeof makeStore>
export type RootState = ReturnType<AppStore["getState"]>
export type AppDispatch = AppStore["dispatch"]

export const useAppDispatch = useDispatch.withTypes<AppDispatch>()
export const useAppSelector = useSelector.withTypes<RootState>()
