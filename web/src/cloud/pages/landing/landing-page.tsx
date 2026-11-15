import {
  BookCta,
  Cta,
  StartCta,
  docs,
  repository,
} from "@/cloud/pages/landing/landing-parts"
import {
  DayOne,
  HelpAndFooter,
  Pricing,
  Proof,
  RunIt,
  StartInTwoSteps,
  TryWithYourTeam,
} from "@/cloud/pages/landing/landing-sections"
import { WorksWith } from "@/cloud/pages/landing/works-with"
import type { CloudPage } from "@/cloud/types"
import { offerTerms } from "@/domain/landing"

// LandingPage is what a visitor sees first: what gusto is, the product in use, how to run it, what comes with it, what it costs,
// and where to start.
export function LandingPage({ page }: { page: CloudPage }) {
  return (
    <div className="bg-canvas text-copy text-ink scroll-pt-18 leading-[1.6]">
      <LandingHeader page={page} />
      <main>
        <Hero page={page} />
      </main>
      <RunIt page={page} />
      <StartInTwoSteps />
      <DayOne />
      <WorksWith />
      <Proof page={page} />
      <Pricing page={page} />
      <TryWithYourTeam page={page} />
      <HelpAndFooter />
    </div>
  )
}

function LandingHeader({ page }: { page: CloudPage }) {
  const links: [string, string][] = [
    ["#run", "Run it"],
    ["#day-one", "What you get"],
    ["#pricing", "Pricing"],
    [`${docs}/QUICKSTART.md`, "Docs"],
  ]
  return (
    <header className="border-line bg-canvas sticky top-0 z-50 flex items-center justify-between gap-4 border-b px-[max(1.25rem,calc((100vw-64rem)/2))] py-4">
      <a
        className="flex items-center gap-3 font-heading text-lg leading-none font-semibold tracking-[-0.01em] no-underline"
        href="/"
      >
        <img className="h-auto w-7" src="/static/mark.svg" alt="" /> gusto
      </a>
      <nav className="text-md flex items-center gap-5" aria-label="gusto">
        {links.map(([href, label]) => (
          <a
            key={href}
            className="text-quiet hover:text-ink no-underline max-[48rem]:hidden"
            href={href}
          >
            {label}
          </a>
        ))}
        <a
          className="text-quiet hover:text-ink inline-flex items-center"
          href={repository}
          aria-label="gusto on GitHub"
        >
          <GitHubMark />
        </a>
        {page.SignedIn ? (
          <Cta
            solid
            className="px-4 py-2 whitespace-nowrap"
            href="/app"
            data-testid="open-gusto"
          >
            Open gusto
          </Cta>
        ) : (
          <>
            <a
              className="text-quiet hover:text-ink whitespace-nowrap no-underline"
              href="/signin"
              data-testid="sign-in"
            >
              Sign in
            </a>
            <StartCta page={page} className="px-4 py-2 whitespace-nowrap" />
          </>
        )}
      </nav>
    </header>
  )
}

function Hero({ page }: { page: CloudPage }) {
  return (
    <section className="overflow-hidden">
      <div className="mx-auto grid max-w-5xl items-center gap-10 px-5 pt-14 min-[44rem]:pt-20 min-[64rem]:grid-cols-[minmax(0,25rem)_minmax(0,1fr)] min-[64rem]:gap-12">
        <div className="grid gap-5">
          <p className="text-accent-ink m-0 text-sm font-semibold tracking-wide uppercase">
            Open source · Apache-2.0
          </p>
          <h1 className="m-0 text-[length:clamp(2rem,5vw,3rem)] leading-[1.1] font-semibold tracking-[-0.02em]">
            One workspace for every AI agent your team uses.
          </h1>
          <p className="text-lead text-quiet m-0">
            Your team and its agents work in shared rooms. Agents reach only the
            models you approve, and budgets stop spend before it happens.
          </p>
          <div className="flex flex-wrap gap-3">
            <StartCta page={page} />
            <BookCta page={page} />
          </div>
          <p
            className="text-quiet m-0 text-[0.875rem]"
            data-testid="offer-terms"
          >
            {offerTerms(page)}
          </p>
        </div>
        <HeroPicture page={page} />
      </div>
    </section>
  )
}

// HeroPicture is the product in use beside the promise: the operator's own video when one is named, otherwise two people and their
// agents in one room, in a window that runs off the page's edge.
function HeroPicture({ page }: { page: CloudPage }) {
  if (page.Video) {
    return (
      <div aria-label="gusto, from inside">
        {page.VideoFile ? (
          <video
            className="aspect-video w-full rounded-lg bg-black shadow-lg"
            controls
            playsInline
            preload="metadata"
            poster={page.VideoPoster || undefined}
            src={page.Video}
          />
        ) : (
          <iframe
            className="aspect-video w-full rounded-lg border-0 bg-black shadow-lg"
            src={page.Video}
            title="gusto"
            loading="lazy"
            allow="fullscreen; picture-in-picture"
            allowFullScreen
            referrerPolicy="strict-origin-when-cross-origin"
          />
        )}
      </div>
    )
  }
  if (!page.Shot) return null
  return (
    <div className="min-w-0 min-[64rem]:w-[58rem]" aria-label="gusto in use">
      <div className="border-line-strong overflow-hidden rounded-lg border bg-surface shadow-lg min-[64rem]:rounded-r-none min-[64rem]:border-r-0">
        <div
          className="border-line bg-wash flex items-center gap-1.5 border-b px-4 py-3"
          aria-hidden="true"
        >
          <span className="bg-line-strong size-2.5 rounded-full" />
          <span className="bg-line-strong size-2.5 rounded-full" />
          <span className="bg-line-strong size-2.5 rounded-full" />
        </div>
        <div className="max-h-[22rem] overflow-hidden min-[44rem]:max-h-[34rem]">
          <picture>
            <source
              media="(prefers-color-scheme: dark)"
              srcSet={page.ShotDark}
            />
            <img
              className="block h-auto w-full"
              src={page.Shot}
              width={1568}
              height={1238}
              alt="Two people and their agents working in one room in gusto"
            />
          </picture>
        </div>
      </div>
    </div>
  )
}

function GitHubMark() {
  return (
    <svg
      viewBox="0 0 16 16"
      width="22"
      height="22"
      fill="currentColor"
      aria-hidden="true"
    >
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z" />
    </svg>
  )
}
