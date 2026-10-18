// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import PandaLookup from './PandaLookup'
import { parseLookup } from './lookup'

const fetchMock = vi.fn<typeof fetch>()
const reference = { gid: 42, token: 'stored-token', url: 'https://configured.example/g/42/stored-token/' }
const pendingJob = { id: 'job', status: 'pending', entries: [{ gid: 42, status: 'pending' }] }

beforeEach(() => { fetchMock.mockReset(); vi.stubGlobal('fetch', fetchMock) })
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.useRealTimers() })

function submit(value: string) {
  fireEvent.change(screen.getByRole('textbox', { name: 'Look up gallery' }), { target: { value } })
  fireEvent.click(screen.getByRole('button', { name: 'Look up' }))
}

test('accepts positive safe IDs and existing URL conventions', () => {
  expect(parseLookup(' 42 ')).toEqual({ gid: 42 })
  expect(parseLookup('https://example.test/g/42/token/?p=2')).toEqual({ gid: 42, token: 'token' })
  for (const invalid of ['0', '-1', '1.5', '9007199254740992', 'https://example.test/g/42/']) {
    expect(() => parseLookup(invalid)).toThrow('Enter a positive gallery ID')
  }
})

test('shows token-only references and every retained metadata field including expunged galleries', async () => {
  fetchMock.mockResolvedValueOnce(Response.json(reference))
  render(<PandaLookup />)
  submit('42')
  await screen.findByText('Metadata not collected.')
  expect(screen.getByRole('link', { name: 'Open on Panda' }).getAttribute('href')).toBe(reference.url)
  expect(screen.getByText('stored-token')).toBeTruthy()
  fetchMock.mockResolvedValueOnce(Response.json({ ...reference, refreshed_at: '2026-09-13T10:00:00Z', metadata: {
    gid: 42, token: 'stored-token', error: '', title: 'Collected title', title_jpn: '日本語', category: 'Manga',
    thumb: 'https://thumb.example/image', uploader: 'Uploader name', posted: '100', filecount: '20', filesize: 2000,
    expunged: true, rating: '4.5', torrentcount: '1', tags: ['language:english'],
    torrents: [{ hash: 'hash-value', added: '90', name: 'Archive', tsize: '12', fsize: '2000' }],
    parent_gid: '40', parent_key: 'parent-token', current_gid: '43', current_key: 'current-token', first_gid: '39', first_key: 'first-token',
  } }))
  submit('42')
  await screen.findByText('Collected title')
  for (const text of ['日本語', 'Manga', 'Uploader name', 'language:english', 'parent-token', 'current-token', 'first-token', 'Yes']) {
    expect(screen.getByText(text)).toBeTruthy()
  }
  expect(screen.getByText(/hash-value/).textContent).toContain('tsize')
  expect(document.querySelectorAll('.panda-metadata dt')).toHaveLength(22)
  expect(document.querySelector('time')?.dateTime).toBe('2026-09-13T10:00:00Z')
})

test('distinguishes missing references from collector outages', async () => {
  render(<PandaLookup />)
  fetchMock.mockResolvedValueOnce(Response.json({ error: 'gallery_not_found' }, { status: 404 }))
  submit('42')
  expect((await screen.findByRole('alert')).textContent).toBe('Gallery not found in the collector.')
  fetchMock.mockResolvedValueOnce(Response.json({ error: 'collector_unavailable' }, { status: 502 }))
  submit('42')
  expect((await screen.findByRole('alert')).textContent).toContain('Collector unavailable')
})

test('polls an unverified URL job and reloads collected metadata by ID on success', async () => {
  vi.useFakeTimers()
  fetchMock.mockResolvedValueOnce(Response.json({ ...reference, unverified: true, fetch_job: pendingJob }))
    .mockResolvedValueOnce(Response.json({ ...pendingJob, status: 'completed', entries: [{ gid: 42, status: 'successful' }] }))
    .mockResolvedValueOnce(Response.json(reference))
  render(<PandaLookup />)
  submit('https://pasted.example/g/42/input-token/')
  await act(async () => {})
  expect(screen.getByText(/Unverified reference/)).toBeTruthy()
  expect(screen.getByRole('status').textContent).toContain('continues if you leave')
  await act(async () => { await vi.advanceTimersByTimeAsync(2000) })
  expect(fetchMock.mock.calls[1][0]).toBe('/api/collector/metadata/fetches/job')
  expect(fetchMock.mock.calls[2][1]?.body).toBe('{"gid":42}')
  expect(screen.queryByText(/Unverified reference/)).toBeNull()
  expect(screen.queryByRole('status')).toBeNull()
})

test('reports failed validation without treating the provisional token as verified', async () => {
  vi.useFakeTimers()
  fetchMock.mockResolvedValueOnce(Response.json({ ...reference, unverified: true, fetch_job: pendingJob }))
    .mockResolvedValueOnce(Response.json({ ...pendingJob, status: 'completed', entries: [{ gid: 42, status: 'failed', error: 'Invalid token' }] }))
  render(<PandaLookup />)
  submit('https://example.test/g/42/input-token/')
  await act(async () => {})
  await act(async () => { await vi.advanceTimersByTimeAsync(2000) })
  expect(screen.getByRole('alert').textContent).toContain('Invalid token')
  expect(screen.getByText(/Unverified reference/)).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledTimes(2)
})

test('new lookup ignores older responses and unmount stops polling without cancelling collector work', async () => {
  vi.useFakeTimers()
  let resolveFirst!: (response: Response) => void
  fetchMock.mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
    .mockResolvedValueOnce(Response.json({ ...reference, unverified: true, fetch_job: pendingJob }))
  const view = render(<PandaLookup />)
  submit('1')
  submit('https://example.test/g/42/token/')
  await act(async () => {})
  await act(async () => { resolveFirst(Response.json({ gid: 1, token: 'obsolete', url: '/obsolete' })) })
  expect(screen.queryByText('obsolete')).toBeNull()
  expect(screen.getByText('stored-token')).toBeTruthy()
  view.unmount()
  await act(async () => { await vi.advanceTimersByTimeAsync(6000) })
  expect(fetchMock).toHaveBeenCalledTimes(2)
})
