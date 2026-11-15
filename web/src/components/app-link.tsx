import type { ComponentProps } from "react"
import { Link } from "react-router"

// The pages this app draws. A link to one of them moves within the app; a link anywhere else is a page the server still
// draws, and loads it.
const appPages =
  /^\/admin(?:\/spending|\/usage(?:\/[^?#]*)?|\/people)?(?:[?#].*)?$/

export function isAppPage(href: string): boolean {
  return appPages.test(href)
}

export function AppLink({
  href,
  ...props
}: ComponentProps<"a"> & { href: string }) {
  return isAppPage(href) ? (
    <Link to={href} {...props} />
  ) : (
    <a href={href} {...props} />
  )
}
