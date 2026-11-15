// What the end-to-end tests and the server they drive agree on: where each part listens, the shared password, and the people
// who sign in.

export const gustoPort = 8840
export const identityPort = 8841
export const modelPort = 8842
// The way out of a sandbox: gusto's proxy, which carries an agent's model calls.
export const proxyPort = 8843

export const gustoURL = `http://127.0.0.1:${gustoPort}`
export const identityURL = `http://127.0.0.1:${identityPort}`

export const sharedPassword = "e2e-shared-password"

// The identity provider knows gusto by this client.
export const client = { id: "gusto-e2e", secret: "e2e-client-secret" }

// The organization's members: one admin and one who runs agents.
export const admin = { email: "ada@example.com", role: "admin" }
export const runner = { email: "grace@example.com", role: "runner" }

// Where each person's signed-in browser state is kept between the setup project and the tests.
export const stateOf = (person) => `e2e/.auth/${person.email}.json`
