import { below } from "@/lib/base"
import { Failure } from "@/lib/http"

// sendForm sends a form as the browser would, without leaving the page, and gives the address the server sent the person on to
// (its path below the base path, and query), which is where any refusal is said. The page then moves there in the app instead of loading it again.
// files are sent as the form's "file" field, for a form whose files are kept by the page rather than by its input.
export async function sendForm(
  form: HTMLFormElement,
  files: File[] = []
): Promise<string> {
  const multipart = form.enctype === "multipart/form-data"
  const fields = new FormData(form)
  if (files.length) {
    fields.delete("file")
    for (const file of files) fields.append("file", file)
  }
  const body = multipart
    ? fields
    : new URLSearchParams(fields as unknown as Record<string, string>)
  const answer = await fetch(form.action, {
    method: "POST",
    credentials: "same-origin",
    body,
  })
  if (!answer.ok) {
    throw new Failure(
      answer.status,
      (await answer.text()).trim() || answer.statusText
    )
  }
  const landed = new URL(answer.url)
  return landed.origin === location.origin
    ? below(landed.pathname) + landed.search + landed.hash
    : ""
}
