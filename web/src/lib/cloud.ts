// gusto cloud knows what this server does not: which organizations and teams a person is in, and whether they confirmed
// their address. It is asked from the page with the person's gusto cloud session, which the server never sees.

export async function askCloud<T>(
  address: string,
  signal?: AbortSignal
): Promise<T> {
  const answer = await fetch(address, {
    credentials: "include",
    cache: "no-store",
    signal,
  })
  if (!answer.ok) throw new Error(`gusto cloud answered ${answer.status}`)
  return (await answer.json()) as T
}

// tellCloud posts to gusto cloud and gives back its status, for a caller that words each answer.
export async function tellCloud(address: string): Promise<number> {
  const answer = await fetch(address, {
    method: "POST",
    credentials: "include",
  })
  return answer.status
}
