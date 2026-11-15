import { useQuery } from "@tanstack/react-query"

import { getJSON } from "@/lib/api"

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
  const progress = useQuery({
    queryKey: [`${path}/progress`],
    queryFn: ({ signal }) =>
      getJSON<Record<string, Progress>>(`${path}/progress`, signal),
    enabled: running,
    refetchInterval: 2_000,
  })
  return Object.fromEntries(
    Object.entries(progress.data ?? {}).map(([turn, each]) => [
      turn,
      progressWords(each),
    ])
  )
}
