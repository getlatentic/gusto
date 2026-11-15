import { readdirSync, readFileSync } from "node:fs"
import { join } from "node:path"

import { expect } from "@playwright/test"

import { cloudOutbox } from "./values.mjs"

// mailTo is the newest mail gusto cloud wrote to an address whose subject matches, waiting for it as a person waits for mail.
export async function mailTo(
  address: string,
  subject: RegExp
): Promise<string> {
  let found = ""
  await expect
    .poll(() => {
      const files = readdirSync(cloudOutbox).sort().reverse()
      for (const file of files) {
        const text = readFileSync(join(cloudOutbox, file), "utf8")
        const headers = text.slice(0, text.indexOf("\n\n"))
        if (headers.includes(`To: ${address}`) && subject.test(headers)) {
          found = text
          return true
        }
      }
      return false
    })
    .toBe(true)
  return found
}

// The code a sign-in mail carries, and the link a confirmation mail carries.
export const codeIn = (mail: string) =>
  /sign-in code is (\d{6})/.exec(mail)?.[1] ?? ""
export const linkIn = (mail: string) =>
  /(http:\/\/\S+\?t=\S+)/.exec(mail)?.[1] ?? ""
