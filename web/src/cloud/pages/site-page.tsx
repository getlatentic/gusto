import type { ReactNode } from "react"

import {
  Cta,
  StartCta,
  docs,
  repository,
} from "@/cloud/pages/landing/landing-parts"
import type { CloudPage } from "@/cloud/types"

// SitePage is a page of gusto's public site that is not the landing page: the site's header, the page's own sections, and the
// site's footer, on the same plain page as the landing page.
export function SitePage({
  page,
  children,
}: {
  page: CloudPage
  children: ReactNode
}) {
  return (
    <div className="bg-canvas text-ink flex min-h-dvh flex-col text-base">
      <SiteHeader page={page} />
      <main className="flex-1">{children}</main>
      <Footer />
    </div>
  )
}

// PageIntro is a site page's title and what the page is for, centred as the landing page's hero is.
export function PageIntro({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <header className="mx-auto grid max-w-3xl justify-items-center gap-3 px-5 pt-10 pb-8 text-center min-[44rem]:pt-14">
      <h1 className="m-0 text-[length:clamp(1.75rem,4vw,2.5rem)] leading-[1.15] font-semibold tracking-[-0.02em] text-balance">
        {title}
      </h1>
      <p className="text-quiet m-0 max-w-2xl text-[15px] leading-snug text-balance min-[44rem]:text-lg min-[44rem]:leading-normal">
        {children}
      </p>
    </header>
  )
}

// SiteHeader is the site's header on every public page: the mark, the way to the landing page's sections and the docs, and
// the way in.
export function SiteHeader({ page }: { page: CloudPage }) {
  const links: [string, string][] = [
    ["/#features", "Features"],
    ["/#pricing", "Pricing"],
    [`${docs}/QUICKSTART.md`, "Docs"],
  ]
  return (
    <header className="border-line bg-canvas sticky top-0 z-50 flex items-center justify-between gap-4 border-b px-[max(1.25rem,calc((100vw-69.5rem)/2))] py-4">
      <a
        className="flex shrink-0 items-center gap-3 font-heading text-lg leading-none font-semibold tracking-[-0.01em] no-underline"
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
          className="text-quiet hover:text-ink inline-flex items-center max-[30rem]:hidden"
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

export function Footer() {
  const links: [string, string][] = [
    [`${docs}/QUICKSTART.md`, "Quickstart"],
    [`${docs}/REFERENCE.md`, "Reference"],
    [`${repository}/issues`, "Help"],
  ]
  return (
    <footer className="bg-canvas text-quiet py-8 text-sm">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-3 px-5">
        <a
          className="hover:text-ink no-underline"
          href="https://getlatentic.com"
        >
          Made by Latentic
        </a>
        <span className="flex-1" />
        <nav aria-label="Help" className="flex flex-wrap gap-x-6 gap-y-2">
          {links.map(([href, label]) => (
            <a key={href} className="hover:text-ink no-underline" href={href}>
              {label}
            </a>
          ))}
        </nav>
        <span>Apache-2.0</span>
      </div>
    </footer>
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
