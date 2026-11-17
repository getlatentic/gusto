import { lazy, Suspense, type CSSProperties } from "react"

import {
  BookCta,
  Cta,
  StartCta,
  docs,
  repository,
} from "@/cloud/pages/landing/landing-parts"
import {
  Footer,
  Plans,
  TryWithYourTeam,
} from "@/cloud/pages/landing/landing-sections"
import { ProductDemo } from "@/cloud/pages/landing/demo/product-demo"
import { Features } from "@/cloud/pages/landing/features"
import type { CloudPage } from "@/cloud/types"
import { offerTerms } from "@/domain/landing"

const LandingMotion = lazy(() =>
  import("@/cloud/pages/landing/landing-motion").then((m) => ({
    default: m.LandingMotion,
  }))
)

// LandingPage is what a visitor sees first: what gusto is worth to a team, the product in use, what comes with it, how to start,
// and the three ways to run it with what each costs.
export function LandingPage({ page }: { page: CloudPage }) {
  return (
    <div className="bg-canvas text-ink scroll-pt-18 text-base">
      <LandingHeader page={page} />
      <main>
        <Hero page={page} />
        <section
          aria-labelledby="demo-title"
          className="pb-14 min-[44rem]:pb-20"
        >
          <h2 id="demo-title" className="sr-only">
            See gusto at work
          </h2>
          <div className="mx-auto max-w-6xl px-5">
            <ProductDemo />
          </div>
        </section>
      </main>
      <Features />
      <Plans page={page} />
      <TryWithYourTeam page={page} />
      <Footer />
      <Suspense>
        <LandingMotion />
      </Suspense>
    </div>
  )
}

function LandingHeader({ page }: { page: CloudPage }) {
  const links: [string, string][] = [
    ["#features", "Features"],
    ["#pricing", "Pricing"],
    [`${docs}/QUICKSTART.md`, "Docs"],
  ]
  return (
    <header className="border-line bg-canvas sticky top-0 z-50 flex items-center justify-between gap-4 border-b px-[max(1.25rem,calc((100vw-69.5rem)/2))] py-4">
      <a
        className="flex items-center gap-3 font-heading text-lg leading-none font-semibold tracking-[-0.01em] no-underline"
        href="/"
      >
        <img className="h-auto w-7" src="/static/mark.svg" alt="" /> gusto
      </a>
      <nav className="flex items-center gap-5 text-sm" aria-label="gusto">
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
          <Cta solid href="/app" data-testid="open-gusto">
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
            <StartCta page={page} />
          </>
        )}
      </nav>
    </header>
  )
}

// HeroBackdrop is the soft green wash behind the hero, one image for each theme, faded out at the hero's foot so the demo below
// sits on the plain page.
function HeroBackdrop() {
  return (
    <div
      aria-hidden
      // The images are served by the gusto server, not bundled, so they reach the CSS through variables Vite leaves alone.
      style={
        {
          "--hero-light": "url(/static/landing/hero-light.webp)",
          "--hero-dark": "url(/static/landing/hero-dark.webp)",
        } as CSSProperties
      }
      className="pointer-events-none absolute inset-0 -z-10 bg-(image:--hero-light) [mask-image:linear-gradient(to_bottom,black_70%,transparent)] bg-cover bg-[position:70%_35%] dark:bg-(image:--hero-dark)"
    />
  )
}

// Hero is the promise, centred, with the ways to start under it and what starting costs. The product itself is shown straight after, at full width.
function Hero({ page }: { page: CloudPage }) {
  return (
    // The hero is isolated so its backdrop stays behind it.
    <section className="relative isolate">
      <HeroBackdrop />
      <div className="mx-auto grid max-w-3xl justify-items-center gap-4 px-5 pt-12 pb-8 text-center min-[44rem]:pt-16 min-[44rem]:pb-10">
        <h1 className="m-0 text-[length:clamp(2.25rem,5vw,3rem)] leading-[1.08] font-semibold tracking-[-0.02em] text-balance">
          Bring your team and AI agents together.
        </h1>
        <p className="text-quiet m-0 max-w-2xl text-base leading-snug text-balance min-[44rem]:text-lg min-[44rem]:leading-normal">
          Collaborate in one workspace. Run your chosen agents in gusto’s AI
          agent cloud, with control over spending, models and integrations.
        </p>
        <div className="mt-2 flex flex-wrap justify-center gap-3">
          <StartCta page={page} />
          <BookCta page={page} />
        </div>
        <p className="text-quiet m-0 text-sm" data-testid="offer-terms">
          {offerTerms(page.StartingCredit)}
        </p>
      </div>
    </section>
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
