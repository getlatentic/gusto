import { useState, type FormEvent } from "react"
import { PlusIcon } from "lucide-react"

import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { usePageData, useRefreshPage } from "@/lib/api"
import { postForm } from "@/lib/http"
import { ago } from "@/lib/format"

export interface TokenRow {
  ID: string
  Name: string
  May: string
  Made: string
  MadeDay: string
  By: string
  Ended: string
  LastUsed: string
  UsedAt: number
  Live: boolean
}

export interface TokensData {
  Tokens: TokenRow[]
  Offered: { Scope: string; Words: string }[]
  Refused: string
}

// A token just made: its secret is in the one answer that made it, and shown only until this page is left.
interface MadeToken {
  Made: string
  Secret: string
}

// The tokens programs call this server with, and the way to make one.
export function TokensPage() {
  const [made, setMade] = useState<MadeToken>()
  const [making, setMaking] = useState(false)
  const loaded = usePageData<TokensData>()
  const refresh = useRefreshPage()
  const page = loaded.state === "ready" ? loaded.data : undefined
  const formShown = making || (page !== undefined && page.Tokens.length === 0)
  return (
    <>
      <PageBar title="API tokens">
        {page && page.Tokens.length > 0 && !making && (
          <Button data-testid="new-token" onClick={() => setMaking(true)}>
            <PlusIcon /> New token
          </Button>
        )}
      </PageBar>
      <PageBody>
        <PageSub>What programs use to call this server</PageSub>
        <Loading loaded={loaded}>
          {(data) => (
            <div className="grid gap-4">
              {data.Refused && <Notice>{data.Refused}</Notice>}
              {made && <Secret made={made} />}
              {formShown && (
                <MakeToken
                  offered={data.Offered}
                  onMade={(token) => {
                    setMade(token)
                    setMaking(false)
                    void refresh()
                  }}
                />
              )}
              <Tokens tokens={data.Tokens} />
            </div>
          )}
        </Loading>
      </PageBody>
    </>
  )
}

function Secret({ made }: { made: MadeToken }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Token {made.Made}</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-2 text-sm">
        <p className="m-0">
          Copy it now. This page is the only place it is ever shown.
        </p>
        <code
          data-testid="token-secret"
          className="block overflow-x-auto rounded-sm bg-muted px-3 py-2 font-mono text-sm"
        >
          {made.Secret}
        </code>
        <p className="m-0 text-muted-foreground">
          Set it as <code className="font-mono">GUSTO_TOKEN</code> where{" "}
          <code className="font-mono">gusto submit</code> runs, or send it as{" "}
          <code className="font-mono">Authorization: Bearer</code>.
        </p>
      </CardContent>
    </Card>
  )
}

// MakeToken sends the form itself, so the secret it answers stays in this page and is never in an address.
function MakeToken({
  offered,
  onMade,
}: {
  offered: TokensData["Offered"]
  onMade: (token: MadeToken) => void
}) {
  const [refused, setRefused] = useState("")
  const [sending, setSending] = useState(false)
  async function send(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSending(true)
    try {
      onMade(
        await postForm<MadeToken>(
          "/admin/tokens",
          new FormData(event.currentTarget)
        )
      )
    } catch (err) {
      setRefused(err instanceof Error ? err.message : String(err))
    } finally {
      setSending(false)
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Make a token</CardTitle>
      </CardHeader>
      <CardContent>
        <form className="grid max-w-prose gap-4" onSubmit={send}>
          {refused && <Notice>{refused}</Notice>}
          <div className="grid gap-2">
            <Label htmlFor="token-name">Name</Label>
            <Input
              id="token-name"
              data-testid="token-name"
              name="name"
              placeholder="ci-nightly"
              pattern="[a-z0-9][a-z0-9\-]*"
              required
            />
          </div>
          <fieldset className="m-0 grid gap-2 border-0 p-0">
            <legend className="mb-2 text-sm font-medium">It may</legend>
            {offered.map((scope) => (
              <Label key={scope.Scope} className="font-normal">
                <Checkbox
                  name="scope"
                  value={scope.Scope}
                  defaultChecked={scope.Scope === "submit"}
                />
                {scope.Words}
              </Label>
            ))}
          </fieldset>
          <div>
            <Button type="submit" disabled={sending} data-testid="make-token">
              Make token
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

function Tokens({ tokens }: { tokens: TokenRow[] }) {
  if (tokens.length === 0) {
    return (
      <p className="m-0 text-sm text-muted-foreground">
        No token has been made yet.
      </p>
    )
  }
  return (
    <>
      <Card className="@container gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead className="@max-xl:hidden">Can</TableHead>
              <TableHead className="@max-md:hidden">Made</TableHead>
              <TableHead>Last used</TableHead>
              <TableHead>
                <span className="sr-only">Revoke</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {tokens.map((token) => (
              <TableRow key={token.ID} data-testid="token-row">
                <TableCell className="font-medium whitespace-nowrap">
                  {token.Name}
                </TableCell>
                <TableCell className="@max-xl:hidden">
                  <span className="flex flex-wrap gap-1">
                    {token.May.split(", ").map((may) => (
                      <Badge key={may} variant="secondary">
                        {may}
                      </Badge>
                    ))}
                  </span>
                </TableCell>
                <TableCell
                  className="whitespace-nowrap @max-md:hidden"
                  title={token.Made}
                >
                  {token.MadeDay}{" "}
                  <span className="text-xs text-muted-foreground">
                    by {token.By}
                  </span>
                </TableCell>
                <TableCell
                  className="whitespace-nowrap text-muted-foreground"
                  title={token.LastUsed || undefined}
                >
                  {token.UsedAt ? ago(token.UsedAt) : "Never"}
                </TableCell>
                <TableCell className="text-right">
                  {token.Live ? (
                    <form
                      method="post"
                      action={`/admin/tokens/${token.ID}/revoke`}
                      className="m-0"
                    >
                      <Button variant="destructive" size="sm" type="submit">
                        Revoke
                      </Button>
                    </form>
                  ) : (
                    <span className="text-muted-foreground" title={token.Ended}>
                      Revoked
                    </span>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
      <p className="m-0 text-xs text-muted-foreground">
        A token is shown once, when it is made. Revoking stops it at once, and
        the shared password no longer opens the API.
      </p>
    </>
  )
}
