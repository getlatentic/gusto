import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { Provider } from "react-redux"
import { createBrowserRouter } from "react-router"
import { RouterProvider } from "react-router/dom"

import "./index.css"

import { TooltipProvider } from "@/components/ui/tooltip"
import { basePath } from "@/lib/base"
import { appRoutes } from "@/router/app-routes"
import { makeStore } from "@/store"

const store = makeStore()
const router = createBrowserRouter(appRoutes(store), {
  basename: basePath() || undefined,
})

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <Provider store={store}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </Provider>
  </StrictMode>
)
