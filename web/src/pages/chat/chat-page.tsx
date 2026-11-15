import { Loading } from "@/components/controls"
import { Notice, PageBar } from "@/components/page"
import { usePageData } from "@/lib/api"
import { ChatStart } from "@/pages/chat/chat-start"
import { FirstModel } from "@/pages/chat/first-model"
import { NoSandbox } from "@/pages/chat/no-sandbox"
import type { ChatData } from "@/pages/chat/types"

// headingOf is what the page asks: to add a model first, to agree to no sandbox, or what we are working on.
function headingOf(page: ChatData): string {
  if (page.NeedsModel) return "Add the model you use"
  if (page.Wall?.Unconfirmed) return "Agents will run with no sandbox"
  return "What are we working on?"
}

// ChatPage is where a person starts talking to an agent.
export function ChatPage() {
  const loaded = usePageData<ChatData>()
  return (
    <>
      <PageBar title="New chat" />
      <section
        aria-labelledby="chat-heading"
        className="grid min-h-0 flex-1 content-center justify-items-center gap-6 overflow-y-auto p-6 max-sm:flex max-sm:flex-col max-sm:items-center max-sm:gap-3 max-sm:p-3 max-sm:pb-[max(0.75rem,env(safe-area-inset-bottom))]"
      >
        <Loading loaded={loaded}>{(page) => <Chat page={page} />}</Loading>
      </section>
    </>
  )
}

function Chat({ page }: { page: ChatData }) {
  return (
    <div className="grid w-full justify-items-center gap-6 max-sm:gap-3">
      <h1
        id="chat-heading"
        className="m-0 text-center font-heading text-2xl font-semibold max-sm:my-auto"
      >
        {headingOf(page)}
      </h1>
      {page.Refused && (
        <p
          id="refused"
          role="alert"
          className="m-0 w-full max-w-208 rounded-md bg-bad-wash px-4 py-3 text-sm text-bad"
        >
          {page.Refused}
        </p>
      )}
      {(page.Lacking ?? []).map((lack) => (
        <Notice key={lack} tone="warn">
          {lack}
        </Notice>
      ))}
      {page.NeedsModel ? (
        <FirstModel page={page} />
      ) : !page.Ready ? (
        <p className="m-0 text-muted-foreground">
          This deployment cannot start an agent yet.
        </p>
      ) : page.Wall?.Unconfirmed ? (
        <NoSandbox wall={page.Wall} />
      ) : (
        <ChatStart page={page} />
      )}
    </div>
  )
}
