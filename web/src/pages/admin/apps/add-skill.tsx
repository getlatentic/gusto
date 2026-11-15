import { useState } from "react"
import { PlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import {
  GitImportForm,
  UploadImportForm,
  WriteSkillSetForm,
} from "@/pages/admin/apps/add-skill-forms"
import type { ImportedChoice } from "@/pages/admin/apps/types"

type Way = "new" | "git" | "upload"

const ways: { way: Way; title: string; words: string }[] = [
  {
    way: "new",
    title: "Write a skill set",
    words: "Name it, then write its skills here",
  },
  {
    way: "git",
    title: "Import from git",
    words: "An Agent Plugin in a repository",
  },
  {
    way: "upload",
    title: "Import an upload",
    words: "A .zip or .tar.gz of an Agent Plugin",
  },
]

// AddSkill is one button that opens the ways to add a skill; choosing a way shows only its form.
export function AddSkill({
  writes,
  imports,
  servers,
  imported,
}: {
  writes: boolean
  imports: boolean
  servers: string[]
  imported: ImportedChoice[]
}) {
  const offered = ways.filter((each) => (each.way === "new" ? writes : imports))
  const [way, setWay] = useState<Way>(writes ? "new" : "git")
  return (
    <Collapsible className="mb-5">
      <CollapsibleTrigger render={<Button />}>
        <PlusIcon /> Add a skill
      </CollapsibleTrigger>
      <CollapsibleContent>
        <Card className="mt-3">
          <CardContent className="grid gap-5">
            <RadioGroup
              value={way}
              onValueChange={(value) => setWay(value as Way)}
              aria-label="How to add it"
              className="grid-cols-[repeat(auto-fit,minmax(min(13rem,100%),1fr))] gap-2"
            >
              {offered.map((each) => (
                <Label
                  key={each.way}
                  className="cursor-pointer items-start gap-3 rounded-md border p-3 font-normal hover:bg-accent has-data-checked:border-foreground has-data-checked:bg-accent"
                >
                  <RadioGroupItem value={each.way} className="mt-0.5" />
                  <span className="grid gap-0.5">
                    <strong className="font-semibold">{each.title}</strong>
                    <span className="text-xs text-muted-foreground">
                      {each.words}
                    </span>
                  </span>
                </Label>
              ))}
            </RadioGroup>
            {way === "new" && writes && (
              <WriteSkillSetForm servers={servers} imported={imported} />
            )}
            {way === "git" && imports && <GitImportForm />}
            {way === "upload" && imports && <UploadImportForm />}
          </CardContent>
        </Card>
      </CollapsibleContent>
    </Collapsible>
  )
}
