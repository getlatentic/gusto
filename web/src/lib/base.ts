// The path this server's pages are under, when a gusto cloud serves the organization at one, such as /workspace/acme. The
// server names it in the page (internal/web/base.go); at the root of a host there is none, and it is "".
export function basePath(): string {
  return (
    document.querySelector<HTMLMetaElement>('meta[name="gusto-base"]')
      ?.content ?? ""
  )
}

// withBase is an address of this server as the browser asks for it: one of the server's own paths begins with the base path, and any
// other address, of another site or relative to the page, is as it was. The router's links add the base path themselves, so
// only an address the browser follows without the router goes through here.
export function withBase(address: string): string
export function withBase(address: string | undefined): string | undefined
export function withBase(address: string | undefined): string | undefined {
  return address?.startsWith("/") && !address.startsWith("//")
    ? basePath() + address
    : address
}

// below is a path the browser is at, as the server's own: without the base path.
export function below(path: string): string {
  const base = basePath()
  if (!base || !path.startsWith(base)) return path
  const rest = path.slice(base.length)
  return rest === "" ? "/" : rest.startsWith("/") ? rest : path
}
