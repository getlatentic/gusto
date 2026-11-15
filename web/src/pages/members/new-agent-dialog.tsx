import { useState } from "react"
import { CheckIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { kindNotes } from "@/domain/members"
import { cn } from "@/lib/utils"
import { AgentFace } from "@/pages/members/faces"
import { FormSelect } from "@/pages/members/form-select"
import type { Kind, MembersData } from "@/pages/members/types"
import { useMembersForm } from "@/pages/members/use-members-form"

const alert = "m-0 rounded-md bg-bad-wash px-3 py-2 text-sm text-bad"

// NewAgentDialog makes an agent: its name and specialty, the agent type it runs in, chosen once, and its model. An agent type
// this computer lacks is installed from here, on a person's own gusto, and the dialog opens again once it is.
export function NewAgentDialog({
  page,
  open,
  onClose,
}: {
  page: MembersData
  open: boolean
  onClose: () => void
}) {
  const make = useMembersForm()
  const install = useMembersForm()
  const [installing, setInstalling] = useState("")
  const offer = page.NewAgent
  const installable = offer.Kinds.filter((kind) => kind.Installable)
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent
        className="max-h-[calc(100dvh-4rem)] overflow-y-auto sm:max-w-120"
        data-testid="new-agent-dialog"
      >
        <form
          method="post"
          action="/members/agents"
          onSubmit={make.submit}
          className="grid gap-5"
          data-testid="new-agent-form"
        >
          <DialogHeader>
            <DialogTitle>New agent</DialogTitle>
            <DialogDescription>
              It starts private: only you can work with it until you share it,
              and it is in no room until you add it to one.
            </DialogDescription>
          </DialogHeader>
          {page.Refused && (
            <p role="alert" className={alert}>
              {page.Refused}
            </p>
          )}
          {(make.failed || install.failed) && (
            <p role="alert" className={alert}>
              {make.failed || install.failed}
            </p>
          )}
          <NameFields />
          <fieldset className="m-0 grid gap-1.5 border-0 p-0">
            <legend className="mb-1.5 text-sm font-medium">
              How it runs{" "}
              <span className="font-normal text-muted-foreground">
                chosen once; it keeps its conversations in this agent type
              </span>
            </legend>
            <ul className="m-0 grid list-none gap-1.5 p-0">
              {offer.Kinds.map((kind) => (
                <li key={kind.Name}>
                  <KindChoice
                    kind={kind}
                    first={offer.FirstReady}
                    installing={install.sending && installing === kind.Name}
                    onInstall={() => setInstalling(kind.Name)}
                  />
                </li>
              ))}
            </ul>
          </fieldset>
          {!offer.FirstReady && (
            <p
              role="status"
              className="m-0 rounded-md bg-warn-wash px-3 py-2 text-sm"
            >
              No agent type can run on this server yet. Ask whoever runs it to
              add one.
            </p>
          )}
          {offer.Offered.length > 0 && (
            <div className="grid gap-2">
              <Label htmlFor="new-agent-model">Model</Label>
              <FormSelect
                id="new-agent-model"
                name="model"
                value=""
                groups={[
                  {
                    choices: [
                      { value: "", label: "Default" },
                      ...offer.Offered,
                    ],
                  },
                ]}
              />
            </div>
          )}
          <DialogFooter>
            <DialogClose render={<Button variant="outline" type="button" />}>
              Cancel
            </DialogClose>
            <Button type="submit" disabled={!offer.FirstReady || make.sending}>
              Make the agent
            </Button>
          </DialogFooter>
        </form>
        {installable.map((kind) => (
          <form
            key={kind.Name}
            id={`install-${kind.Name}`}
            hidden
            method="post"
            action={`/agents/${encodeURIComponent(kind.Name)}/install`}
            onSubmit={install.submit}
          >
            <input type="hidden" name="back" value="/members?new=1" />
          </form>
        ))}
      </DialogContent>
    </Dialog>
  )
}

function NameFields() {
  return (
    <>
      <div className="grid gap-2">
        <Label htmlFor="new-agent-name">Name</Label>
        <Input
          id="new-agent-name"
          name="name"
          placeholder="Atlas"
          maxLength={32}
          required
          autoComplete="off"
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="new-agent-specialty">
          Specialty{" "}
          <span className="font-normal text-muted-foreground">
            helps teammates know what to ask it for
          </span>
        </Label>
        <Input
          id="new-agent-specialty"
          name="specialty"
          placeholder="Research and synthesis"
          maxLength={80}
        />
      </div>
    </>
  )
}

// KindChoice is one agent type to run the agent in: chosen with its radio, or, when this computer lacks it and gusto can install
// it, installed with its button.
function KindChoice({
  kind,
  first,
  installing,
  onInstall,
}: {
  kind: Kind
  first: string
  installing: boolean
  onInstall: () => void
}) {
  return (
    <label
      className={cn(
        "flex items-center gap-3 rounded-lg border px-3 py-2.5",
        kind.Ready
          ? "cursor-pointer hover:border-input has-[:checked]:border-primary has-[:checked]:bg-primary/5"
          : "cursor-not-allowed bg-sunken"
      )}
    >
      <input
        className="peer sr-only"
        type="radio"
        name="harness"
        value={kind.Name}
        defaultChecked={kind.Name === first}
        disabled={!kind.Ready}
      />
      <span className={cn(!kind.Ready && "opacity-50 grayscale")}>
        <AgentFace mark={kind.Mark} name={kind.Title} />
      </span>
      <span className="grid min-w-0">
        <span
          className={cn(
            "text-sm font-medium",
            !kind.Ready && "text-muted-foreground"
          )}
        >
          {kind.Title}
        </span>
        {kindNotes(kind).map((note) => (
          <span
            key={note.text}
            title={note.title}
            className={cn(
              "truncate text-xs",
              note.warn ? "text-warn" : "text-subtle"
            )}
          >
            {note.text}
          </span>
        ))}
      </span>
      {kind.Installable && (
        <Button
          type="submit"
          form={`install-${kind.Name}`}
          variant="ghost"
          size="sm"
          className="ml-auto flex-none"
          disabled={installing}
          onClick={onInstall}
        >
          {installing ? "Installing…" : "Install"}
        </Button>
      )}
      <CheckIcon
        aria-hidden="true"
        className="ml-auto hidden size-4 flex-none text-primary peer-checked:block"
      />
    </label>
  )
}
