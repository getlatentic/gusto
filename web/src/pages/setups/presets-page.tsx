import { Trash2Icon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody } from "@/components/page"
import { Button, buttonVariants } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { usePageData } from "@/lib/api"
import { RolePicker } from "@/pages/setups/role-picker"
import type { PresetRow, PresetsData } from "@/pages/setups/types"
import { withBase } from "@/lib/base"

// PresetsPage is where a new agent is chosen: the person's own presets, each ready-made kind of work, or a skill set of their own.
export function PresetsPage() {
  const loaded = usePageData<PresetsData>()
  return (
    <>
      <PageBar title="Presets" />
      <PageBody>
        <Loading loaded={loaded}>
          {(page) => (
            <>
              {page.Refused && <Notice>{page.Refused}</Notice>}
              {page.Presets.length > 0 && (
                <YourPresets presets={page.Presets} />
              )}
              <h2 className="m-0 mb-3 text-sm font-semibold">
                Built-in presets
              </h2>
              <RolePicker roles={page.Roles} fields={page.Fields} />
              <p className="m-0 text-sm">
                <AppLink
                  href="/setups"
                  className="text-muted-foreground hover:text-foreground"
                >
                  Build your own skill set
                </AppLink>
              </p>
            </>
          )}
        </Loading>
      </PageBody>
    </>
  )
}

// YourPresets are what the person saved from the chat, each to start a chat with again or to remove.
function YourPresets({ presets }: { presets: PresetRow[] }) {
  return (
    <section className="mb-8 grid gap-3" aria-labelledby="your-presets">
      <h2 id="your-presets" className="m-0 text-sm font-semibold">
        Your presets
      </h2>
      <Card className="gap-0 py-0">
        <ul className="m-0 list-none divide-y p-0" data-testid="your-presets">
          {presets.map((preset) => (
            <li
              key={preset.ID}
              className="grid min-h-16 grid-cols-[minmax(0,1fr)_auto_2rem] items-center gap-3 py-2 pr-3 pl-5"
              data-testid="preset-row"
            >
              <span className="grid min-w-0 gap-0.5">
                <strong>{preset.Name}</strong>
                <span className="truncate text-xs text-muted-foreground">
                  {preset.Line}
                </span>
              </span>
              <AppLink
                href={`/chat?preset=${encodeURIComponent(preset.ID)}`}
                className={buttonVariants({ variant: "outline", size: "sm" })}
              >
                New chat
              </AppLink>
              <form
                method="post"
                action={withBase(
                  `/presets/${encodeURIComponent(preset.ID)}/delete`
                )}
                className="m-0"
              >
                <Button
                  type="submit"
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Remove ${preset.Name}`}
                  title="Remove"
                >
                  <Trash2Icon />
                </Button>
              </form>
            </li>
          ))}
        </ul>
      </Card>
    </section>
  )
}
