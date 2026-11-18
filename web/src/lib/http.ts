// Asking this server for a page's data, and sending it a form whose answer the page reads itself.

import { withBase } from "@/lib/base"

export class Failure extends Error {
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

export async function getJSON<T>(
  path: string,
  signal?: AbortSignal
): Promise<T> {
  const answer = await fetch(withBase(path), {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
    signal,
  })
  // A session that ended answers 401. Loading the page again takes the person to sign in, and back to this page after.
  if (answer.status === 401) {
    window.location.reload()
    throw new Failure(401, "signed out")
  }
  if (!answer.ok) {
    throw new Failure(
      answer.status,
      (await answer.text()).trim() || answer.statusText
    )
  }
  return (await answer.json()) as T
}

// postForm sends a form to the server as the browser would, for a page that reads the answer itself, and gives the answer
// as JSON, or the server's reason when it refused.
export async function postForm<T>(path: string, form: FormData): Promise<T> {
  const fields = new URLSearchParams()
  for (const [key, value] of form) {
    if (typeof value === "string") fields.append(key, value)
  }
  const answer = await fetch(withBase(path), {
    method: "POST",
    credentials: "same-origin",
    headers: { Accept: "application/json" },
    body: fields,
  })
  if (!answer.ok) {
    throw new Failure(
      answer.status,
      (await answer.text()).trim() || answer.statusText
    )
  }
  return (await answer.json()) as T
}
