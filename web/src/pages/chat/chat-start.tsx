import { useEffect, useEffectEvent, useRef, useState } from "react"
import { useSearchParams } from "react-router"

import { Composer } from "@/components/chat/composer"
import {
  agentsFor,
  chooseAgent,
  chooseFolder,
  chooseModel,
  choosePreset,
  modelsFor,
  roleOf,
  saveChoice,
  startChoices,
  type Choices,
  type Offer,
} from "@/domain/chat-choices"
import type { ShownFolders } from "@/domain/folders"
import { ChoiceSelect } from "@/pages/chat/choice-select"
import { FolderChip, noFolder, type Folder } from "@/pages/chat/folder-chip"
import { FolderDialog } from "@/pages/chat/folder-dialog"
import { RunOn } from "@/pages/chat/run-on"
import { SavePreset } from "@/pages/chat/save-preset"
import { Starters } from "@/pages/chat/starters"
import type { ChatData } from "@/pages/chat/types"

function offerOf(page: ChatData): Offer {
  return {
    agents: page.Agents ?? [],
    models: page.Models ?? {},
    roles: page.Roles ?? [],
  }
}

// opening is what the page starts with: the preset, agent and model the server chose, and the project's folder when the page
// was opened from one, whose agent it then keeps.
function opening(
  offer: Offer,
  page: ChatData
): { choices: Choices; folder: Folder } {
  const choices = startChoices(offer, {
    preset: page.Role,
    harness: page.Harness,
    model: page.Model,
  })
  if (!page.Start.At) return { choices, folder: noFolder }
  return {
    choices: chooseFolder(offer, choices, page.Start.Harness),
    folder: { at: page.Start.At, key: "", name: "", label: page.Start.Name },
  }
}

// ChatStart is the box a new chat starts from, with where it works and what it runs on.
export function ChatStart({ page }: { page: ChatData }) {
  const offer = offerOf(page)
  const [start] = useState(() => opening(offer, page))
  const [choices, setChoices] = useState(start.choices)
  const [folder, setFolder] = useState(start.folder)
  const [params] = useSearchParams()
  const [prompt] = useState(() => params.get("prompt") ?? "")
  const [text, setText] = useState(prompt)
  const [showChoices, setShowChoices] = useState(false)
  const [saving, setSaving] = useState(false)
  const [choosing, setChoosing] = useState(false)
  const box = useRef<HTMLTextAreaElement>(null)
  useDesktopFolder((picked) => {
    setFolder({
      at: "",
      key: picked.key,
      name: picked.name,
      label: `${picked.name} (this computer)`,
    })
    setChoices((now) => chooseFolder(offer, now, ""))
  })
  const write = (said: string) => {
    if (!box.current) return
    box.current.value = said
    box.current.focus()
    setText(said)
  }
  const chosen = (shown: ShownFolders) => {
    setFolder({
      at: shown.at,
      key: "",
      name: "",
      label: shown.crumbs[shown.crumbs.length - 1]?.name ?? "",
    })
    setChoices((now) => chooseFolder(offer, now, shown.harness ?? ""))
    setChoosing(false)
  }
  return (
    <div className="flex w-full max-w-208 flex-col">
      <form method="post" action="/chat" className="order-2">
        <input type="hidden" name="choice" value={choices.preset} />
        <input type="hidden" name="role" value={roleOf(offer, choices)} />
        <input type="hidden" name="harness" value={choices.harness} />
        <input type="hidden" name="model" value={choices.model} />
        <Composer
          label="Message"
          placeholder="Message"
          defaultValue={prompt}
          describedBy={page.Refused ? "refused" : undefined}
          autoFocus
          boxRef={box}
          canSend={text.trim() !== ""}
          onWrite={setText}
          tools={
            <>
              <FolderChip
                folder={folder}
                onOpen={() => setChoosing(true)}
                onClear={() => {
                  setFolder(noFolder)
                  setChoices((now) => chooseFolder(offer, now, ""))
                }}
              />
            </>
          }
          actions={
            <RunOn
              offer={offer}
              choices={choices}
              open={showChoices}
              onToggle={() => setShowChoices((open) => !open)}
            />
          }
          below={
            showChoices && (
              <div
                id="chat-choices"
                className="flex flex-wrap items-center gap-1 border-t pt-2"
              >
                <ChoiceSelect
                  label="Preset"
                  choices={offer.roles}
                  value={choices.preset}
                  fixedBy={choices.fixedBy}
                  onChoose={(value) =>
                    value === saveChoice
                      ? setSaving(true)
                      : (setSaving(false),
                        setChoices(choosePreset(offer, choices, value)))
                  }
                />
                <ChoiceSelect
                  label="Agent"
                  choices={agentsFor(offer, choices.preset)}
                  value={choices.harness}
                  fixedBy={choices.fixedBy}
                  onChoose={(harness) =>
                    setChoices(chooseAgent(offer, choices, harness))
                  }
                />
                <ChoiceSelect
                  label="Model"
                  choices={modelsFor(offer, choices.harness)}
                  value={choices.model}
                  onChoose={(model) => setChoices(chooseModel(choices, model))}
                />
              </div>
            )
          }
        />
      </form>
      <div className="order-3">
        <Starters onChoose={write} />
      </div>
      {saving && (
        <div className="order-4">
          <SavePreset
            harness={choices.harness}
            model={choices.model}
            role={roleOf(offer, choices)}
            onCancel={() => setSaving(false)}
          />
        </div>
      )}
      {page.Wall && <Wall wall={page.Wall} />}
      <FolderDialog
        open={choosing}
        start={folder.at}
        harness={choices.harness}
        onClose={() => setChoosing(false)}
        onChoose={chosen}
      />
    </div>
  )
}

interface PickedFolder {
  key: string
  name: string
}

// useDesktopFolder hears of a folder of the person's own computer, which gusto's desktop app asks the system for and then tells
// the page about by calling window.latenticFolder.
function useDesktopFolder(onPicked: (picked: PickedFolder) => void) {
  const told = useEffectEvent(onPicked)
  useEffect(() => {
    const page = window as unknown as {
      latenticFolder?: (picked: PickedFolder) => void
    }
    page.latenticFolder = (picked) => told(picked)
    return () => {
      delete page.latenticFolder
    }
  }, [])
}

// Wall says what agents run in on this server, and why nothing stronger when something stronger could be.
function Wall({ wall }: { wall: NonNullable<ChatData["Wall"]> }) {
  return (
    <aside
      className="order-5 mx-auto mt-3 max-w-208 text-center text-xs text-muted-foreground"
      title={wall.Words}
    >
      <p className="m-0">Agents run in {wall.Short}.</p>
      {(wall.Missing ?? []).length > 0 && (
        <details className="group mt-2">
          <summary className="inline-flex cursor-pointer list-none items-center gap-1 hover:text-foreground [&::-webkit-details-marker]:hidden">
            Why not stronger?
          </summary>
          {wall.Missing!.map((why) => (
            <p key={why} className="m-0 mt-2">
              {why}.
            </p>
          ))}
        </details>
      )}
    </aside>
  )
}
