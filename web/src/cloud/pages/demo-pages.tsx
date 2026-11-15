import { buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Brand,
  CloudCard,
  Heading,
  Quiet,
  Refusal,
  TextLink,
  Waiting,
} from "@/cloud/components/cloud-card"
import { Submit } from "@/cloud/components/forms"
import type { CloudPage } from "@/cloud/types"
import { currentStep } from "@/domain/cloud-org"
import { cn } from "@/lib/utils"

const openDemo = (page: CloudPage) =>
  `/open?org=${encodeURIComponent(page.Org?.Slug ?? "")}`

function ButtonLink({
  href,
  children,
  testid,
}: {
  href: string
  children: string
  testid?: string
}) {
  return (
    <a
      className={cn(buttonVariants(), "h-11")}
      href={href}
      data-testid={testid}
    >
      {children}
    </a>
  )
}

// DemoWaitPage is a demo's own page: how far it is from ready while it starts, then the time it has left, the way in and the way
// to keep it ten minutes longer.
export function DemoWaitPage({ page }: { page: CloudPage }) {
  const step = currentStep(page.Steps)
  return (
    <CloudCard>
      <div className="grid justify-items-center gap-5 text-center">
        <Brand />
        <Heading>Your gusto demo</Heading>
        {page.Notice && (
          <p className="m-0 text-sm text-bad" role="status">
            {page.Notice}
          </p>
        )}
        {page.DemoWords ? (
          <>
            <p className="m-0" role="status" data-testid="demo-state">
              {page.DemoWords}
            </p>
            {step && (
              <Waiting aria-current="step" className="font-semibold">
                {step.Label}
              </Waiting>
            )}
          </>
        ) : (
          <>
            <p className="m-0" data-testid="demo-left">
              You have <strong>{page.DemoLeft}</strong> left. Everything in it
              is deleted when it ends.
            </p>
            {page.CanOpen && (
              <ButtonLink href={openDemo(page)} testid="open-demo">
                Open my demo
              </ButtonLink>
            )}
            {page.Extendable && (
              <form method="post" action="/demo/extend">
                <Submit data-testid="extend-demo">
                  Give me 10 more minutes
                </Submit>
              </form>
            )}
          </>
        )}
        {page.Book && (
          <Quiet>
            Want this for your team?{" "}
            <TextLink href={page.Book}>Book a call</TextLink>
          </Quiet>
        )}
      </div>
    </CloudCard>
  )
}

// DemoBookPage is where a demo is booked with the team, where only the team starts them: the booking service's calendar in the page
// where it may be framed, or a link to it.
export function DemoBookPage({ page }: { page: CloudPage }) {
  return (
    <CloudCard wide>
      <Brand />
      <div className="grid gap-2">
        <Heading>Book a demo</Heading>
        <Quiet className="max-w-[40rem]">
          Someone from the gusto team runs it with you on a call: agents of
          several vendors at work in one room, and the controls your
          organization keeps over them.
        </Quiet>
      </div>
      {page.BookFrame ? (
        <iframe
          className="block h-[44rem] w-full rounded-md border bg-card"
          src={page.BookFrame}
          title="Pick a time for a demo of gusto"
          loading="lazy"
          referrerPolicy="strict-origin-when-cross-origin"
        />
      ) : (
        page.Book && (
          <span className="max-w-[26rem]">
            <ButtonLink href={page.Book}>Book a demo</ButtonLink>
          </span>
        )
      )}
      <Quiet>
        Or <TextLink href="/signup">create your organization</TextLink> and try
        gusto now.
        {page.BookFrame && (
          <>
            {" "}
            The calendar not loading?{" "}
            <TextLink href={page.Book}>Open it on its own page</TextLink>.
          </>
        )}
      </Quiet>
      <p className="m-0 text-xs text-muted-foreground">
        <TextLink href="/demo?team=1">gusto team: start a demo</TextLink>
      </p>
    </CloudCard>
  )
}

// DemoColleaguePage brings a colleague into a demo's room, by a link that works once.
export function DemoColleaguePage({ page }: { page: CloudPage }) {
  const back = (
    <Quiet className="text-center">
      <TextLink href={openDemo(page)}>Back to the demo</TextLink>
    </Quiet>
  )
  return (
    <CloudCard>
      <Brand />
      <Heading>Bring a colleague</Heading>
      {page.Link ? (
        <>
          <p className="m-0" role="status">
            {page.Email} is in the demo&apos;s room. Send them this link. It
            works once.
          </p>
          <Input
            className="h-11"
            type="text"
            value={page.Link}
            readOnly
            aria-label="Invitation link"
            onFocus={(event) => event.currentTarget.select()}
            data-testid="colleague-link"
          />
          <Quiet>
            We also mailed it to them. Both of you can then work in the room at
            once, and see each other&apos;s messages as they are sent.
          </Quiet>
          <ButtonLink href={openDemo(page)}>Back to the demo</ButtonLink>
        </>
      ) : page.IsAdmin ? (
        <>
          <p className="m-0">
            Your colleague joins this demo for the {page.DemoLeft} it has left,
            and lands in its room with you and the agents. Everything is deleted
            when the demo ends.
          </p>
          <Refusal>{page.Error}</Refusal>
          <form
            method="post"
            action="/demo/colleague"
            className="grid grid-cols-[1fr_auto] items-center gap-3"
          >
            <Input
              className="h-11"
              type="email"
              name="email"
              defaultValue={page.Email}
              placeholder="Their email address"
              aria-label="Their email address"
              autoComplete="off"
              required
              data-testid="colleague-email"
            />
            <Submit data-testid="invite-colleague">Invite</Submit>
          </form>
          {back}
        </>
      ) : (
        <>
          <p className="m-0">
            Only the person who started this demo can bring a colleague in.
          </p>
          {back}
        </>
      )}
    </CloudCard>
  )
}

// DemoStopPage asks before a demo is ended early, since everything in it is deleted.
export function DemoStopPage() {
  return (
    <CloudCard>
      <Brand />
      <Heading>End your demo now?</Heading>
      <p className="m-0">
        Your organization, its computers and everything your agents made are
        deleted straight away. This cannot be undone.
      </p>
      <form className="grid" method="post" action="/demo/end">
        <Submit variant="destructive" data-testid="end-demo">
          End my demo
        </Submit>
      </form>
      <Quiet className="text-center">
        <TextLink href="/demo/wait">Keep going</TextLink>
      </Quiet>
    </CloudCard>
  )
}

// DemoEndPage is a demo being deleted, which the page looks again at by itself, and then the demo that has ended.
export function DemoEndPage({ page }: { page: CloudPage }) {
  if (page.Ending) {
    return (
      <CloudCard>
        <Brand />
        <Heading>Ending your demo</Heading>
        <p className="m-0" role="status">
          Deleting your computers and everything on them. This takes a minute,
          and this page looks again by itself.
        </p>
      </CloudCard>
    )
  }
  return (
    <CloudCard>
      <Brand />
      <Heading>Your demo has ended</Heading>
      <p className="m-0" data-testid="demo-ended">
        Thanks{page.Name && `, ${page.Name}`}. Everything in it has been
        deleted.
      </p>
      <Quiet>
        We keep your name, work email and company so that we can follow up about
        a call.
      </Quiet>
      {page.Book && (
        <>
          <p className="m-0">
            The next step is a call, to see how this fits your team.
          </p>
          <ButtonLink href={page.Book}>Book a call</ButtonLink>
        </>
      )}
      <Quiet className="text-center">
        <TextLink href="/signup">Get started</TextLink> to keep going with your
        own team.
      </Quiet>
    </CloudCard>
  )
}
