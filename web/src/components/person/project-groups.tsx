import { FolderIcon, PlusIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { sideItem } from "@/components/person/side-row"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { isOpen, type RailGroup } from "@/domain/side-list"
import { ago } from "@/lib/format"
import { cn } from "@/lib/utils"
import { FlipList } from "@/motion/flip-list"

// ProjectGroups are a person's projects, each an agent's folder with its latest conversations, and a way to start another
// conversation in it.
export function ProjectGroups({
  groups,
  here,
  onGo,
}: {
  groups: RailGroup[]
  here: string
  onGo?: () => void
}) {
  return (
    <FlipList>
      <ul className="m-0 grid list-none grid-cols-[minmax(0,1fr)] gap-px p-0">
        {groups.map((group, index) => (
          <li
            key={group.ID}
            data-flip-id={group.ID}
            className="group/proj relative min-w-0"
            data-testid="side-project"
          >
            <Collapsible defaultOpen={isOpen(group, index, here)}>
              <CollapsibleTrigger
                className="flex min-h-9 w-full cursor-pointer items-center gap-2 rounded-md border-0 bg-transparent py-1 pr-8 pl-3 text-left text-sm font-medium text-foreground hover:bg-hover [&>svg]:size-4 [&>svg]:flex-none [&>svg]:text-subtle"
                title={`${group.Name}${group.Detail ? ` · ${group.Detail}` : ""} (${group.Runs})`}
              >
                <FolderIcon aria-hidden="true" />
                <span className="grid min-w-0 flex-1">
                  <span className="truncate">{group.Name}</span>
                  {group.Detail && (
                    <span className="truncate text-xs leading-tight font-normal text-subtle">
                      {group.Detail}
                    </span>
                  )}
                </span>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <ol className="m-0 mt-0.5 mb-2 grid list-none gap-px p-0">
                  {(group.Chats ?? []).map((chat) => (
                    <li key={chat.Where} data-flip-id={chat.Where}>
                      <AppLink
                        href={chat.Where}
                        data-testid="side-row"
                        className={cn(sideItem, "pl-9")}
                        aria-current={chat.Where === here ? "page" : undefined}
                        title={chat.Name}
                        onClick={onGo}
                      >
                        <span className="min-w-0 flex-1 truncate">
                          {chat.Name}
                        </span>
                        <span className="flex-none text-xs text-subtle">
                          {ago(chat.When)}
                        </span>
                      </AppLink>
                    </li>
                  ))}
                </ol>
              </CollapsibleContent>
            </Collapsible>
            <AppLink
              href={`/chat?at=${encodeURIComponent(group.ID)}`}
              className="absolute top-1.5 right-1 grid size-6 place-items-center rounded-sm text-muted-foreground opacity-0 transition-opacity group-hover/proj:opacity-100 hover:bg-surface hover:text-foreground hover:shadow-sm focus-visible:opacity-100 [&>svg]:size-4"
              title={`New chat in ${group.Name}`}
              aria-label={`New chat in ${group.Name}`}
              onClick={onGo}
            >
              <PlusIcon />
            </AppLink>
          </li>
        ))}
      </ul>
    </FlipList>
  )
}
