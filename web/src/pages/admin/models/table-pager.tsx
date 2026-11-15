import { Pager } from "@/components/controls"
import type { Pages } from "./types"

// TablePager is the way to the pages either side of one of this page's tables.
export function TablePager({ pages }: { pages?: Pages }) {
  return (
    <Pager
      newer={pages?.Previous}
      older={pages?.Next}
      newerLabel="Previous"
      olderLabel="Next"
      shown={pages?.Shown}
    />
  )
}
