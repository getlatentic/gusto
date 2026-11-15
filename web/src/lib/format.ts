// The words and figures the server's templates write, written the same way here, so a page reads the same in either.

// usd is an amount as a headline shows it: cents once there are dollars, and fractions of a cent until then.
export function usd(amount: number): string {
  return amount >= 1 || amount === 0
    ? `$${amount.toFixed(2)}`
    : `$${amount.toFixed(4)}`
}

// compact is a count of tokens at a glance: 950, 4.0k, 12k, 1.2M.
export function compact(count: number): string {
  if (count >= 1_000_000) return `${(count / 1e6).toFixed(1)}M`
  if (count >= 10_000) return `${(count / 1e3).toFixed(0)}k`
  if (count >= 1_000) return `${(count / 1e3).toFixed(1)}k`
  return String(count)
}

// plural is the word for one thing, or with an s for any other count, as the templates' plural writes it.
export function plural(count: number, one: string): string {
  return count === 1 ? one : `${one}s`
}

export function percent(part: number, whole: number): string {
  return whole === 0 ? "0%" : `${Math.round((100 * part) / whole)}%`
}

const months = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
]
const two = (n: number) => String(n).padStart(2, "0")

// when is a moment as "2 Jan 15:04".
export function when(unix: number): string {
  if (!unix) return ""
  const d = new Date(unix * 1000)
  return `${d.getDate()} ${months[d.getMonth()]} ${two(d.getHours())}:${two(d.getMinutes())}`
}

// ago is how long ago, at a glance, with the date once it is more than a day old.
export function ago(unix: number, now = Date.now()): string {
  if (!unix) return ""
  const since = now / 1000 - unix
  if (since < 60) return "just now"
  if (since < 3600) return `${Math.floor(since / 60)}m ago`
  if (since < 86400) return `${Math.floor(since / 3600)}h ago`
  const d = new Date(unix * 1000)
  return `${d.getDate()} ${months[d.getMonth()]}`
}

export function initial(name: string): string {
  const first = [...name][0]
  return first ? first.toUpperCase() : "·"
}

// isoDay is a date as an <input type=date> and the server's from and to write it.
export function isoDay(d: Date): string {
  return `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())}`
}

// dayFromISO is the date an ISO day names, at local midnight, or undefined for none.
export function dayFromISO(text: string | null): Date | undefined {
  const m = text ? /^(\d{4})-(\d{2})-(\d{2})$/.exec(text) : null
  return m ? new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3])) : undefined
}

// bytes is a file's size at a glance: 512 B, 12 KB, 3.4 MB.
export function bytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1_048_576) return `${Math.round(n / 1024)} KB`
  return `${(n / 1_048_576).toFixed(1)} MB`
}

// shortWhen is when something changed: the time when it was today, the day when it was this year, and the date before that.
export function shortWhen(unix: number, now = new Date()): string {
  if (!unix) return ""
  const then = new Date(unix * 1000)
  if (then.toDateString() === now.toDateString()) {
    return then.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })
  }
  return then.toLocaleDateString([], {
    day: "numeric",
    month: "short",
    year: then.getFullYear() === now.getFullYear() ? undefined : "numeric",
  })
}
