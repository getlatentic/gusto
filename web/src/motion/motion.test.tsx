import { useRef } from "react"
import { render } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { FlipList } from "@/motion/flip-list"
import { Flip, gsap } from "@/motion/gsap"
import { useArrivals } from "@/motion/use-arrivals"
import { usePageEnter } from "@/motion/use-page-enter"

// motion says whether the person has asked for less motion, as the browser would.
function motion(calm: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: calm && query.includes("no-preference"),
    media: query,
    addEventListener() {},
    removeEventListener() {},
  }))
}

afterEach(() => vi.unstubAllGlobals())

function Page({ at }: { at: string }) {
  const box = useRef<HTMLDivElement>(null)
  usePageEnter(box, at)
  return <div ref={box}>{at}</div>
}

function Messages({ ids }: { ids: string[] }) {
  const box = useRef<HTMLOListElement>(null)
  useArrivals(box, ids)
  return (
    <ol ref={box}>
      {ids.map((id) => (
        <li key={id} data-arrive-id={id}>
          {id}
        </li>
      ))}
    </ol>
  )
}

function Rows({ ids }: { ids: string[] }) {
  return (
    <FlipList>
      <ul>
        {ids.map((id) => (
          <li key={id} data-flip-id={id}>
            {id}
          </li>
        ))}
      </ul>
    </FlipList>
  )
}

describe("moving between pages", () => {
  it("fades the next page in, and not the first one or one whose address is the same", () => {
    motion(true)
    const fade = vi.spyOn(gsap, "fromTo")
    const drawn = render(<Page at="/jobs" />)
    drawn.rerender(<Page at="/jobs" />)
    expect(fade).not.toHaveBeenCalled()
    drawn.rerender(<Page at="/usage" />)
    expect(fade).toHaveBeenCalledOnce()
  })

  it("does not move for a person who asked for less motion", () => {
    motion(false)
    const fade = vi.spyOn(gsap, "fromTo")
    render(<Page at="/jobs" />).rerender(<Page at="/usage" />)
    expect(fade).not.toHaveBeenCalled()
  })
})

describe("messages arriving", () => {
  it("brings in only what is new, and none of what was there at first", () => {
    motion(true)
    const bring = vi.spyOn(gsap, "from")
    const drawn = render(<Messages ids={["a", "b"]} />)
    expect(bring).not.toHaveBeenCalled()
    drawn.rerender(<Messages ids={["a", "b", "c"]} />)
    expect(bring).toHaveBeenCalledOnce()
    const brought = bring.mock.calls[0][0] as HTMLElement[]
    expect(brought.map((element) => element.textContent)).toEqual(["c"])
  })
})

describe("a list whose rows move", () => {
  it("moves the rows when their order changes, and leaves them when it does not", () => {
    motion(true)
    const move = vi.spyOn(Flip, "from")
    const drawn = render(<Rows ids={["a", "b"]} />)
    drawn.rerender(<Rows ids={["a", "b"]} />)
    expect(move).not.toHaveBeenCalled()
    drawn.rerender(<Rows ids={["b", "a"]} />)
    expect(move).toHaveBeenCalledOnce()
  })

  it("does not move for a person who asked for less motion", () => {
    motion(false)
    const move = vi.spyOn(Flip, "from")
    render(<Rows ids={["a", "b"]} />).rerender(<Rows ids={["b", "a"]} />)
    expect(move).not.toHaveBeenCalled()
  })
})
