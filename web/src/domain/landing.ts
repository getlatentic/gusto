// offerTerms is the line under the landing page's first call to start: what a person gets for nothing when they sign up.
export function offerTerms(startingCredit: number): string {
  return startingCredit > 0
    ? `Free to start with $${startingCredit.toFixed(0)} of model credit. No credit card.`
    : "Free to start. No credit card."
}
