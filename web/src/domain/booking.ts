// A booking service's calendar framed on gusto's page tells the page how tall it is, so the frame fits it and shows none of
// the service's own background. Calendly does so when the address names the page's domain.

const calendly = "https://calendly.com"

// framedAt is the address to frame a calendar from, on a page at host: Calendly's says the domain it is embedded in, which
// Calendly needs to send the page its height. Another service's is left as it is.
export function framedAt(frame: string, host: string): string {
  const address = new URL(frame)
  if (address.origin === calendly)
    address.searchParams.set("embed_domain", host)
  return address.toString()
}

// calendarHeight is the height in pixels a framed calendar says it is, from a message it posted to the page, or null for any
// other message or sender.
export function calendarHeight(origin: string, data: unknown): number | null {
  if (origin !== calendly || typeof data !== "object" || data === null)
    return null
  const said = data as { event?: unknown; payload?: { height?: unknown } }
  if (
    said.event !== "calendly.page_height" ||
    typeof said.payload?.height !== "string"
  )
    return null
  const height = Number.parseFloat(said.payload.height)
  return Number.isFinite(height) && height > 0 ? height : null
}
