import { Loading } from "@/components/controls"
import { Button, buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import {
  offersSharedLink,
  providerWay,
  signInAction,
  type SignIn,
} from "@/domain/sign-in"
import { usePageData } from "@/lib/api"
import { cn } from "@/lib/utils"
import { ProviderIcon } from "@/pages/sign-in/provider-icon"
import { PublicPanel } from "@/pages/sign-in/public-panel"

// SignInPage is where a person signs in: with the organization's identity provider, or the shared password where there is one.
// The password is only ever posted, never put in an address. In the desktop app it continues in the person's browser.
export function SignInPage() {
  const loaded = usePageData<{ SignIn: SignIn }>()
  return (
    <PublicPanel>
      <p className="m-0 flex items-center gap-3 font-heading text-lg font-semibold">
        <img className="h-auto w-7" src="/static/mark.svg" alt="" /> gusto
      </p>
      <Loading loaded={loaded}>{(page) => <Ways page={page.SignIn} />}</Loading>
    </PublicPanel>
  )
}

function Ways({ page }: { page: SignIn }) {
  const way = providerWay(page)
  return (
    <div className="grid gap-6">
      {page.Refused && <p className="m-0 text-xs text-bad">{page.Refused}</p>}
      {page.Wrong && (
        <p className="m-0 text-sm text-bad" role="alert">
          {page.Local ? (
            <>
              This copy of gusto opens with a link, not a password. Run{" "}
              <code>gusto open</code> for a new one.
            </>
          ) : (
            "That password is not right. Check it and try again."
          )}
        </p>
      )}
      {way === "browser" && (
        <>
          <a
            className={cn(buttonVariants(), "h-11")}
            href="/auth/desktop/begin"
          >
            Continue in your browser
          </a>
          <p className="m-0 text-center text-xs text-muted-foreground">
            Your browser opens to sign you in, and brings you back here.
          </p>
        </>
      )}
      {way === "provider" && (
        <a
          className={cn(buttonVariants(), "h-11")}
          href="/auth/start"
          data-testid="provider-sign-in"
        >
          <ProviderIcon />
          Continue with Google
        </a>
      )}
      {page.Shared && <SharedPassword page={page} />}
      {offersSharedLink(page) && (
        <p className="m-0 text-center text-xs text-muted-foreground">
          <a
            className="no-underline hover:text-foreground"
            href="/login?shared"
            data-testid="use-shared-password"
          >
            Use the shared password
          </a>
        </p>
      )}
    </div>
  )
}

// SharedPassword is the form of the shared password, the way in where there is no provider, and when the provider is not.
function SharedPassword({ page }: { page: SignIn }) {
  return (
    <>
      {page.Provider && <Separator />}
      <form className="grid gap-3" method="post" action={signInAction(page)}>
        <Input
          className="h-11"
          type="password"
          name="password"
          aria-label="Shared password"
          placeholder="Shared password"
          autoComplete="current-password"
          autoFocus={!page.Provider}
          required
          data-testid="shared-password"
        />
        <Button
          className="h-11"
          variant={page.Provider ? "ghost" : "default"}
          type="submit"
          data-testid="sign-in"
        >
          Sign in
        </Button>
      </form>
    </>
  )
}
