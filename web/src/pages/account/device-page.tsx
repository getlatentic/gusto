import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { usePageData } from "@/lib/api"
import { ComputersTable } from "@/pages/account/computers-table"
import { ConfirmForm } from "@/pages/account/confirm-form"
import type { DeviceData } from "@/pages/account/types"
import {
  useAccountForm,
  type AccountForm,
} from "@/pages/account/use-account-form"

// DevicePage is where a person approves a computer that asks to sign in as them, by the code it shows, and ends the ones they
// approved, or every sign-in of theirs at once.
export function DevicePage() {
  const loaded = usePageData<DeviceData>()
  return (
    <>
      <PageBar title="Your computers" />
      <PageBody className="max-w-3xl">
        <Loading loaded={loaded}>{(page) => <Devices page={page} />}</Loading>
      </PageBody>
    </>
  )
}

function Devices({ page }: { page: DeviceData }) {
  const form = useAccountForm()
  const refused = form.failed || page.Refused
  return (
    <>
      {refused && <Notice>{refused}</Notice>}
      <PageSub>
        A computer you approve acts as you, on your own work only. You can end
        it here at any time.
      </PageSub>
      <div className="grid gap-4">
        {page.Approved && (
          <p
            role="status"
            className="m-0 rounded-md bg-ok-wash px-4 py-3 text-sm text-ok"
            data-testid="device-approved"
          >
            Approved <strong>{page.Approved}</strong>. Go back to the computer:
            it signs in on its own.
          </p>
        )}
        {page.Owner && (
          <p className="m-0 text-sm text-muted-foreground">
            This server has no sign-in provider, so the shared password is its
            one owner. A computer you approve acts as that owner.
          </p>
        )}
        {page.Shared ? (
          <p className="m-0 text-sm text-muted-foreground">
            The shared sign-in is not one person, so it has no computers. Sign
            in with your own account.
          </p>
        ) : (
          <OwnComputers page={page} form={form} />
        )}
      </div>
    </>
  )
}

function OwnComputers({ page, form }: { page: DeviceData; form: AccountForm }) {
  return (
    <>
      <Card>
        <CardContent>
          {/* Drawn again for each code the page is opened with, so a code just approved leaves the field. */}
          <form
            key={page.Code + page.Approved}
            method="post"
            action="/device"
            onSubmit={form.submit}
            className="m-0 flex flex-wrap items-end gap-3"
            data-testid="approve-form"
          >
            <div className="grid min-w-48 flex-1 gap-2">
              <Label htmlFor="code">The code your computer shows</Label>
              <Input
                id="code"
                name="code"
                defaultValue={page.Code}
                placeholder="BCDF-GHJK"
                autoComplete="off"
                required
                className="font-mono tracking-widest uppercase"
              />
            </div>
            <Button type="submit" disabled={form.sending}>
              Approve
            </Button>
          </form>
        </CardContent>
      </Card>

      <section aria-labelledby="approved-title" className="mt-4 grid gap-3">
        <h2 id="approved-title" className="m-0 text-base font-semibold">
          Approved
        </h2>
        {page.Tokens.length > 0 ? (
          <Card className="@container gap-0 overflow-hidden py-0">
            <ComputersTable computers={page.Tokens} form={form} />
          </Card>
        ) : (
          <p className="m-0 text-sm text-muted-foreground">None yet.</p>
        )}
      </section>

      <section aria-labelledby="everywhere-title" className="mt-4 grid gap-2">
        <h2 id="everywhere-title" className="m-0 text-base font-semibold">
          Sign out everywhere
        </h2>
        <p className="m-0 text-sm text-muted-foreground">
          Ends your sign-in in every browser and in the app, and every computer
          above. Use it when a computer is lost.
        </p>
        {/* Signing out leads to the sign-in page, so this form goes as the browser sends it. */}
        <ConfirmForm
          action="/device/sign-out-everywhere"
          label="Sign out everywhere"
          question="Sign out everywhere?"
          consequence="Every browser and the app sign in again, and every computer you approved is ended."
          send={(sent) => sent.submit()}
          testId="sign-out-everywhere"
        />
      </section>
    </>
  )
}
