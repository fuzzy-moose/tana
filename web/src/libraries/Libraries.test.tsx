// @vitest-environment jsdom
import { StrictMode } from 'react'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import Libraries from './Libraries'
import type { Library } from './api'

const library: Library = {
  id: 'manga-1',
  name: 'Manga',
  path: '/mnt/manga',
  availability: 'available',
  last_checked_at: '2026-09-05T10:00:00Z',
}
const fetchMock = vi.fn<typeof fetch>()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

test('registers a library from the empty state and displays the server result', async () => {
  fetchMock.mockResolvedValueOnce(Response.json([]))
    .mockResolvedValueOnce(Response.json(library, { status: 201 }))
  const user = userEvent.setup()
  render(<Libraries />)
  await user.click(await screen.findByRole('button', { name: 'Add your first library' }))
  const form = screen.getByRole('form', { name: 'Add library' })
  await user.type(within(form).getByLabelText('Name'), '  Manga  ')
  await user.type(within(form).getByLabelText('Library root'), '/mnt/manga')
  await user.click(within(form).getByRole('button', { name: 'Add library' }))
  expect(await screen.findByRole('heading', { name: 'Manga' })).toBeTruthy()
  expect(screen.getByText('/mnt/manga')).toBeTruthy()
  expect(screen.getByText('Available')).toBeTruthy()
  expect(fetchMock).toHaveBeenLastCalledWith('/api/libraries', expect.objectContaining({
    method: 'POST',
    headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ name: 'Manga', path: '/mnt/manga' }),
  }))
})

test('retains input on registration errors and permits correction', async () => {
  fetchMock.mockResolvedValueOnce(Response.json([]))
    .mockResolvedValueOnce(Response.json({ error: 'root_conflict' }, { status: 409 }))
    .mockResolvedValueOnce(Response.json({ ...library, path: '/mnt/other' }, { status: 201 }))
  const user = userEvent.setup()
  render(<Libraries />)
  await user.click(await screen.findByRole('button', { name: 'Add your first library' }))
  const form = screen.getByRole('form', { name: 'Add library' })
  const name = within(form).getByLabelText('Name') as HTMLInputElement
  const path = within(form).getByLabelText('Library root') as HTMLInputElement
  await user.type(name, 'Manga')
  await user.type(path, '/mnt/manga')
  await user.click(within(form).getByRole('button', { name: 'Add library' }))
  expect((await screen.findByRole('alert')).textContent).toContain('overlaps an existing library')
  expect(name.value).toBe('Manga')
  expect(path.value).toBe('/mnt/manga')
  await user.clear(path)
  await user.type(path, '/mnt/other')
  await user.click(within(form).getByRole('button', { name: 'Add library' }))
  expect(await screen.findByText('/mnt/other')).toBeTruthy()
  expect(screen.queryByRole('alert')).toBeNull()
})

test('shows a load failure instead of an empty library and supports retry', async () => {
  fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'))
    .mockResolvedValueOnce(Response.json([{ ...library, availability: 'unknown', last_checked_at: null }]))
  const user = userEvent.setup()
  render(<Libraries />)
  expect(screen.getByRole('status').textContent).toBe('Loading libraries…')
  expect((await screen.findByRole('alert')).textContent).toContain('Could not reach Tana')
  expect(screen.queryByText('Your collection starts here.')).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('Not yet checked')).toBeTruthy()
  expect(screen.getByText('No availability check recorded')).toBeTruthy()
  expect(screen.queryByRole('alert')).toBeNull()
})

test('renames a library without offering or sending a root change', async () => {
  fetchMock.mockResolvedValueOnce(Response.json([library]))
    .mockResolvedValueOnce(Response.json({ ...library, name: 'Comics' }))
  const user = userEvent.setup()
  render(<Libraries />)
  await user.click(await screen.findByRole('button', { name: 'Rename' }))
  const form = screen.getByRole('form', { name: 'Rename library' })
  expect(within(form).queryByLabelText('Library root')).toBeNull()
  await user.clear(within(form).getByLabelText('Name'))
  await user.type(within(form).getByLabelText('Name'), 'Comics')
  await user.click(within(form).getByRole('button', { name: 'Save name' }))
  expect(await screen.findByRole('heading', { name: 'Comics' })).toBeTruthy()
  expect(fetchMock).toHaveBeenLastCalledWith('/api/libraries/manga-1', expect.objectContaining({
    method: 'PATCH', body: JSON.stringify({ name: 'Comics' }),
  }))
  expect(screen.getByText('/mnt/manga')).toBeTruthy()
})

test('requires removal confirmation, retains the library on failure, and handles 204', async () => {
  fetchMock.mockResolvedValueOnce(Response.json([library]))
    .mockResolvedValueOnce(Response.json({ error: 'internal_error' }, { status: 500 }))
    .mockResolvedValueOnce(new Response(null, { status: 204 }))
  const user = userEvent.setup()
  render(<Libraries />)
  await user.click(await screen.findByRole('button', { name: 'Remove' }))
  expect(screen.getByText(/Files in the library root stay untouched/)).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(fetchMock).toHaveBeenCalledTimes(1)
  await user.click(screen.getByRole('button', { name: 'Remove' }))
  await user.click(screen.getByRole('button', { name: 'Remove library' }))
  expect((await screen.findByRole('alert')).textContent).toContain('500')
  expect(screen.getByRole('heading', { name: 'Manga' })).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Remove library' }))
  expect(await screen.findByText('Your collection starts here.')).toBeTruthy()
  expect(fetchMock).toHaveBeenLastCalledWith('/api/libraries/manga-1', expect.objectContaining({ method: 'DELETE' }))
})

test('waits for an availability observation and retains unavailable libraries', async () => {
  fetchMock.mockResolvedValueOnce(Response.json([library]))
    .mockResolvedValueOnce(Response.json(library))
    .mockResolvedValueOnce(Response.json({ status: 'accepted' }, { status: 202 }))
    .mockResolvedValueOnce(Response.json({ ...library, availability: 'unavailable', last_checked_at: '2026-09-05T10:01:00Z' }))
  const user = userEvent.setup()
  render(<Libraries />)
  await user.click(await screen.findByRole('button', { name: 'Check availability' }))
  expect((screen.getByRole('button', { name: 'Checking…' }) as HTMLButtonElement).disabled).toBe(true)
  expect(screen.getByText('Available')).toBeTruthy()
  expect(await screen.findByText('Unavailable', {}, { timeout: 3000 })).toBeTruthy()
  expect(screen.getByRole('heading', { name: 'Manga' })).toBeTruthy()
  expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/libraries/manga-1/availability-check', expect.objectContaining({ method: 'POST' }))
})

test('ignores an obsolete initial response under StrictMode', async () => {
  let resolveOld!: (response: Response) => void
  fetchMock.mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
    .mockResolvedValueOnce(Response.json([library]))
  render(<StrictMode><Libraries /></StrictMode>)
  expect(await screen.findByRole('heading', { name: 'Manga' })).toBeTruthy()
  resolveOld(Response.json([]))
  await waitFor(() => expect(screen.getByRole('heading', { name: 'Manga' })).toBeTruthy())
})
