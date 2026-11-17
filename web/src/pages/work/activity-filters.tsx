import type { FormEvent } from "react"
import { useNavigate } from "react-router"

import { Input } from "@/components/ui/input"
import {
  offersAgentTypes,
  searchOf,
  showsFilters,
  sortChosen,
} from "@/domain/work"
import { FormSelect } from "@/components/form-select"
import type { Board } from "@/pages/work/types"
import { withBase } from "@/lib/base"

// ActivityFilters narrow the list: a search and a person when the index is read, an agent type, and an order. The choices send
// the form as soon as one is made, and the address the form makes is the page's, so the list is read again from it.
export function ActivityFilters({ board }: { board: Board }) {
  const navigate = useNavigate()
  if (!showsFilters(board)) return null
  const index = board.Index
  const search = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const fields = [...new FormData(event.currentTarget)].map(
      ([name, value]) => [name, String(value)] as [string, string]
    )
    navigate(searchOf("/jobs", fields))
  }
  // Each is drawn again from what the address says, once the address changes.
  const key = JSON.stringify([
    board.State,
    board.Harness,
    index?.Query,
    index?.Person,
    index?.Sort,
  ])
  return (
    <form
      key={key}
      method="get"
      action={withBase("/jobs")}
      onSubmit={search}
      data-testid="activity-filters"
      className="mb-5 flex flex-wrap items-center gap-2 text-sm"
    >
      {board.State && <input type="hidden" name="state" value={board.State} />}
      {index && (
        <label className="min-w-48 flex-1">
          <span className="sr-only">Search</span>
          <Input
            type="search"
            name="q"
            defaultValue={index.Query}
            placeholder="Search tasks"
          />
        </label>
      )}
      {index?.Everyone && (
        <label className="w-36">
          <span className="sr-only">Person</span>
          <Input
            type="text"
            name="person"
            defaultValue={index.Person}
            placeholder="Person"
          />
        </label>
      )}
      {offersAgentTypes(board) && (
        <FormSelect
          name="agent_type"
          label="Agent type"
          value={board.Harness}
          sendOnChoose
          className="w-auto"
          testId="agent-type"
          groups={[
            {
              choices: [
                { value: "", label: "Every agent" },
                ...(board.Harnesses ?? []).map((name) => ({
                  value: name,
                  label: name,
                })),
              ],
            },
          ]}
        />
      )}
      {index && (
        <FormSelect
          name="sort"
          label="Order"
          value={sortChosen(index.Sort)}
          sendOnChoose
          className="w-auto"
          testId="sort"
          groups={[
            {
              choices: index.Sorts.map((sort) => ({
                value: sort.Name,
                label: sort.Label,
              })),
            },
          ]}
        />
      )}
    </form>
  )
}
