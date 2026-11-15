// An app for the end-to-end tests that a person connects from Connections: an MCP server that asks for OAuth, as MCP's
// authorization says, with its own authorization server. Gusto finds how to sign in from the 401 its MCP server answers, registers
// itself as a client, and sends the person to its sign-in page, where they allow it; the app then sends them back with a code,
// which gusto trades, with its PKCE verifier, for tokens.
import { createHash, randomBytes } from "node:crypto"
import { createServer } from "node:http"

import { answer, escapeHTML, readBody } from "./identity.mjs"

// A code is traded within this long, once, and codes not yet traded are capped.
const codeLife = 60_000
const mostCodes = 1_000

const json = (res, status, value) =>
  answer(res, status, "application/json", JSON.stringify(value), {
    "cache-control": "no-store",
  })

// allowPage is the app's own page, where the person lets gusto use their account; what gusto asked for is carried along.
function allowPage(query) {
  const carried = ["redirect_uri", "state", "code_challenge", "resource"]
    .map(
      (name) =>
        `<input type="hidden" name="${name}" value="${escapeHTML(query.get(name) ?? "")}">`
    )
    .join("")
  return `<!doctype html><meta charset="utf-8"><title>Allow gusto</title>
<form method="post" action="/authorize">${carried}
<p>gusto asks to use your tasks.</p>
<button data-testid="app-allow" type="submit">Allow</button>
</form>`
}

export function startApp({ port }) {
  const base = `http://127.0.0.1:${port}`
  const mcp = `${base}/mcp`
  const codes = new Map()

  const authorize = async (req, res) => {
    const form = await readBody(req)
    if (form.get("resource") !== mcp) {
      return answer(res, 400, "text/plain", "that is not this app's resource")
    }
    if (codes.size >= mostCodes) codes.delete(codes.keys().next().value)
    const code = randomBytes(24).toString("base64url")
    codes.set(code, {
      challenge: form.get("code_challenge"),
      until: Date.now() + codeLife,
    })
    const back = new URL(form.get("redirect_uri") ?? "")
    back.searchParams.set("code", code)
    back.searchParams.set("state", form.get("state") ?? "")
    answer(res, 303, "text/plain", "", { location: back.toString() })
  }

  const token = async (req, res) => {
    const form = await readBody(req)
    const grant = codes.get(form.get("code") ?? "")
    codes.delete(form.get("code") ?? "")
    const challenge = createHash("sha256")
      .update(form.get("code_verifier") ?? "")
      .digest("base64url")
    if (
      form.get("grant_type") !== "authorization_code" ||
      !grant ||
      grant.until < Date.now() ||
      grant.challenge !== challenge ||
      form.get("resource") !== mcp
    ) {
      return json(res, 400, { error: "invalid_grant" })
    }
    json(res, 200, {
      access_token: randomBytes(24).toString("base64url"),
      refresh_token: randomBytes(24).toString("base64url"),
      token_type: "Bearer",
      expires_in: 3600,
    })
  }

  const server = createServer((req, res) => {
    const url = new URL(req.url ?? "/", base)
    const fail = (err) => answer(res, 400, "text/plain", String(err))
    switch (`${req.method} ${url.pathname}`) {
      case "POST /mcp":
        return answer(res, 401, "text/plain", "sign in first", {
          "www-authenticate": `Bearer resource_metadata="${base}/.well-known/oauth-protected-resource/mcp", scope="tasks"`,
        })
      case "GET /.well-known/oauth-protected-resource/mcp":
        return json(res, 200, { resource: mcp, authorization_servers: [base] })
      case "GET /.well-known/oauth-authorization-server":
        return json(res, 200, {
          issuer: base,
          authorization_endpoint: `${base}/authorize`,
          token_endpoint: `${base}/token`,
          registration_endpoint: `${base}/register`,
          code_challenge_methods_supported: ["S256"],
        })
      case "POST /register":
        return json(res, 201, { client_id: "gusto-e2e-app-client" })
      case "GET /authorize":
        return answer(
          res,
          200,
          "text/html; charset=utf-8",
          allowPage(url.searchParams)
        )
      case "POST /authorize":
        return void authorize(req, res).catch(fail)
      case "POST /token":
        return void token(req, res).catch(fail)
      default:
        return answer(res, 404, "text/plain", "not found")
    }
  })
  return new Promise((resolve, reject) => {
    server.once("error", reject)
    server.listen(port, "127.0.0.1", () =>
      resolve({ mcp, close: () => server.close() })
    )
  })
}
