export async function request<T>(path: string, init: RequestInit = {}, messages: Record<string, string> = {}): Promise<T> {
  let response: Response
  try {
    response = await fetch(path, {
      ...init,
      headers: { Accept: 'application/json', ...init.headers },
    })
  } catch (error) {
    if (init.signal?.aborted) throw error
    throw new Error('Could not reach Tana. Check the connection and try again.')
  }

  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new Error(messages[body?.error] ?? `Tana could not complete the request (${response.status}). Try again.`)
  }
  if (response.status === 204) return undefined as T
  try {
    return await response.json() as T
  } catch {
    throw new Error('Tana returned an unexpected response. Check that the API is reachable.')
  }
}
