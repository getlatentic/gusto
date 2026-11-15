import { StrictMode } from "react"
import { createRoot } from "react-dom/client"

import "./cloud.css"

import { CloudPageView } from "@/cloud/cloud-page"
import { readPage } from "@/cloud/page-data"

// gusto cloud's pages: the server chooses which page to answer with and writes its data into it, so this draws that page once,
// and every link and form is an ordinary request to the server.
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <CloudPageView page={readPage()} />
  </StrictMode>
)
