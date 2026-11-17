import { useEffect, useRef, useState, type ReactNode } from "react"
import { PauseIcon, PlayIcon } from "lucide-react"

import {
  ComputerScene,
  computerScene,
} from "@/cloud/pages/landing/demo/computer-scene"
import {
  ControlScene,
  controlScene,
} from "@/cloud/pages/landing/demo/control-scene"
import { DemoCursor } from "@/cloud/pages/landing/demo/demo-cursor"
import { CompactDemo } from "@/cloud/pages/landing/demo/demo-parts"
import { RoomScene, roomScene } from "@/cloud/pages/landing/demo/room-scene"
import { frameAt } from "@/cloud/pages/landing/demo/script"
import { useDemoClock } from "@/cloud/pages/landing/demo/use-demo-clock"
import {
  useDemoSize,
  useOnScreen,
} from "@/cloud/pages/landing/demo/use-demo-view"
import { cn } from "@/lib/utils"

interface Drawn {
  view: ReactNode
  cursor: string | null
  label: string
  clicking: boolean
}

// Each scene: its tab, what it shows in words for a screen reader, how long it plays, and how to draw it at a moment.
const scenes: {
  key: string
  tab: string
  // short is the tab's name on a narrow screen.
  short: string
  about: string
  length: number
  draw: (ms: number) => Drawn
}[] = [
  {
    key: "rooms",
    tab: "Shared rooms",
    short: "Rooms",
    about:
      "In a shared room, Sam asks the Hermes agent for a release plan. Hermes writes release-plan.md, and Priya asks Claude Code to review it.",
    length: roomScene.length,
    draw: (ms) => {
      const frame = frameAt(roomScene, ms)
      return { ...frame, view: <RoomScene frame={frame} /> }
    },
  },
  {
    key: "computers",
    tab: "Private computers",
    short: "Computers",
    about:
      "Sam opens the private cloud computer Hermes works in and watches it read the roadmap, write the plan and finish in 41 seconds for two cents.",
    length: computerScene.length,
    draw: (ms) => {
      const frame = frameAt(computerScene, ms)
      return { ...frame, view: <ComputerScene frame={frame} /> }
    },
  },
  {
    key: "control",
    tab: "Stay in control",
    short: "Control",
    about:
      "An admin raises Priya's budget to $50 a month, which brings the team's budget to $130, allows the DeepSeek V4 Pro model, and on the tools page sees Hermes blocked from sending an email with Gmail, which the workspace does not allow.",
    length: controlScene.length,
    draw: (ms) => {
      const frame = frameAt(controlScene, ms)
      return { ...frame, view: <ControlScene frame={frame} /> }
    },
  },
]

const lengths = scenes.map((scene) => scene.length)

// ProductDemo is gusto at work, drawn in the browser: a window that plays a scene of the product for each tab, a cursor showing
// what is clicked. It plays only while on screen and stops moving on by itself once a person picks a tab or pauses. A person who
// asked for less motion sees each scene as it ends, with no cursor.
export function ProductDemo() {
  const box = useRef<HTMLDivElement>(null)
  const stage = useRef<HTMLDivElement>(null)
  const onScreen = useOnScreen(box)
  const size = useDemoSize(box)
  const calm = useCalm()
  const { clock, choose, togglePause } = useDemoClock(lengths, onScreen && calm)
  const scene = scenes[clock.scene]
  const drawn = scene.draw(calm ? clock.ms : Number.POSITIVE_INFINITY)
  return (
    <div ref={box} className="grid gap-4" data-testid="product-demo">
      <div className="flex items-end justify-center gap-1">
        <div
          role="tablist"
          aria-label="See gusto at work"
          className="flex gap-6 max-[30rem]:gap-4"
        >
          {scenes.map((each, at) => {
            const current = at === clock.scene
            return (
              <button
                key={each.key}
                id={`demo-tab-${each.key}`}
                type="button"
                role="tab"
                aria-selected={current}
                aria-controls="demo-panel"
                aria-label={each.tab}
                onClick={() => choose(at)}
                className={cn(
                  "grid cursor-pointer gap-2 border-0 bg-transparent px-0 pt-1 text-sm font-medium whitespace-nowrap",
                  current ? "text-ink" : "text-quiet hover:text-ink"
                )}
              >
                <span className="max-[30rem]:hidden">{each.tab}</span>
                <span className="min-[30rem]:hidden">{each.short}</span>
                <Progress
                  part={current ? (calm ? clock.ms / each.length : 1) : 0}
                />
              </button>
            )
          })}
        </div>
        {calm && (
          <button
            type="button"
            onClick={togglePause}
            aria-label={clock.paused ? "Play the demo" : "Pause the demo"}
            className="text-quiet hover:text-ink grid size-8 cursor-pointer place-items-center rounded-full border-0 bg-transparent"
          >
            {clock.paused ? (
              <PlayIcon className="size-3.5" />
            ) : (
              <PauseIcon className="size-3.5" />
            )}
          </button>
        )}
      </div>
      <div
        id="demo-panel"
        role="tabpanel"
        aria-labelledby={`demo-tab-${scene.key}`}
        className="border-line-strong mx-auto max-w-full overflow-hidden rounded-xl border shadow-lg"
        style={{
          width: size.width * size.scale,
          height: size.height * size.scale,
        }}
      >
        <p className="sr-only">{scene.about}</p>
        <div
          ref={stage}
          aria-hidden="true"
          data-compact={size.compact}
          className="group relative origin-top-left text-left select-none"
          style={{
            width: size.width,
            height: size.height,
            transform: size.scale === 1 ? undefined : `scale(${size.scale})`,
          }}
        >
          <CompactDemo value={size.compact}>{drawn.view}</CompactDemo>
          {calm && (
            <DemoCursor
              stage={stage}
              scale={size.scale}
              target={drawn.cursor}
              label={drawn.label}
              clicking={drawn.clicking}
            />
          )}
        </div>
      </div>
    </div>
  )
}

// Progress is the thin line under a tab: how much of its scene has played.
function Progress({ part }: { part: number }) {
  return (
    <span className="bg-line block h-0.5 overflow-hidden rounded-full">
      <span
        className="bg-ink block h-full origin-left transition-transform duration-100 ease-linear"
        style={{ transform: `scaleX(${Math.min(1, part)})` }}
      />
    </span>
  )
}

// useCalm is whether the person has not asked for less motion, and follows them if they change it.
function useCalm(): boolean {
  const query = "(prefers-reduced-motion: no-preference)"
  const [calm, setCalm] = useState(() => window.matchMedia(query).matches)
  useEffect(() => {
    const media = window.matchMedia(query)
    const follow = () => setCalm(media.matches)
    media.addEventListener("change", follow)
    return () => media.removeEventListener("change", follow)
  }, [])
  return calm
}
