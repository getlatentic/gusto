import { useEffect, useRef } from "react"

import { Quiet, Refusal, TextLink } from "@/cloud/components/cloud-card"
import { Field, Submit } from "@/cloud/components/forms"
import { PageIntro, SitePage } from "@/cloud/pages/site-page"
import type { CloudPage } from "@/cloud/types"
import { demoFormWords } from "@/domain/demo-form"

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
  const words = demoFormWords(page)
  return (
    <SitePage page={page}>
      <PageIntro title={words.title}>{words.intro}</PageIntro>
      <section
        aria-label="Start a demo"
        className="mx-auto grid max-w-[26rem] gap-4 px-5 pb-14"
      >
        <Refusal>{page.Error}</Refusal>
        <form
          className="grid gap-3"
          method="post"
          action="/demo"
          onInput={warm}
        >
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
            label={words.email}
            type="email"
            name="email"
            defaultValue={page.Email}
            autoComplete="email"
            required
            data-testid="demo-email"
          />
          <Field
            label={words.company}
            type="text"
            name="company"
            defaultValue={page.OrgName}
            maxLength={80}
            autoComplete="organization"
            required
            data-testid="demo-company"
          />
          {page.TurnstileSite && <BotCheck site={page.TurnstileSite} />}
          <Submit data-testid="start-demo">{words.submit}</Submit>
        </form>
        {words.offersCall && page.Book && (
          <Quiet className="text-center">
            Rather be shown? <TextLink href={page.Book}>Book a call</TextLink>
          </Quiet>
        )}
      </section>
    </SitePage>
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
