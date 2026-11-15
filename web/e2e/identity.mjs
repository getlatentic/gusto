// An OpenID Connect provider for the end-to-end tests. The shared password signs everybody in as one nameless admin, so the
// tests sign people in as themselves through this provider, the way a deployment with its own provider does. Its sign-in page
// asks only for an email address; everything after that is the real exchange: a code for the state gusto sent, PKCE, and an
// RS256 identity token carrying gusto's nonce, which gusto checks against the keys published here.
import { createHash, generateKeyPairSync, randomBytes, sign } from "node:crypto"
import { createServer } from "node:http"

// A code is exchanged within this long, once.
const codeLife = 60_000
// Codes not yet exchanged are capped, so a run that never finishes its sign-ins cannot grow the map without end.
const mostCodes = 1_000

const base64url = (data) => Buffer.from(data).toString("base64url")

const escapeHTML = (text) =>
  String(text).replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ]
  )

function readBody(req) {
  return new Promise((resolve, reject) => {
    let body = ""
    req.setEncoding("utf8")
    req.on("data", (chunk) => {
      body += chunk
      if (body.length > 64 * 1024) reject(new Error("too large"))
    })
    req.on("end", () => resolve(new URLSearchParams(body)))
    req.on("error", reject)
  })
}

function answer(res, status, type, body, headers = {}) {
  res.writeHead(status, { "content-type": type, ...headers })
  res.end(body)
}

const json = (res, status, value) =>
  answer(res, status, "application/json", JSON.stringify(value), {
    "cache-control": "no-store",
  })

// signInPage is what a person sees at the provider: their address, and the request gusto made carried along unchanged.
function signInPage(query) {
  const carried = ["redirect_uri", "state", "nonce", "code_challenge"]
    .map(
      (name) =>
        `<input type="hidden" name="${name}" value="${escapeHTML(query.get(name) ?? "")}">`
    )
    .join("")
  return `<!doctype html><meta charset="utf-8"><title>Sign in</title>
<form method="post" action="/authorize">${carried}
<label>Email <input data-testid="provider-email" type="email" name="email" required autofocus></label>
<button data-testid="provider-continue" type="submit">Continue</button>
</form>`
}

// clientOf reads the client's id and secret from the Basic header, or from the form when the client sends them there.
function clientOf(req, form) {
  const header = req.headers.authorization ?? ""
  if (header.startsWith("Basic ")) {
    const [id, secret] = Buffer.from(header.slice(6), "base64")
      .toString()
      .split(":")
      .map(decodeURIComponent)
    return { id, secret }
  }
  return { id: form.get("client_id"), secret: form.get("client_secret") }
}

export function startIdentity({ port, client, redirect }) {
  const issuer = `http://127.0.0.1:${port}`
  const { privateKey, publicKey } = generateKeyPairSync("rsa", {
    modulusLength: 2048,
  })
  const kid = randomBytes(8).toString("hex")
  const jwk = {
    ...publicKey.export({ format: "jwk" }),
    kid,
    alg: "RS256",
    use: "sig",
  }
  const codes = new Map()

  const idToken = (grant) => {
    const now = Math.floor(Date.now() / 1000)
    const head = base64url(JSON.stringify({ alg: "RS256", typ: "JWT", kid }))
    const body = base64url(
      JSON.stringify({
        iss: issuer,
        sub: grant.email,
        aud: client.id,
        iat: now,
        exp: now + 600,
        nonce: grant.nonce,
        email: grant.email,
        email_verified: true,
      })
    )
    const signature = sign("sha256", Buffer.from(`${head}.${body}`), privateKey)
    return `${head}.${body}.${base64url(signature)}`
  }

  const authorize = async (req, res) => {
    const form = await readBody(req)
    if (form.get("redirect_uri") !== redirect) {
      return answer(
        res,
        400,
        "text/plain",
        "that redirect is not this client's"
      )
    }
    if (codes.size >= mostCodes) codes.delete(codes.keys().next().value)
    const code = randomBytes(24).toString("base64url")
    codes.set(code, {
      email: form.get("email"),
      nonce: form.get("nonce"),
      challenge: form.get("code_challenge"),
      until: Date.now() + codeLife,
    })
    const back = new URL(redirect)
    back.searchParams.set("code", code)
    back.searchParams.set("state", form.get("state") ?? "")
    answer(res, 303, "text/plain", "", { location: back.toString() })
  }

  const token = async (req, res) => {
    const form = await readBody(req)
    const asking = clientOf(req, form)
    if (asking.id !== client.id || asking.secret !== client.secret) {
      return json(res, 401, { error: "invalid_client" })
    }
    const grant = codes.get(form.get("code") ?? "")
    codes.delete(form.get("code") ?? "")
    const verifier = form.get("code_verifier") ?? ""
    const challenge = createHash("sha256").update(verifier).digest("base64url")
    if (!grant || grant.until < Date.now() || grant.challenge !== challenge) {
      return json(res, 400, { error: "invalid_grant" })
    }
    json(res, 200, {
      access_token: randomBytes(24).toString("base64url"),
      token_type: "Bearer",
      expires_in: 600,
      id_token: idToken(grant),
    })
  }

  const server = createServer((req, res) => {
    const url = new URL(req.url ?? "/", issuer)
    const route = `${req.method} ${url.pathname}`
    const fail = (err) => answer(res, 400, "text/plain", String(err))
    switch (route) {
      case "GET /.well-known/openid-configuration":
        return json(res, 200, {
          issuer,
          authorization_endpoint: `${issuer}/authorize`,
          token_endpoint: `${issuer}/token`,
          jwks_uri: `${issuer}/jwks`,
          response_types_supported: ["code"],
          subject_types_supported: ["public"],
          id_token_signing_alg_values_supported: ["RS256"],
          code_challenge_methods_supported: ["S256"],
        })
      case "GET /jwks":
        return json(res, 200, { keys: [jwk] })
      case "GET /authorize":
        if (url.searchParams.get("client_id") !== client.id) {
          return answer(res, 400, "text/plain", "that client is not known here")
        }
        return answer(
          res,
          200,
          "text/html; charset=utf-8",
          signInPage(url.searchParams)
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
      resolve({ issuer, close: () => server.close() })
    )
  })
}
