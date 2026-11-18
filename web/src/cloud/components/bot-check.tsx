import { useEffect } from "react"

const turnstileScript = "https://challenges.cloudflare.com/turnstile/v0/api.js"

// BotCheck is Cloudflare's check that a person is sending the form. Its script draws the check into the box it finds, so the
// script is added once the box is on the page.
export function BotCheck({ site }: { site: string }) {
  useEffect(() => {
    if (document.querySelector(`script[src="${turnstileScript}"]`)) return
    const script = document.createElement("script")
    script.src = turnstileScript
    script.async = true
    document.head.append(script)
  }, [])
  return (
    <div className="cf-turnstile" data-size="flexible" data-sitekey={site} />
  )
}
