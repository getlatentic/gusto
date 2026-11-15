// What the landing page offers depends on how the front door is set up: a demo anybody may start, or sign-up with some free credit
// on gusto's models, and a call to book where there is somewhere to book it.

export interface Offer {
  DemoPublic: boolean
  DemoLeftLong: string
  StartingCredit: number
}

export const startHref = (offer: Offer) =>
  offer.DemoPublic ? "/demo" : "/signup"

export const startLabel = (offer: Offer) =>
  offer.DemoPublic ? "Try it now" : "Start free"

export const bookLabel = (offer: Offer) =>
  offer.DemoPublic ? "Talk to us" : "Book a demo"

// offerTerms is the line under the first call to start: what it costs, and what a visitor gets for nothing.
export function offerTerms(offer: Offer): string {
  if (offer.DemoPublic) {
    return `Free. No credit card. A ${offer.DemoLeftLong} isolated demo, deleted when it ends.`
  }
  return `Free to start, with $${offer.StartingCredit.toFixed(0)} of gusto's models. Then add your own model key. No credit card.`
}
