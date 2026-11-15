import { useState } from "react"

import {
  Band,
  BandTitle,
  BookCta,
  Cta,
  StartCta,
  Tile,
  TileText,
  docs,
  repository,
} from "@/cloud/pages/landing/landing-parts"
import type { CloudPage } from "@/cloud/types"
import { bookLabel } from "@/domain/landing"

export function RunIt({ page }: { page: CloudPage }) {
  return (
    <Band id="run" wash className="mt-14 min-[44rem]:mt-20">
      <BandTitle>Choose how to run gusto</BandTitle>
      <ul className="m-0 mt-10 grid list-none grid-cols-[minmax(0,1fr)] gap-5 p-0 min-[52rem]:grid-cols-3">
        <Tile title="Hosted by gusto">
          <TileText>We run it for you.</TileText>
          <StartCta page={page} className="mt-2 justify-self-start" />
        </Tile>
        <Tile title="Run it yourself">
          <TileText>Open source. On your laptop or in your own cloud.</TileText>
          <Cta className="mt-2 justify-self-start" href={repository}>
            Get it on GitHub
          </Cta>
        </Tile>
        <Tile title="For your company">
          <TileText>Your cloud, your compute, our help.</TileText>
          {page.Book ? (
            <BookCta page={page} className="mt-2 justify-self-start" />
          ) : (
            <Cta className="mt-2 justify-self-start" href={`${docs}/PILOT.md`}>
              Read the pilot guide
            </Cta>
          )}
        </Tile>
      </ul>
    </Band>
  )
}

const install =
  "curl -fsSL https://github.com/getlatentic/gusto/releases/download/v0.1.0-rc.2/install.sh | sh -s -- v0.1.0-rc.2"

export function StartInTwoSteps() {
  const steps: [string, string][] = [
    ["Start gusto.", "Run one command, or use gusto cloud and skip it."],
    [
      "Make a room and add an agent.",
      "Add a model key, pick an agent and ask it something.",
    ],
  ]
  return (
    <Band id="start">
      <div className="grid grid-cols-[minmax(0,1fr)] items-start gap-10 min-[52rem]:grid-cols-2 min-[52rem]:gap-12">
        <div className="grid content-start gap-6">
          <BandTitle>Start in two steps</BandTitle>
          <ol className="m-0 grid list-none gap-6 p-0">
            {steps.map(([title, text], index) => (
              <li
                key={title}
                className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3"
              >
                <span className="bg-solid text-md text-on-solid row-span-2 inline-grid size-7 place-items-center rounded-full font-semibold">
                  {index + 1}
                </span>
                <h3 className="text-copy m-0 leading-[1.3] font-semibold">
                  {title}
                </h3>
                <TileText>{text}</TileText>
              </li>
            ))}
          </ol>
          <p className="text-md text-quiet m-0">
            gusto is a pre-release.{" "}
            <a
              className="text-ink font-semibold underline"
              href={`${docs}/QUICKSTART.md`}
            >
              Read the quickstart
            </a>
            .
          </p>
        </div>
        <InstallCommand />
      </div>
    </Band>
  )
}

// InstallCommand is the one command that installs gusto on a computer, with a button that copies it where the browser lets it.
function InstallCommand() {
  const [copied, setCopied] = useState(false)
  const copy = () =>
    navigator.clipboard.writeText(install).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  return (
    <div className="carbon border-line-strong bg-canvas text-ink overflow-hidden rounded-lg border shadow-lg">
      <div className="border-line flex items-center justify-between border-b px-4 py-2">
        <span className="text-quiet text-sm">On your computer</span>
        {typeof navigator !== "undefined" && navigator.clipboard && (
          <button
            className="border-line-strong rounded-sm border px-3 py-1 text-sm font-semibold hover:bg-surface"
            type="button"
            onClick={copy}
            data-testid="copy-install"
          >
            {copied ? "Copied" : "Copy"}
          </button>
        )}
      </div>
      <pre className="m-0 px-4 py-4 font-mono text-[0.875rem] leading-[1.6] break-all whitespace-pre-wrap">
        <code data-testid="install-command">{install}</code>
      </pre>
    </div>
  )
}

const dayOne: [string, string][] = [
  [
    "Shared rooms",
    "Ask an agent by name. Everyone in the room sees the answer and the files.",
  ],
  [
    "Any agent",
    "Claude Code, Codex, OpenCode and more, each on a model you approve.",
  ],
  [
    "Your apps stay yours",
    "Each person chooses which of their apps an agent may use. It never holds the sign-in.",
  ],
  [
    "Only the models you approve",
    "Each agent reaches its approved model and nothing else. Refused connections are listed.",
  ],
  [
    "Budgets that stop agents",
    "A monthly budget per person, checked before every model call.",
  ],
  [
    "A record of every run",
    "Administrators see what every run did, what it cost and how long it took.",
  ],
]

