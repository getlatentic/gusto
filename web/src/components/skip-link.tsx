// SkipLink is the first thing the keyboard reaches on every page: it goes past the side list, straight to the page.
export function SkipLink() {
  return (
    <a
      href="#main"
      className="absolute top-3 left-3 z-100 -translate-y-[200%] rounded-sm bg-surface px-4 py-2 text-foreground shadow-md focus:translate-y-0"
    >
      Skip to the page
    </a>
  )
}
