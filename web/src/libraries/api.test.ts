import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { checkLibraryAvailability, createLibrary } from './api'

const library = { id: 1, name: 'Comics', path: '/comics', availability: 'available', last_checked_at: '2026-09-05T10:00:00Z' }
const fetchMock = vi.fn<typeof fetch>()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

test('does not confuse acceptance or an old observation with a completed availability check', async () => {
  vi.useFakeTimers()
  fetchMock.mockImplementation(async (_url, init) => init?.method === 'POST'
    ? Response.json({ status: 'accepted' }, { status: 202 })
    : Response.json(library))
  const result = checkLibraryAvailability(library.id, new AbortController().signal)
  await vi.advanceTimersByTimeAsync(15000)
  expect(await result).toEqual({ library, completed: false })
})

test('stops polling when the component cancels its check', async () => {
  vi.useFakeTimers()
  fetchMock.mockResolvedValueOnce(Response.json(library))
    .mockResolvedValueOnce(Response.json({ status: 'accepted' }, { status: 202 }))
  const controller = new AbortController()
  const result = checkLibraryAvailability(library.id, controller.signal)
  const rejection = expect(result).rejects.toMatchObject({ name: 'AbortError' })
  await vi.advanceTimersByTimeAsync(0)
  controller.abort()
  await rejection
  await vi.advanceTimersByTimeAsync(15000)
  expect(fetchMock).toHaveBeenCalledTimes(2)
})

test.each([
  ['invalid_path', 400, 'absolute path'],
  ['root_unavailable', 422, 'must exist and be a directory'],
  ['storage_overlap', 409, 'application storage'],
])('explains %s registration failures', async (error, status, message) => {
  fetchMock.mockResolvedValueOnce(Response.json({ error }, { status }))
  await expect(createLibrary('Comics', '/comics')).rejects.toThrow(message)
})
