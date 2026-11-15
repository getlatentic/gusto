import { useState } from "react"
import { CheckIcon, SearchIcon } from "lucide-react"

import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Toggle } from "@/components/ui/toggle"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { narrowRoles, pickedRole, type Role } from "@/domain/roles"
import { RoleDetail } from "@/pages/setups/role-detail"

// RolePicker is the built-in presets: a list narrowed by a search and a field of work, and the one picked beside it. On a narrow
// window the list is a menu of them all.
export function RolePicker({
  roles,
  fields,
}: {
  roles: Role[]
  fields: string[]
}) {
  const [chosen, setChosen] = useState(roles[0]?.Name ?? "")
  const [field, setField] = useState("")
  const [search, setSearch] = useState("")
  const shown = narrowRoles(roles, field, search)
  const picked = pickedRole(roles, shown, chosen)
  return (
    <div className="mb-8 flex flex-wrap items-start gap-6 max-md:flex-col max-md:items-stretch max-md:gap-4">
      <div className="grid max-w-[26rem] min-w-0 flex-[1_1_21rem] gap-3 max-md:max-w-none max-md:flex-none">
        <NarrowMenu roles={roles} picked={picked} onPick={setChosen} />
        <InputGroup className="h-10 max-md:hidden">
          <InputGroupAddon>
            <SearchIcon />
          </InputGroupAddon>
          <InputGroupInput
            type="search"
            aria-label="Search presets"
            placeholder="Search built-in presets"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            data-testid="role-search"
          />
        </InputGroup>
        <ToggleGroup
          variant="outline"
          size="sm"
          aria-label="Field of work"
          className="flex-wrap max-md:hidden"
          value={[field]}
          onValueChange={(next) => setField(next[0] ?? "")}
        >
          {["", ...fields].map((each) => (
            <ToggleGroupItem
              key={each}
              value={each}
              className="rounded-full px-3 text-xs"
            >
              {each || "All"}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <RoleList shown={shown} picked={picked} onPick={setChosen} />
      </div>
      <div className="min-w-0 flex-[2_1_28rem] max-md:w-full max-md:flex-none">
        {picked && <RoleDetail role={picked} />}
      </div>
    </div>
  )
}

function RoleList({
  shown,
  picked,
  onPick,
}: {
  shown: Role[]
  picked?: Role
  onPick: (name: string) => void
}) {
  if (shown.length === 0) {
    return (
      <p className="m-3 text-sm text-muted-foreground max-md:hidden">
        No role matches.
      </p>
    )
  }
  return (
    <ul className="m-0 grid list-none gap-1 p-0 max-md:hidden">
      {shown.map((role) => {
        const pressed = role.Name === picked?.Name
        return (
          <li key={role.Name}>
            <Toggle
              pressed={pressed}
              onPressedChange={() => onPick(role.Name)}
              data-testid="pick-role"
              className="grid h-auto w-full justify-stretch gap-1 px-4 py-3 text-left whitespace-normal aria-pressed:bg-card aria-pressed:shadow-sm aria-pressed:ring-1 aria-pressed:ring-foreground/10"
            >
              <span className="flex items-center justify-between gap-3">
                <strong className="inline-flex items-center gap-2">
                  {role.Title}
                  {pressed && (
                    <CheckIcon
                      className="size-4 text-primary"
                      aria-hidden="true"
                    />
                  )}
                </strong>
                <span className="text-xs text-muted-foreground">
                  {role.Industry}
                </span>
              </span>
              <span className="text-sm text-muted-foreground">
                {role.Summary}
              </span>
            </Toggle>
          </li>
        )
      })}
    </ul>
  )
}

// NarrowMenu is every built-in preset as one menu, for a window too narrow for the list beside the one picked.
function NarrowMenu({
  roles,
  picked,
  onPick,
}: {
  roles: Role[]
  picked?: Role
  onPick: (name: string) => void
}) {
  return (
    <div className="hidden gap-2 max-md:grid">
      <Label htmlFor="role-select" className="text-xs text-muted-foreground">
        Built-in preset
      </Label>
      <Select
        items={roles.map((role) => ({
          value: role.Name,
          label: `${role.Title} · ${role.Industry}`,
        }))}
        value={picked?.Name ?? ""}
        onValueChange={(next) => onPick(String(next ?? ""))}
      >
        <SelectTrigger id="role-select" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {roles.map((role) => (
            <SelectItem key={role.Name} value={role.Name}>
              {role.Title} · {role.Industry}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
