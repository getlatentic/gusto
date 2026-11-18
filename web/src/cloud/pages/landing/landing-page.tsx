import { lazy, Suspense, type CSSProperties } from "react"

import { BookCta, StartCta } from "@/cloud/pages/landing/landing-parts"
import { Plans, TryWithYourTeam } from "@/cloud/pages/landing/landing-sections"
import { Footer, SiteHeader } from "@/cloud/pages/site-page"
import { ProductDemo } from "@/cloud/pages/landing/demo/product-demo"
import { Features } from "@/cloud/pages/landing/features"
import { Contact } from "@/cloud/pages/landing/contact"
import type { CloudPage } from "@/cloud/types"

const LandingMotion = lazy(() =>
  import("@/cloud/pages/landing/landing-motion").then((m) => ({
    default: m.LandingMotion,
  }))
)

// LandingPage is what a visitor sees first: what gusto is worth to a team, the product in use, what comes with it, how to start,
// the three ways to run it with what each costs, and a note to the team.
export function LandingPage({ page }: { page: CloudPage }) {
  return (
    <div className="bg-canvas text-ink scroll-pt-18 text-base">
      <SiteHeader page={page} />
      <main>
        <Hero page={page} />
        <section
          aria-labelledby="demo-title"
          className="pb-14 min-[44rem]:pb-20"
        >
          <h2 id="demo-title" className="sr-only">
            See gusto at work
          </h2>
          <div className="mx-auto max-w-6xl px-5 max-[30rem]:px-3">
            <ProductDemo />
          </div>
        </section>
      </main>
      <Features />
      <Plans page={page} />
      <Contact page={page} />
      <TryWithYourTeam page={page} />
      <Footer />
      <Suspense>
        <LandingMotion />
      </Suspense>
    </div>
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

// Hero is the promise, centred, with the ways to start under it. The product itself is shown straight after, at full width.
function Hero({ page }: { page: CloudPage }) {
  return (
    // The hero is isolated so its backdrop stays behind it.
    <section className="relative isolate">
      <HeroBackdrop />
      <div className="mx-auto grid max-w-3xl justify-items-center gap-4 px-5 pt-10 pb-8 text-center min-[44rem]:pt-16 min-[44rem]:pb-10">
        <h1 className="m-0 text-[length:clamp(1.875rem,5vw,3rem)] leading-[1.1] font-semibold tracking-[-0.02em] text-balance">
          Bring your team and AI agents together.
        </h1>
        <p className="text-quiet m-0 max-w-2xl text-[15px] leading-snug text-balance min-[30rem]:text-base min-[44rem]:text-lg min-[44rem]:leading-normal">
          Collaborate in one workspace. Run your chosen agents in gusto’s AI
          agent cloud, with control over spending, models and integrations.
        </p>
        <div className="mt-2 flex flex-wrap justify-center gap-3">
          <StartCta page={page} />
          <BookCta page={page} />
        </div>
      </div>
    </section>
  )
}
