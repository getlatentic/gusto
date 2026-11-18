import { useEffect, useRef, useState } from "react"

import { BotCheck } from "@/cloud/components/bot-check"
import { Refusal, TextLink } from "@/cloud/components/cloud-card"
import { Field, Submit, TextField } from "@/cloud/components/forms"
import { Band, BandText, BandTitle } from "@/cloud/pages/landing/landing-parts"
import type { CloudContact, CloudPage } from "@/cloud/types"

// Contact is a note to the team: a work address, a company and what the visitor wants to talk about. The team answers by email.
export function Contact({ page }: { page: CloudPage }) {
  const contact = page.Contact
  const answered = useAnswerInView(
    !!contact && (contact.Sent || contact.Error !== "")
  )
  if (!contact) return null
  return (
    <Band id="contact" ref={answered} className="scroll-mt-18">
      <div className="grid gap-8 min-[52rem]:grid-cols-[minmax(0,1fr)_minmax(0,26rem)] min-[52rem]:gap-16">
        <div className="grid content-start gap-3">
          <BandTitle>Talk to us</BandTitle>
          <BandText>
            Tell us about your team and the work you want agents to do. We write
            back by email.
          </BandText>
          {page.Book && (
            <BandText>
              Rather talk now? <TextLink href={page.Book}>Book a call</TextLink>
            </BandText>
          )}
        </div>
        {contact.Sent ? (
          <p
            className="m-0 self-center text-sm"
            role="status"
            data-testid="contact-sent"
          >
            Thank you. Your note is with the gusto team, and we will write back
            by email.
          </p>
        ) : (
          <ContactForm contact={contact} site={page.TurnstileSite} />
        )}
      </div>
    </Band>
  )
}

// useAnswerInView brings the section into view when it answers a note: the page is drawn after the browser looked for #contact,
// and a refused note is answered at /contact, so neither would be seen at the top of the page.
function useAnswerInView(answered: boolean) {
  const section = useRef<HTMLElement>(null)
  useEffect(() => {
    if (answered) section.current?.scrollIntoView({ block: "start" })
  }, [answered])
  return section
}

// ContactForm loads the bot check only once the visitor starts typing, so a visit that sends no note asks nothing of Cloudflare.
// A note that was refused has been typed already, so it has the check at once.
function ContactForm({
  contact,
  site,
}: {
  contact: CloudContact
  site: string
}) {
  const [typing, setTyping] = useState(contact.Error !== "")
  return (
    <form
      className="grid gap-3"
      method="post"
      action="/contact"
      onInput={() => setTyping(true)}
      data-testid="contact-form"
    >
      <Refusal>{contact.Error}</Refusal>
      <Field
        label="Work email"
        type="email"
        name="email"
        defaultValue={contact.Email}
        autoComplete="email"
        required
        data-testid="contact-email"
      />
      <Field
        label="Company"
        type="text"
        name="company"
        defaultValue={contact.Company}
        maxLength={80}
        autoComplete="organization"
        required
        data-testid="contact-company"
      />
      <TextField
        label="What would you like to talk about?"
        name="note"
        defaultValue={contact.Note}
        maxLength={2000}
        required
        data-testid="contact-note"
      />
      {site && typing && <BotCheck site={site} />}
      <Submit data-testid="contact-send">Send</Submit>
    </form>
  )
}
