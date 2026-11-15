import {
  Brand,
  CloudCard,
  Heading,
  Quiet,
  Refusal,
  TextLink,
} from "@/cloud/components/cloud-card"
import { Field, Submit } from "@/cloud/components/forms"
import type { CloudPage } from "@/cloud/types"

// SignInPage asks for an email address and mails a code to it; once sent, it takes the code.
export function SignInPage({ page }: { page: CloudPage }) {
  return (
    <CloudCard>
      <Brand />
      {page.Sent ? <EnterCode page={page} /> : <AskForCode page={page} />}
    </CloudCard>
  )
}

function AskForCode({ page }: { page: CloudPage }) {
  return (
    <>
      <Heading>Sign in</Heading>
      <Refusal>{page.Error}</Refusal>
      <form className="grid gap-3" method="post" action="/signin">
        <Field
          label="Email"
          type="email"
          name="email"
          defaultValue={page.Email}
          autoComplete="email"
          autoFocus
          required
          data-testid="email"
        />
        <Submit data-testid="send-code">Email me a sign-in code</Submit>
      </form>
      <Quiet className="text-center">
        New to gusto? <TextLink href="/signup">Get started</TextLink>
      </Quiet>
    </>
  )
}

function EnterCode({ page }: { page: CloudPage }) {
  return (
    <>
      <Heading>Check your email</Heading>
      <p className="m-0">
        If <strong className="font-semibold">{page.Email}</strong> has a gusto
        account, we sent it a 6-digit code. Enter it here, or open the link in
        the same email.
      </p>
      <Refusal>{page.Error}</Refusal>
      <form className="grid gap-3" method="post" action="/signin/code">
        <input type="hidden" name="email" value={page.Email} />
        <Field
          label="Sign-in code"
          className="text-center font-mono text-xl tracking-[0.3em]"
          type="text"
          name="code"
          inputMode="numeric"
          autoComplete="one-time-code"
          pattern="[0-9 \-]{6,8}"
          maxLength={8}
          autoFocus
          required
          data-testid="code"
        />
        <Submit data-testid="sign-in">Sign in</Submit>
      </form>
      <Quiet>
        The code and the link work once, for 20 minutes. Not in your inbox?
        Check spam, or ask again in a few minutes.
      </Quiet>
      <Quiet className="text-center">
        <TextLink href="/signin">Use a different email</TextLink>
      </Quiet>
    </>
  )
}
