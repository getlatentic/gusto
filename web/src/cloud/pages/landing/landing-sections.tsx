import type { ReactNode } from "react"

import {
  Band,
  BandText,
  BandTitle,
  BookCta,
  Cta,
  StartCta,
  docs,
  repository,
} from "@/cloud/pages/landing/landing-parts"
import type { CloudPage } from "@/cloud/types"

// Plans are the three ways to run gusto, each with what it costs and one way to start, the calls level along the bottom.
export function Plans({ page }: { page: CloudPage }) {
  return (
    <Band id="pricing" wash>
      <BandTitle>Choose how to run gusto</BandTitle>
      <ul className="m-0 mt-10 grid list-none grid-cols-[minmax(0,1fr)] gap-5 p-0 min-[52rem]:grid-cols-3">
        <Plan
          name="Run it yourself"
          price="Free"
          text="Open source under Apache-2.0. On your laptop or your own cloud."
        >
          <Cta href={repository}>Get it on GitHub</Cta>
        </Plan>
        <Plan
          name="gusto cloud"
          price="$25"
          text="A person a month. We run it. Your own model key, or model credit from gusto."
        >
          <StartCta />
        </Plan>
        <Plan
          name="For your company"
          price="Let's talk"
          text="Your cloud, your compute, our help."
        >
          {page.Book ? (
            <BookCta page={page} />
          ) : (
            <Cta href={`${docs}/PILOT.md`}>Read the pilot guide</Cta>
          )}
        </Plan>
      </ul>
    </Band>
  )
}

function Plan({
  name,
  price,
  text,
  children,
}: {
  name: string
  price: string
  text: string
  children: ReactNode
}) {
  return (
    <li
      data-reveal
      className="border-line-strong flex flex-col gap-3 rounded-2xl border bg-surface p-6"
    >
      <h3 className="text-quiet m-0 text-sm font-medium">{name}</h3>
      <p className="m-0 font-numeric text-[2rem] leading-10 font-semibold">
        {price}
      </p>
      <BandText>{text}</BandText>
      <div className="mt-auto pt-3">{children}</div>
    </li>
  )
}

export function TryWithYourTeam({ page }: { page: CloudPage }) {
  return (
    <section className="carbon bg-canvas text-ink py-14 text-center min-[44rem]:py-20">
      <div className="mx-auto grid max-w-6xl justify-items-center gap-6 px-5">
        <BandTitle>Try gusto with your team.</BandTitle>
        <div className="flex flex-wrap justify-center gap-3">
          <StartCta />
          <BookCta page={page} />
        </div>
      </div>
    </section>
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
