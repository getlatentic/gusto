import { dataOf, useData } from "@/lib/api"

interface Progress {
  what: string
  detail: string
  steps: number
}

// progressWords is what a running reply is doing, as its line says it: the step it is on, and what it is acting on.
export function progressWords(progress: Progress | undefined): string {
  if (!progress?.what) return ""
  const step = progress.steps
    ? `${progress.what} · step ${progress.steps}`
    : progress.what
  return progress.detail ? `${step} ${progress.detail}` : step
}

// useProgress is what each running reply of a room is doing, asked every two seconds while any is running and the page is seen.
export function useProgress(
  path: string,
  running: boolean
): Record<string, string> {
  const progress = useData<Record<string, Progress>>(`${path}/progress`, {
    pollingInterval: 2_000,
    skip: !running,
  })
  return Object.fromEntries(
    Object.entries(dataOf(progress) ?? {}).map(([turn, each]) => [
      turn,
      progressWords(each),
    ])
  )
}
