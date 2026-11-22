import { BotCheck } from "@/cloud/components/bot-check"
import { Quiet, Refusal, TextLink } from "@/cloud/components/cloud-card"
import { Field, Submit } from "@/cloud/components/forms"
import { PageIntro, SitePage } from "@/cloud/pages/site-page"
import type { CloudPage } from "@/cloud/types"
import { demoFormWords } from "@/domain/demo-form"

// DemoFormPage starts a demo.
export function DemoFormPage({ page }: { page: CloudPage }) {
  const words = demoFormWords(page)
  return (
    <SitePage page={page}>
      <PageIntro title={words.title}>{words.intro}</PageIntro>
      <section
        aria-label="Start a demo"
        className="mx-auto grid max-w-[26rem] gap-4 px-5 pb-14"
      >
        <Refusal>{page.Error}</Refusal>
        <form className="grid gap-3" method="post" action="/demo">
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
