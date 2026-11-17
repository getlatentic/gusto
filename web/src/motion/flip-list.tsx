import { Component, createRef, type ReactNode } from "react"

import { Flip, gsap, moves, settle } from "@/motion/gsap"

interface Snapshot {
  ids: string
  state: Flip.FlipState
}

const rows = "[data-flip-id]"

const idsOf = (elements: HTMLElement[]) =>
  elements.map((element) => element.dataset.flipId).join("\n")

// FlipList moves the rows inside it, each an element marked data-flip-id, to their new places when the list changes, and fades
// in rows that are new, so a list that reorders or filters moves rather than jumps. It draws no box of its own.
//
// It is a class because a row's place has to be read after React has decided on the change and before it changes the page,
// which only getSnapshotBeforeUpdate offers; an effect runs after the rows have already moved.
export class FlipList extends Component<{ children: ReactNode }> {
  private readonly box = createRef<HTMLDivElement>()

  private rows(): HTMLElement[] {
    return [...(this.box.current?.querySelectorAll<HTMLElement>(rows) ?? [])]
  }

  getSnapshotBeforeUpdate(): Snapshot | null {
    if (!moves()) return null
    const now = this.rows()
    return { ids: idsOf(now), state: Flip.getState(now, { simple: true }) }
  }

  componentDidUpdate(
    _props: unknown,
    _state: unknown,
    before: Snapshot | null
  ) {
    const after = this.rows()
    if (!before || before.ids === idsOf(after)) return
    Flip.from(before.state, {
      targets: after,
      duration: settle,
      ease: "power2.inOut",
      simple: true,
      onEnter: (entering) =>
        gsap.fromTo(entering, { opacity: 0 }, { opacity: 1, duration: settle }),
    })
  }

  componentWillUnmount() {
    Flip.killFlipsOf(this.rows())
  }

  render() {
    return (
      <div ref={this.box} className="contents">
        {this.props.children}
      </div>
    )
  }
}
