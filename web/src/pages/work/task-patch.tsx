import { CutNote } from "@/pages/work/cut-note"
import { Disclosure } from "@/pages/work/disclosure"
import { SectionCard } from "@/pages/work/section-card"
import { plural } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { DiffFile, TaskData } from "@/pages/work/types"

// TaskPatch is what the run changed, file by file, each folded to its name and how much it added and removed. A patch that
// changed nothing adds nothing to the page.
export function TaskPatch({ page }: { page: TaskData }) {
  const patch = page.Patch
  if (!patch) {
    return (
      <p className="m-0 text-muted-foreground" data-testid="task-no-patch">
        Nothing was recorded.
      </p>
    )
  }
  if (patch.Empty) return null
  const files = patch.Files ?? []
  return (
    <SectionCard title="What changed" testId="task-patch">
      <p className="m-0 text-xs text-muted-foreground">
        {files.length} {plural(files.length, "file")}.
      </p>
      <ul className="m-0 grid list-none divide-y p-0">
        {files.map((file) => (
          <li key={file.Name} className="py-2">
            <PatchFile file={file} />
          </li>
        ))}
      </ul>
      <CutNote cut={patch} />
    </SectionCard>
  )
}

function PatchFile({ file }: { file: DiffFile }) {
  const hunks = file.Hunks ?? []
  return (
    <Disclosure
      testId="patch-file"
      summary={
        <span className="flex min-w-0 items-center gap-3 text-foreground">
          <code className="min-w-0 text-xs wrap-anywhere">{file.Name}</code>
          <span className="text-xs text-ok">+{file.Added}</span>
          <span className="text-xs text-bad">−{file.Removed}</span>
        </span>
      }
    >
      {(file.Meta ?? []).map((meta) => (
        <div key={meta} className="font-mono text-xs text-muted-foreground">
          {meta}
        </div>
      ))}
      {file.Binary && (
        <p className="m-0 mt-2 text-muted-foreground">
          A binary file. The patch names it and does not carry its contents.
        </p>
      )}
      {hunks.length > 0 && (
        <div className="mt-2 overflow-x-auto rounded-sm border">
          <table className="w-full font-mono text-xs leading-[1.55]">
            <tbody>
              {hunks.map((hunk, at) => (
                <HunkRows key={`${hunk.Header}-${at}`} hunk={hunk} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Disclosure>
  )
}

const number =
  "w-[1%] min-w-10 px-2 text-right align-top text-subtle select-none"
const code = "px-2 align-top whitespace-pre"

function HunkRows({ hunk }: { hunk: NonNullable<DiffFile["Hunks"]>[number] }) {
  return (
    <>
      <tr className="bg-muted text-muted-foreground">
        <td className={number} />
        <td className={number} />
        <td className={code}>{hunk.Header}</td>
      </tr>
      {(hunk.Lines ?? []).map((line, at) => (
        <tr
          key={at}
          data-kind={line.Kind}
          className={cn(
            line.Kind === "added" && "bg-ok-wash",
            line.Kind === "removed" && "bg-bad-wash"
          )}
        >
          <td className={number}>{line.Old || ""}</td>
          <td className={number}>{line.New || ""}</td>
          <td className={code}>{line.Text}</td>
        </tr>
      ))}
    </>
  )
}
