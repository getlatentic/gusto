import { useEffect, useState, type ReactNode } from "react"

import { Cta, StartCta } from "@/cloud/pages/landing/landing-parts"
import { PageIntro, SitePage } from "@/cloud/pages/site-page"
import type { CloudPage } from "@/cloud/types"
import { calendarHeight, framedAt } from "@/domain/booking"

// DemoBookPage is where a demo is booked with the team, where only the team starts them: the booking service's calendar on the
// page where it may be framed, or a link to it, and the other ways on.
export function DemoBookPage({ page }: { page: CloudPage }) {
  return (
    <SitePage page={page}>
      <PageIntro title="Book a demo">
        Someone from the gusto team runs it with you on a call: agents of
        several vendors at work in one room, and the controls your organization
        keeps over them.
      </PageIntro>
      <Calendar page={page} />
      <OtherWays page={page} />
    </SitePage>
  )
}

// Calendar is the booking service's own calendar, at the page's width and as tall as it says it is, or a button to it where it
// may not be framed.
function Calendar({ page }: { page: CloudPage }) {
  const height = useCalendarHeight()
  if (page.BookFrame) {
    return (
      <section aria-label="Pick a time" className="mx-auto max-w-4xl px-5">
        <iframe
          className="block h-[44rem] w-full"
          style={height ? { height } : undefined}
          src={framedAt(page.BookFrame, window.location.hostname)}
          title="Pick a time for a demo of gusto"
          loading="lazy"
          referrerPolicy="strict-origin-when-cross-origin"
        />
      </section>
    )
  }
  if (!page.Book) return null
  return (
    <p className="m-0 flex justify-center px-5">
      <Cta solid href={page.Book}>
        Book a demo
      </Cta>
    </p>
  )
}

// OtherWays is what a visitor may do instead of booking: open the calendar elsewhere when it does not load here, or start on
// their own. The team's way into a demo is last and quietest.
function OtherWays({ page }: { page: CloudPage }) {
  return (
    <section
      aria-label="Other ways"
      className="text-quiet mx-auto grid max-w-4xl justify-items-center gap-6 px-5 pt-6 pb-16 text-center text-sm"
    >
      {page.BookFrame && page.Book && (
        <p className="m-0">
          Calendar not loading?{" "}
          <Link href={page.Book}>Open it in a new tab</Link>
        </p>
      )}
      <p className="m-0 flex flex-wrap items-center justify-center gap-3">
        Rather try it yourself?
        <StartCta page={page} />
      </p>
      <a
        className="text-quiet hover:text-ink text-xs no-underline"
        href="/demo?team=1"
      >
        Team demo
      </a>
    </section>
  )
}

// useCalendarHeight is the height the framed calendar last said it is, in pixels, and nothing until it has said.
function useCalendarHeight(): number | undefined {
  const [height, setHeight] = useState<number>()
  useEffect(() => {
    const told = (event: MessageEvent) => {
      const said = calendarHeight(event.origin, event.data)
      if (said !== null) setHeight(said)
    }
    window.addEventListener("message", told)
    return () => window.removeEventListener("message", told)
  }, [])
  return height
}

function Link({ href, children }: { href: string; children: ReactNode }) {
  return (
    <a
      className="text-ink underline underline-offset-2"
      href={href}
      target="_blank"
      rel="noreferrer"
    >
      {children}
    </a>
  )
}
