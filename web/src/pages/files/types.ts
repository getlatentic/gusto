// The Files pages' data, as internal/web/files_pages.go gives it.

import type { ShownFolders } from "@/domain/folders"

// FilesData is /app/api/files?at=: the place open, and the page the person came from, when they came from one.
export interface FilesData {
  Here: ShownFolders
  Back: string
}

export type FileKind =
  | "text"
  | "markdown"
  | "image"
  | "pdf"
  | "html"
  | "audio"
  | "video"
  | "office"
  | "table"
  | "binary"

// OfficePart is one part of a document, slide deck or sheet: its paragraphs, each with its heading level, or its rows.
export interface OfficePart {
  Title: string
  Lines: { Text: string; Heading: number }[] | null
  Rows: string[][] | null
}

// ViewedFile is one file as the viewer reads it. Raw is the file itself, Text what is shown of one read as words, and Cut whether
// that is not all of it. An editable file carries all of it as Source, and Hash of what it was.
export interface ViewedFile {
  Name: string
  Path: string
  Dir: string
  Kind: FileKind
  Size: string
  Modified: number
  Raw: string
  Download: string
  Text: string
  Editable: boolean
  Source: string
  Hash: string
  Language: string
  Lines: number
  Cut: boolean
  Rows: string[][] | null
  Parts: OfficePart[] | null
}

// FileData is /app/api/instances/{id}/file?path= or /app/api/jobs/{id}/files/{name}: the file, the pages above it and the way back.
// Prose is a Markdown file drawn; Save is where a change to the file is sent, and Source where a web page's source is.
export interface FileData {
  File: ViewedFile
  Crumbs: { label: string; href: string }[]
  Prose: string
  Back: string
  BackWord: string
  Save: string
  Source: string
}
