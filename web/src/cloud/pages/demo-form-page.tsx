import { useEffect, useRef } from "react"

import {
  Brand,
  CloudCard,
  Heading,
  Quiet,
  Refusal,
  TextLink,
} from "@/cloud/components/cloud-card"
import { Field, Submit } from "@/cloud/components/forms"
import type { CloudPage } from "@/cloud/types"

// DemoFormPage starts a demo. A demo's cell takes most of a minute to start, so it is asked for when the visitor first types in the
// form, and is up or nearly by the time they send it.
export function DemoFormPage({ page }: { page: CloudPage }) {
  const warmed = useRef(false)
  const warm = () => {
    if (warmed.current) return
    warmed.current = true
    fetch("/demo/warm", { method: "POST", credentials: "same-origin" }).catch(
      () => {}
    )
  }
  return (
    <CloudCard>
      <Brand />
      <Heading>Try gusto free for {page.DemoLeft}</Heading>
      <Quiet>
        Try gusto, an AI agent cloud for your organization. Run ready-to-use
        agents like Claude Code and Codex, and see how gusto lets companies give
        every employee a private cloud computer where their agents work, with
        built-in controls for models, spending, and security.
      </Quiet>
      <Refusal>{page.Error}</Refusal>
      <form className="grid gap-3" method="post" action="/demo" onInput={warm}>
        <Field
          label="Your name"
          type="text"
          name="name"
          defaultValue={page.Name}
          maxLength={80}
          autoComplete="name"
          autoFocus
          required
          data-testid="demo-name"
        />
        <Field
          label={page.WorkOnly ? "Work email" : "Email"}
          type="email"
          name="email"
          defaultValue={page.Email}
          autoComplete="email"
          required
          data-testid="demo-email"
        />
        <Field
          label="Company"
          type="text"
          name="company"
          defaultValue={page.OrgName}
          maxLength={80}
          autoComplete="organization"
          required
          data-testid="demo-company"
        />
        {page.TurnstileSite && <BotCheck site={page.TurnstileSite} />}
        <Submit data-testid="start-demo">Start my demo</Submit>
      </form>
      {page.Book && (
        <Quiet className="text-center">
          Rather be shown? <TextLink href={page.Book}>Book a call</TextLink>
        </Quiet>
      )}
    </CloudCard>
  )
}

const turnstileScript = "https://challenges.cloudflare.com/turnstile/v0/api.js"

// BotCheck is Cloudflare's check that a person is sending the form. Its script draws the check into the box it finds, so the
// script is added once the box is on the page.
function BotCheck({ site }: { site: string }) {
  useEffect(() => {
    if (document.querySelector(`script[src="${turnstileScript}"]`)) return
    const script = document.createElement("script")
    script.src = turnstileScript
    script.async = true
    document.head.append(script)
  }, [])
  return (
    <div className="cf-turnstile" data-size="flexible" data-sitekey={site} />
  )
}
