import { DownloadIcon, FileIcon } from "lucide-react"

import { buttonVariants } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableRow } from "@/components/ui/table"
import { CodeView } from "@/pages/files/code-view"
import type { FileData, OfficePart, ViewedFile } from "@/pages/files/types"
import { withBase } from "@/lib/base"

const frame = "h-[72vh] w-full rounded-md border [color-scheme:light]"
const stage = "m-0 grid place-items-center rounded-md bg-muted p-4"
const cell = "max-w-96 truncate border px-4 py-2"

// FileBody is what a file is, drawn by its kind, and a word when it is longer than is drawn.
export function FileBody({ page }: { page: FileData }) {
  const file = page.File
  return (
    <div data-testid="file-body" className="grid min-w-0 gap-5">
      <Shown page={page} />
      {file.Cut && (
        <p className="m-0 text-xs text-muted-foreground">
          The file is longer than the page draws. Download it to read the rest.
        </p>
      )}
    </div>
  )
}

function Shown({ page }: { page: FileData }) {
  const file = page.File
  switch (file.Kind) {
    case "markdown":
      return (
        <article
          className="prose max-w-[46rem] min-w-0"
          // The Markdown is drawn and made safe by the server, which rewrites its links to the folder's files.
          dangerouslySetInnerHTML={{ __html: page.Prose }}
        />
      )
    case "text":
      return <CodeView text={file.Text} language={file.Language} />
    case "table":
      return <Grid rows={file.Rows ?? []} heading />
    case "image":
      return (
        <figure className={stage}>
          <img
            className="max-h-[70vh] max-w-full rounded-sm"
            src={withBase(file.Raw)}
            alt={file.Name}
          />
        </figure>
      )
    case "pdf":
      return (
        <iframe className={frame} src={withBase(file.Raw)} title={file.Name} />
      )
    case "html":
      return (
        <iframe
          className={frame}
          sandbox="allow-scripts allow-forms allow-modals"
          src={withBase(file.Raw)}
          title={file.Name}
        />
      )
    case "audio":
      return (
        <figure className={stage}>
          <audio controls src={withBase(file.Raw)} />
        </figure>
      )
    case "video":
      return (
        <figure className={stage}>
          <video
            className="max-h-[70vh] max-w-full rounded-sm"
            controls
            src={withBase(file.Raw)}
          />
        </figure>
      )
    case "office":
      return <Office parts={file.Parts ?? []} />
    default:
      return <NotShown file={file} />
  }
}

// Grid is a table's cells as they are: the first row its heading when it is a table file, and every row as plain cells in a sheet.
function Grid({
  rows,
  heading = false,
}: {
  rows: string[][]
  heading?: boolean
}) {
  return (
    <Table className="w-auto min-w-full text-xs">
      <TableBody>
        {rows.map((row, at) => (
          <TableRow key={at} className="hover:bg-transparent">
            {row.map((value, column) =>
              heading && at === 0 ? (
                <th
                  key={column}
                  scope="col"
                  className="border bg-muted px-4 py-2 text-left font-semibold whitespace-nowrap"
                >
                  {value}
                </th>
              ) : (
                <TableCell key={column} className={cell} title={value}>
                  {value}
                </TableCell>
              )
            )}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

// Office is the words of a document, slide deck or sheet, part by part.
function Office({ parts }: { parts: OfficePart[] }) {
  return (
    <>
      <p className="m-0 rounded-sm bg-muted px-4 py-3 text-xs text-muted-foreground">
        This is the text of the file, not how it looks. Download it to open it
        in its own program.
      </p>
      {parts.map((part, at) => (
        <section
          key={at}
          aria-label={part.Title || undefined}
          className="grid gap-3 rounded-lg border bg-card p-6"
        >
          {part.Title && (
            <h2 className="m-0 text-xs font-semibold text-subtle">
              {part.Title}
            </h2>
          )}
          {(part.Lines ?? []).map((line, n) =>
            line.Heading === 0 ? (
              <p key={n} className="m-0 leading-relaxed whitespace-pre-wrap">
                {line.Text}
              </p>
            ) : line.Heading === 1 ? (
              <h3 key={n} className="m-0 mt-2 text-sm">
                {line.Text}
              </h3>
            ) : (
              <h4 key={n} className="m-0 text-sm">
                {line.Text}
              </h4>
            )
          )}
          {part.Rows && <Grid rows={part.Rows} />}
        </section>
      ))}
    </>
  )
}

function NotShown({ file }: { file: ViewedFile }) {
  return (
    <div className="grid justify-items-center gap-3 rounded-md bg-muted px-4 py-16 text-center text-muted-foreground">
      <FileIcon aria-hidden="true" className="size-6" />
      <p className="m-0">This kind of file cannot be shown here.</p>
      <a
        className={buttonVariants({ variant: "outline", size: "sm" })}
        href={withBase(file.Download)}
        download
      >
        <DownloadIcon aria-hidden="true" />
        <span>Download it</span>
      </a>
    </div>
  )
}
