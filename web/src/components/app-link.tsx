import type { ComponentProps } from "react"
import { Link } from "react-router"

// The pages this app draws. A link to one of them moves within the app; a link anywhere else is a page the server still
// draws, and loads it.
const appPages = [
  /^\/admin$/,
  /^\/admin\/spending$/,
  /^\/admin\/usage(?:\/[^/]+)?$/,
  /^\/admin\/people$/,
  /^\/admin\/budgets$/,
  /^\/admin\/tokens$/,
  /^\/admin\/apps$/,
  /^\/admin\/skill-sets\/[^/]+$/,
  /^\/admin\/integrations\/[^/]+$/,
]

export function isAppPage(href: string): boolean {
  const path = href.split(/[?#]/, 1)[0]
  return appPages.some((page) => page.test(path))
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
