// What the end-to-end tests and the server they drive agree on: where each part listens, the shared password, and the people
// who sign in.

// The seven ports in a row from E2E_PORT_BASE, 8840 when it is not set, so two runs on one machine can each have their own.
const base = Number(process.env.E2E_PORT_BASE || 8840)
export const gustoPort = base
export const identityPort = base + 1
export const modelPort = base + 2
// The way out of a sandbox: gusto's proxy, which carries an agent's model calls.
export const proxyPort = base + 3
// An app a person connects from Connections, which signs them in by OAuth.
export const appPort = base + 4
// gusto cloud, which signs organizations up and starts their servers, and the page its operator manages it from.
export const cloudPort = base + 5
export const operatorPort = base + 6

export const gustoURL = `http://127.0.0.1:${gustoPort}`
export const identityURL = `http://127.0.0.1:${identityPort}`
// gusto cloud tells organizations apart by host name, so it is reached as localhost, the domain they are served under.
export const cloudURL = `http://localhost:${cloudPort}`
export const operatorURL = `http://127.0.0.1:${operatorPort}`
// Where gusto cloud writes the mail it sends, one file each, for a test to read a code or a link from.
export const cloudOutbox = "e2e/.run/cloud/outbox"
export const bookURL = "https://cal.example/gusto"

export const sharedPassword = "e2e-shared-password"

// The identity provider knows gusto by this client.
export const client = { id: "gusto-e2e", secret: "e2e-client-secret" }

// The organization's members: one admin and one who runs agents.
export const admin = { email: "ada@example.com", role: "admin" }
export const runner = { email: "grace@example.com", role: "runner" }

// Where each person's signed-in browser state is kept between the setup project and the tests.
export const stateOf = (person) => `e2e/.auth/${person.email}.json`

// The app the organization offers on Connections, by its id and name.
export const connectedApp = { id: "tasks", name: "Tasks" }
