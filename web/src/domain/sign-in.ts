// What the sign-in page offers, from what the server says of itself and of the request.

export interface SignIn {
  Wrong: boolean
  Local: boolean
  Identity: boolean
  Provider: boolean
  Desktop: boolean
  Shared: boolean
  Refused: string
}

export type ProviderWay = "browser" | "provider" | "none"

// providerWay is how the page offers the identity provider: in the desktop app, a browser to continue in, which signs in there;
// on a page, the provider itself.
export function providerWay(page: SignIn): ProviderWay {
  if (page.Desktop) return page.Provider ? "browser" : "none"
  return page.Identity ? "provider" : "none"
}

// offersSharedLink is whether the page links to the shared password's form when that form is not open: everywhere but the
// desktop app with no provider of its own.
export function offersSharedLink(page: SignIn): boolean {
  return !page.Shared && (!page.Desktop || page.Identity)
}

// signInAction is where the shared password's form posts: the desktop app's sign-in goes back to the app once it is in.
export function signInAction(page: SignIn): string {
  return page.Desktop ? "/login?desktop=1" : "/login"
}
