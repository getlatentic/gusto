import { buttonVariants } from "@/components/ui/button"
import type { CloudPage } from "@/cloud/types"
import { cn } from "@/lib/utils"

// ContinueWithGoogle is the way in with a Google account, above the email form, where gusto cloud has a client at Google. The
// same link signs a person in or signs them up, as the account Google names has a gusto account or not.
export function ContinueWithGoogle({ page }: { page: CloudPage }) {
  if (!page.Google) return null
  return (
    <>
      <a
        href="/signin/google"
        data-testid="google"
        className={cn(
          buttonVariants({ variant: "outline" }),
          "h-11 gap-3 no-underline"
        )}
      >
        <img src="/static/brands/google.svg" alt="" className="size-[18px]" />
        Continue with Google
      </a>
      <p className="m-0 flex items-center gap-3 text-xs text-muted-foreground before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border">
        or
      </p>
    </>
  )
}
