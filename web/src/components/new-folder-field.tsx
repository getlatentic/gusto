import { FolderIcon } from "lucide-react"

import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

// NewFolderField names a folder to make in the place open: Enter makes it, Escape leaves it, and why it could not be made is said
// under it.
export function NewFolderField({
  top,
  problem,
  onMake,
  onCancel,
  large = false,
}: {
  top: boolean
  problem: string
  onMake: (name: string) => void
  onCancel: () => void
  large?: boolean
}) {
  return (
    <>
      <span className="flex items-center gap-3">
        <FolderIcon
          aria-hidden="true"
          className={cn(
            "flex-none text-muted-foreground",
            large ? "size-5" : "size-4"
          )}
        />
        <Input
          autoFocus
          maxLength={80}
          placeholder={top ? "New project" : "New folder"}
          aria-label="Name of the new folder"
          data-testid="new-folder-name"
          aria-invalid={Boolean(problem) || undefined}
          aria-describedby={problem ? "new-folder-problem" : undefined}
          className="h-8"
          onKeyDown={(event) => {
            event.stopPropagation()
            if (event.key === "Enter" && event.currentTarget.value.trim())
              onMake(event.currentTarget.value.trim())
            if (event.key === "Escape") onCancel()
          }}
        />
      </span>
      {problem && (
        <span
          id="new-folder-problem"
          className={cn("block text-xs text-bad", large ? "ml-8" : "ml-7")}
        >
          {problem}
        </span>
      )}
    </>
  )
}
