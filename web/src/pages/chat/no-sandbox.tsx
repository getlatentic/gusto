import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import type { ChatData } from "@/pages/chat/types"
import { withBase } from "@/lib/base"

// NoSandbox asks a person to agree, once, before agents run on a computer that cannot put them in a sandbox.
export function NoSandbox({ wall }: { wall: NonNullable<ChatData["Wall"]> }) {
  return (
    <Card className="w-full max-w-136 p-6">
      <form
        method="post"
        action={withBase("/setup/no-sandbox")}
        className="grid gap-6"
      >
        <div className="grid gap-3">
          <p className="m-0">{wall.Words}</p>
          <p className="m-0 text-sm text-muted-foreground">
            An agent works in the folder of its conversation, and that is the
            only folder it is pointed at. Nothing stops it going further than
            that, and nothing here would undo it.
          </p>
        </div>
        {(wall.Missing ?? []).length > 0 && (
          <section
            aria-labelledby="why-no-sandbox"
            className="grid gap-2 border-t pt-4"
          >
            <h2
              id="why-no-sandbox"
              className="m-0 text-sm font-medium text-muted-foreground"
            >
              Why there is no sandbox
            </h2>
            <ul className="m-0 grid list-disc gap-1 pl-5 text-sm text-muted-foreground">
              {wall.Missing!.map((why) => (
                <li key={why}>{why}.</li>
              ))}
            </ul>
          </section>
        )}
        <div>
          <Button type="submit">
            I understand, start agents with no sandbox
          </Button>
        </div>
      </form>
    </Card>
  )
}
