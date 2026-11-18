import {
  Brand,
  CloudCard,
  Heading,
  Quiet,
  Refusal,
  TextLink,
} from "@/cloud/components/cloud-card"
import { Field, Submit } from "@/cloud/components/forms"
import { ContinueWithGoogle } from "@/cloud/components/google-sign-in"
import type { CloudPage } from "@/cloud/types"

// SignUpPage makes an organization from an email address: the company's, or a workspace of the person's own. It asks for a
// name only when the company has an organization already, and offers a sign-in to an address that already has an account.
export function SignUpPage({ page }: { page: CloudPage }) {
  return (
    <CloudCard>
      <Brand />
      {page.Taken ? <WelcomeBack page={page} /> : <GetStarted page={page} />}
    </CloudCard>
  )
}

function WelcomeBack({ page }: { page: CloudPage }) {
  return (
    <>
      <Heading>Welcome back</Heading>
      <p className="m-0">
        <strong className="font-semibold">{page.Email}</strong> already has a
        gusto account.
      </p>
      <form method="post" action="/signin" className="grid">
        <input type="hidden" name="email" value={page.Email} />
        <Submit data-testid="send-code">Email me a sign-in code</Submit>
      </form>
      <Quiet className="text-center">
        <TextLink href="/signup">Use a different email</TextLink>
      </Quiet>
    </>
  )
}

function GetStarted({ page }: { page: CloudPage }) {
  return (
    <>
      <Heading>Get started</Heading>
      {page.Error ? (
        <Refusal>{page.Error}</Refusal>
      ) : (
        <Quiet data-testid="sign-up-ask">
          {page.Ask ||
            "Enter your email and gusto sets up a workspace for your team."}
        </Quiet>
      )}
      {!page.More && <ContinueWithGoogle page={page} />}
      <form className="grid gap-3" method="post" action="/signup">
        <Field
          label={page.WorkOnly ? "Work email" : "Email"}
          type="email"
          name="email"
          defaultValue={page.Email}
          autoComplete="email"
          autoFocus={!page.Ask}
          required
          data-testid="email"
        />
        {page.More && (
          <Field
            label="Organization name"
            type="text"
            name="org"
            defaultValue={page.OrgName}
            maxLength={80}
            autoComplete="organization"
            autoFocus={Boolean(page.Ask)}
            required
            data-testid="org-name"
          />
        )}
        <Submit data-testid="sign-up">Continue</Submit>
      </form>
      <Quiet className="text-center">
        Already use gusto? <TextLink href="/signin">Sign in</TextLink>
      </Quiet>
    </>
  )
}
