import { useState } from "react"
import { ShieldAlertIcon, ShieldCheckIcon } from "lucide-react"

import { Button, buttonVariants } from "@/components/ui/button"
import { Disclosure } from "@/pages/work/disclosure"
import { SectionCard } from "@/pages/work/section-card"
import type { Proof, TaskData } from "@/pages/work/types"
import { withBase } from "@/lib/base"

// TaskProof is what the page found checking the record the way `gusto verify` does, and how to check it again without this
// server. An admin is shown it; anybody is shown a record that does not check out, open.
export function TaskProof({ page }: { page: TaskData }) {
  const proof = page.Proof
  if (!proof || (!page.Admin && proof.Holds)) return null
  const files = (page.Files ?? []).filter((file) => file.SHA256)
  const fileAt = `/jobs/${encodeURIComponent(page.Job.ID)}/files/`
  return (
    <SectionCard id="proof" testId="task-proof">
      <Disclosure
        open={!proof.Holds}
        summary={
          <>
            Verify this run
            {!proof.Holds && (
              <span className="text-bad">(does not check out)</span>
            )}
          </>
        }
      >
        <ProofSays proof={proof} />
        <p className="m-0 mt-3 mb-3 text-xs text-muted-foreground">
          Check it on your own machine, without gusto.
        </p>
        <p className="m-0 mb-3 flex flex-wrap gap-2">
          <a
            className={buttonVariants({ size: "sm" })}
            href={withBase(proof.Bundle)}
            download
          >
            The record
          </a>
          {proof.Key && (
            <a
              className={buttonVariants({ variant: "outline", size: "sm" })}
              href={withBase(proof.Key)}
              download
            >
              The signing key
            </a>
          )}
        </p>
        <VerifyCommand command={proof.Command} />
        <Disclosure summary="Each file and its fingerprint">
          <ul className="m-0 grid list-none gap-2 p-0 text-sm">
            {files.map((file) => (
              <li key={file.Name} className="grid gap-0.5">
                <a
                  className="no-underline underline-offset-4 hover:underline"
                  href={withBase(fileAt + encodeURIComponent(file.Name))}
                >
                  {file.Name}
                </a>
                <code className="text-xs wrap-anywhere text-subtle">
                  {file.SHA256}
                </code>
              </li>
            ))}
          </ul>
        </Disclosure>
      </Disclosure>
    </SectionCard>
  )
}

const seal =
  "flex items-start gap-2 font-medium [&>svg]:mt-0.5 [&>svg]:size-[1.125rem] [&>svg]:flex-none"

function ProofSays({ proof }: { proof: Proof }) {
  if (!proof.Holds) {
    return (
      <>
        <p className={`m-0 mb-2 text-bad ${seal}`}>
          <ShieldAlertIcon aria-hidden="true" /> This record does not check out.
        </p>
        <ul className="m-0 mt-3 grid list-none gap-2 p-0 text-sm">
          {(proof.Wrong ?? []).map((wrong) => (
            <li key={wrong}>{wrong}</li>
          ))}
        </ul>
      </>
    )
  }
  if (proof.Unchecked) {
    return (
      <p className="m-0 mb-2 font-medium">
        All {proof.Files} files match; {proof.Unchecked}.
      </p>
    )
  }
  if (proof.Signed) {
    return (
      <p className={`m-0 mb-2 text-ok ${seal}`}>
        <ShieldCheckIcon aria-hidden="true" /> Verified: signed by{" "}
        {proof.Signer}, and all {proof.Files} files match.
      </p>
    )
  }
  return (
    <p className="m-0 mb-2 font-medium">
      All {proof.Files} files match. Not signed: this server had no signing key
      when it ran.
    </p>
  )
}

// VerifyCommand is the command that checks the record, and a button that copies it.
function VerifyCommand({ command }: { command: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="mb-3 flex items-center gap-2 rounded-sm bg-muted py-2 pr-2 pl-3">
      <code className="min-w-0 flex-1 text-xs wrap-anywhere" id="verify">
        {command}
      </code>
      <Button
        type="button"
        variant="outline"
        size="xs"
        onClick={() =>
          void navigator.clipboard
            .writeText(command)
            .then(() => setCopied(true))
        }
      >
        {copied ? "Copied" : "Copy"}
      </Button>
    </div>
  )
}