export function DayOne() {
  return (
    <Band id="day-one" wash>
      <BandTitle>What you get from day one</BandTitle>
      <ul className="m-0 mt-10 grid list-none grid-cols-[minmax(0,1fr)] gap-5 p-0 min-[44rem]:grid-cols-2 min-[64rem]:grid-cols-3">
        {dayOne.map(([title, text]) => (
          <Tile key={title} title={title} className="gap-2">
            <TileText>{text}</TileText>
          </Tile>
        ))}
      </ul>
    </Band>
  )
}

export function Proof({ page }: { page: CloudPage }) {
  return (
    <Band id="proof" wash>
      <div className="grid grid-cols-[minmax(0,1fr)] items-center gap-10 min-[52rem]:grid-cols-[minmax(0,5fr)_minmax(0,6fr)] min-[52rem]:gap-12">
        <div className="grid content-start gap-4">
          <BandTitle>Tested across multiple agents.</BandTitle>
          <p className="text-lead text-quiet m-0 leading-[1.6]">
            Pi, Claude Code, Codex, Hermes Agent and OpenCode each fixed the
            same small bug, and a test confirmed the fix.
          </p>
        </div>
        <figure className="m-0">
          <picture>
            <source
              media="(prefers-color-scheme: dark)"
              srcSet={page.RecordDark}
            />
            <img
              className="border-line block h-auto w-full rounded-lg border shadow-lg"
              src={page.Record}
              width={616}
              height={144}
              alt="What one reply cost and how long it took"
              loading="lazy"
              decoding="async"
            />
          </picture>
        </figure>
      </div>
    </Band>
  )
}

function Price({ children }: { children: string }) {
  return (
    <p className="m-0 font-heading text-[length:clamp(1.5rem,3vw,2rem)] leading-none font-semibold">
      {children}
    </p>
  )
}

export function Pricing({ page }: { page: CloudPage }) {
  return (
    <Band id="pricing">
      <BandTitle>Simple pricing</BandTitle>
      <ul className="m-0 mt-10 grid list-none grid-cols-[minmax(0,1fr)] gap-5 p-0 min-[52rem]:grid-cols-3">
        <Tile title="Open source">
          <Price>Free</Price>
          <TileText>Apache-2.0.</TileText>
          <a className="text-md font-semibold underline" href={repository}>
            github.com/getlatentic/gusto
          </a>
        </Tile>
        <Tile title="gusto cloud">
          <Price>$25</Price>
          <TileText>
            A person a month. Your own model key, or gusto&rsquo;s models on
            credit.
          </TileText>
        </Tile>
        <Tile title="Enterprise">
          <Price>Let&rsquo;s talk</Price>
          <TileText>Your cloud and your compute.</TileText>
          {page.Book && (
            <a className="text-md font-semibold underline" href={page.Book}>
              {bookLabel(page)}
            </a>
          )}
        </Tile>
      </ul>
    </Band>
  )
}

export function TryWithYourTeam({ page }: { page: CloudPage }) {
  return (
    <section className="carbon bg-canvas text-ink py-14 text-center min-[44rem]:py-20">
      <div className="mx-auto grid max-w-5xl justify-items-center gap-5 px-5">
        <BandTitle>Try gusto with your team.</BandTitle>
        <div className="flex flex-wrap justify-center gap-3">
          <StartCta page={page} />
          <BookCta page={page} className="hover:bg-surface" />
        </div>
      </div>
    </section>
  )
}

export function HelpAndFooter() {
  const help: [string, string][] = [
    [`${docs}/QUICKSTART.md`, "Quickstart"],
    [`${docs}/REFERENCE.md`, "Reference"],
    [`${repository}/issues`, "GitHub issues"],
  ]
  return (
    <>
      <section id="help" className="bg-canvas text-ink py-10">
        <div className="mx-auto flex max-w-5xl flex-wrap items-baseline gap-x-8 gap-y-2 px-5">
          <h2 className="m-0 text-lg leading-[1.3] font-semibold">
            Help and support
          </h2>
          {help.map(([href, label]) => (
            <a
              key={href}
              className="text-md font-semibold underline"
              href={href}
            >
              {label}
            </a>
          ))}
        </div>
      </section>
      <footer className="bg-canvas text-quiet py-8 text-sm">
        <div className="mx-auto flex max-w-5xl flex-wrap justify-between gap-2 px-5">
          <span>
            Made by{" "}
            <a className="underline" href="https://getlatentic.com">
              Latentic
            </a>
          </span>
          <span>Apache License 2.0</span>
        </div>
      </footer>
    </>
  )
}
