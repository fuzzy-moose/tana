import { request as apiRequest } from '../api'

export interface Library {
  id: string
  name: string
  path: string
  availability: 'available' | 'unavailable' | 'unknown'
  last_checked_at: string | null
}

const errorMessages: Record<string, string> = {
  invalid_name: 'Enter a library name.',
  invalid_path: 'Enter an absolute path on the machine running Tana.',
  root_unavailable: 'The library root must exist and be a directory on the machine running Tana.',
  root_conflict: 'This directory overlaps an existing library. Choose a separate library root.',
  storage_overlap: 'The library root cannot contain Tana’s application storage.',
  not_found: 'This library no longer exists. Refresh the list to see current libraries.',
  operation_timeout: 'The operation timed out. Check the library root and try again.',
  request_canceled: 'The request was canceled. Try again.',
  cross_origin_request: 'Tana rejected this request. Open the app through the same origin as its API.',
}

async function request<T>(path = '', init: RequestInit = {}): Promise<T> {
  return apiRequest<T>(`/api/libraries${path}`, init, errorMessages)
}

const libraryPath = (id: string) => `/${encodeURIComponent(id)}`

export function listLibraries(signal?: AbortSignal) {
  return request<Library[]>('', { signal })
}

export function createLibrary(name: string, path: string) {
  return request<Library>('', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, path }),
  })
}

export function renameLibrary(id: string, name: string) {
  return request<Library>(libraryPath(id), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

export function removeLibrary(id: string) {
  return request<void>(libraryPath(id), { method: 'DELETE' })
}

function pause(signal: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    signal.throwIfAborted()
    const abort = () => {
      clearTimeout(timer)
      reject(signal.reason)
    }
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', abort)
      resolve()
    }, 1000)
    signal.addEventListener('abort', abort, { once: true })
  })
}

export async function checkLibraryAvailability(id: string, signal: AbortSignal) {
  const path = libraryPath(id)
  let library = await request<Library>(path, { signal })
  const previousCheck = library.last_checked_at
  await request(`${path}/availability-check`, { method: 'POST', signal })

  // Acceptance only queues work; a new observation confirms completion.
  for (let attempt = 0; attempt < 15; attempt++) {
    await pause(signal)
    library = await request<Library>(path, { signal })
    if (library.last_checked_at !== previousCheck) return { library, completed: true }
  }
  return { library, completed: false }
}
