import type { Option, RoleChoice } from "@/domain/chat-choices"

// ChatData is what the chat page is given by /app/api/chat.
export interface ChatData {
  Ready: boolean
  Harness: string
  Model: string
  Agents: Option[] | null
  Models: Record<string, Option[]> | null
  Refused: string
  Start: { At: string; Name: string; Harness: string }
  Roles: RoleChoice[] | null
  Role: string
  NeedsModel: boolean
  Presets: { Title: string; Address: string; Auth: string }[]
  Missing: { Name: string; Title: string; Why: string; Installable: boolean }[]
  HandInstall: boolean
  FoundKeys: string[] | null
  Lacking: string[] | null
  Wall: {
    Short: string
    Words: string
    Missing: string[] | null
    Unconfirmed: boolean
  } | null
}
