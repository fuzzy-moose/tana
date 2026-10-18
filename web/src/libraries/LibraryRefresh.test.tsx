// @vitest-environment jsdom
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import App from '../App'
import LibraryRefresh from './LibraryRefresh'
import type { RefreshPlan } from './refreshApi'

vi.mock('../scans/useScan', () => ({ useScan: () => ({ status: null, error: '', disabled: true, start: vi.fn() }) }))

const plan: RefreshPlan = {
  plan_id: 'preview-one',
  candidates: [1, 2, 3].map((id) => ({
    source_id: id, library_id: 1, library_name: 'Manga', path: `/mnt/manga/Volume ${id}.cbz`,
    galleries: [
      { id: id * 10, title: `Volume ${id}`, deleted: true, pages_removed: 10 },
      { id: 40, title: 'Favorites', deleted: false, pages_removed: 2 },
    ],
  })),
  skipped: [{ library_id: 2, library_name: 'NAS', path: '/nas/manga', reason: 'No cataloged source could be confirmed present. Removal is blocked for this library.' }],
}

const fetchMock = vi.fn<typeof fetch>()

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

test.each([
  ['Refresh all', '/api/libraries/refresh', 'Refresh all libraries'],
  ['Refresh', '/api/libraries/1/refresh', 'Refresh library'],
])('opens %s from Libraries with the correct scope', async (label, endpoint, heading) => {
  window.history.replaceState(null, '', '/#/libraries')
  fetchMock.mockResolvedValueOnce(Response.json([{ id: 1, name: 'Manga', path: '/mnt/manga', availability: 'available', last_checked_at: null }]))
    .mockResolvedValueOnce(Response.json({ plan_id: 'empty', candidates: [], skipped: [] }))
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('heading', { name: 'Manga' })
  await user.click(screen.getByRole('link', { name: label }))
  expect(await screen.findByText('No missing sources eligible for removal.')).toBeTruthy()
  expect(screen.getByRole('heading', { name: heading })).toBeTruthy()
  expect(fetchMock).toHaveBeenLastCalledWith(endpoint, expect.objectContaining({ signal: expect.any(AbortSignal) }))
})

test('preselects candidates, shows affected galleries and blocked storage, and removes only confirmed selections', async () => {
  fetchMock.mockResolvedValueOnce(Response.json(plan))
    .mockResolvedValueOnce(Response.json({ removed: [1], failed: [{ source_id: 3, reason: 'Source is present again.' }] }))
  const user = userEvent.setup()
  render(<LibraryRefresh />)
  expect(await screen.findByText('3 sources selected')).toBeTruthy()
  expect(screen.getAllByRole('checkbox', { checked: true })).toHaveLength(3)
  expect(screen.getByText('Volume 1 — gallery will be deleted')).toBeTruthy()
  expect(screen.getAllByText('Favorites — 2 pages will be removed; gallery will be kept')).toHaveLength(3)
  expect(screen.getByRole('note', { name: 'Preserved entries' }).textContent).toContain('Removal is blocked for this library.')
  await user.click(screen.getByRole('checkbox', { name: 'Select /mnt/manga/Volume 2.cbz' }))
  expect(screen.getByText('2 sources selected')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledTimes(1)
  const confirmation = screen.getByRole('group', { name: 'Confirm source removal' })
  await user.click(within(confirmation).getByRole('button', { name: 'Remove 2 sources' }))
  expect(await screen.findByText('1 source removed. 1 could not be removed.')).toBeTruthy()
  expect(fetchMock).toHaveBeenLastCalledWith('/api/libraries/refresh', expect.objectContaining({
    method: 'POST', body: JSON.stringify({ plan_id: 'preview-one', source_ids: [1, 3] }),
  }))
  expect(screen.getByText('Removed from catalog')).toBeTruthy()
  expect(screen.getByText('Excluded from removal')).toBeTruthy()
  expect(screen.getByText('Kept: Source is present again.')).toBeTruthy()
  expect(screen.queryByRole('group', { name: 'Confirm source removal' })).toBeNull()
})

test('requires a new preview after an uncertain removal response', async () => {
  fetchMock.mockResolvedValueOnce(Response.json(plan))
    .mockRejectedValueOnce(new TypeError('Failed to fetch'))
    .mockResolvedValueOnce(Response.json({ ...plan, plan_id: 'fresh' }))
  const user = userEvent.setup()
  render(<LibraryRefresh />)
  await user.click(await screen.findByRole('button', { name: 'Remove 3 sources' }))
  expect((await screen.findByRole('alert')).textContent).toContain('Could not reach Tana')
  expect(screen.queryByRole('group', { name: 'Confirm source removal' })).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Check again' }))
  expect(await screen.findByRole('button', { name: 'Remove 3 sources' })).toBeTruthy()
  expect(screen.queryByRole('alert')).toBeNull()
})
