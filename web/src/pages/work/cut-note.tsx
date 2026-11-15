import type { Cut } from "@/pages/work/types"

// CutNote says the page draws only the start of a record file, and links the whole of it.
export function CutNote({ cut }: { cut: Cut }) {
  if (!cut.Cut) return null
  return (
    <p className="m-0 mt-2 text-sm text-muted-foreground">
      {cut.Name} is {cut.Whole}, and the page draws the first {cut.Shown}.{" "}
      <a
        className="underline decoration-border underline-offset-4 hover:decoration-current"
        href={cut.Raw}
      >
        Read the whole file
      </a>
      .
    </p>
  )
}
