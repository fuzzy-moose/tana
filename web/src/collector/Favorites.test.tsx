// @vitest-environment jsdom
import { cleanup, render as testingRender, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Theme } from '@radix-ui/themes'
import type { ReactNode } from 'react'

import { afterEach, expect, test, vi } from 'vitest'
import Favorites from './Favorites'
import type { FavoriteCategory, FavoritesStatus } from './api'

const render = (ui: ReactNode) => testingRender(ui, { wrapper: Theme })

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

const category = (id: number, overrides: Partial<FavoriteCategory> = {}): FavoriteCategory => ({
  category: id, name: '', favorites: 0, state: 'idle', full: false, queued: false, queued_full: false, entries_saved: 0, pages_saved: 0, ...overrides,
})
const makeStatus = (): FavoritesStatus => ({
  host: 'https://panda.test', account_key: 'current',
  categories: [
    category(0, { name: 'Reading list', favorites: 12, last_synced_at: '2026-09-13T09:00:00Z' }),
    category(1, { name: 'Manga', favorites: 3, last_synced_at: '2026-09-13T09:00:00Z' }),
    ...[2, 3, 4].map((id) => category(id, { name: `Favorites ${id}`, last_synced_at: '2026-09-13T09:0' + id + ':00Z', last_outcome: 'success' })),
  ],
  downloads: { categories: [0], baseline_state: 'ready', baseline_categories: 10 },
})
const props = { available: true, refreshKey: 0, interval: 5000 }

function setup(status = makeStatus()) {
  const control = { status, offline: false }
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input)
    if (path === '/api/collector/favorites/status') {
      if (control.offline) return Response.json({ error: 'collector_unreachable' }, { status: 502 })
      return Response.json(control.status)
    }
    if (path === '/api/collector/favorites/download-settings') {
      const settings = JSON.parse(String(init?.body)) as { categories: number[] }
      control.status = { ...control.status, downloads: { ...control.status.downloads!, categories: settings.categories } }
      return Response.json(settings)
    }
    if (path.endsWith('/sync')) return Response.json({ status: 'accepted' }, { status: 202 })
    if (path.endsWith('/missing-downloads/preview')) return Response.json({
      plan_id: 'p', category: 0, total: 12, present: 2, missing: 10, new_downloads: 10, existing_jobs: 0, retained_archives: 0, failed: 0, cancelled: 0, deleting: 0,
    })
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  return { fetchMock, control }
}

test('shows summary cards and collapses empty categories', async () => {
  setup()
  render(<Favorites {...props} />)
  await screen.findByRole('rowheader', { name: /Reading list/ })
  expect(screen.getByText('15')).toBeTruthy()
  expect(screen.getByText('0 running · 0 queued')).toBeTruthy()
  expect(screen.queryByRole('rowheader', { name: /Favorites 2/ })).toBeNull()
  const collapsed = within(screen.getByRole('rowheader', { name: 'Categories 2–4' }).closest('tr')!)
  expect(collapsed.getByText('Completed')).toBeTruthy()
  expect(collapsed.getByRole('button', { name: 'Show 3 empty' })).toBeTruthy()
})

test('submits an all-category sync and a selected full re-sync', async () => {
  const { fetchMock } = setup()
  const user = userEvent.setup()
  render(<Favorites {...props} />)
  await screen.findByRole('rowheader', { name: /Reading list/ })
  await user.click(screen.getByRole('button', { name: 'Sync favorites' }))
  await screen.findByText('Sync requested for all categories.')
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/all/sync', expect.objectContaining({ method: 'POST', body: '{"full":false}' }))
  await user.click(screen.getByRole('combobox', { name: 'Favorite category' }))
  await user.click(screen.getByRole('option', { name: '1 · Manga' }))
  await user.click(screen.getByRole('radio', { name: 'Full re-sync' }))
  await user.click(screen.getByRole('button', { name: 'Sync favorites' }))
  await screen.findByText('Full re-sync requested for category 1.')
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/1/sync', expect.objectContaining({ method: 'POST', body: '{"full":true}' }))
})

test('saves auto-download categories and shows baseline progress', async () => {
  const initial = makeStatus()
  initial.downloads = { categories: [], baseline_state: 'not_started', baseline_categories: 0 }
  const { fetchMock, control } = setup(initial)
  const user = userEvent.setup()
  render(<Favorites {...props} />)
  await screen.findByText(/Your first sync will establish a baseline/)
  const save = screen.getByRole('button', { name: 'Save download categories' }) as HTMLButtonElement
  expect(save.disabled).toBe(true)
  const manga = screen.getByRole('checkbox', { name: 'Auto-download Manga' })
  expect(manga.getAttribute('aria-checked')).toBe('false')
  await user.click(manga)
  await user.click(save)
  await screen.findByText('Download categories saved.')
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/download-settings', expect.objectContaining({ method: 'PUT', body: '{"categories":[1]}' }))
  cleanup()
  control.status = { ...control.status, downloads: { ...control.status.downloads!, baseline_state: 'collecting', baseline_categories: 4 } }
  render(<Favorites {...props} />)
  await screen.findByText(/Establishing baseline: 4 of 10/)
  expect(screen.getByRole('checkbox', { name: 'Auto-download Manga' }).getAttribute('aria-checked')).toBe('true')
})

test('keeps favorites through a connection failure, disables actions, and recovers', async () => {
  setup()
  const view = render(<Favorites {...props} />)
  await screen.findByRole('rowheader', { name: /Reading list/ })
  view.rerender(<Favorites {...props} available={false} />)
  expect(await screen.findByText(/Favorite statistics are stale/)).toBeTruthy()
  expect(screen.getByRole('rowheader', { name: /Reading list/ })).toBeTruthy()
  expect((screen.getByRole('button', { name: 'Sync favorites' }) as HTMLButtonElement).disabled).toBe(true)
  expect(screen.getAllByRole('button', { name: 'Download missing…' }).every((button) => (button as HTMLButtonElement).disabled)).toBe(true)
  view.rerender(<Favorites {...props} refreshKey={1} />)
  await waitFor(() => expect(screen.queryByText(/Favorite statistics are stale/)).toBeNull())
  expect((screen.getByRole('button', { name: 'Sync favorites' }) as HTMLButtonElement).disabled).toBe(false)
})

test('shows durable progress and collected favorites before the first sync completes', async () => {
  const status = makeStatus()
  status.categories[1] = category(1, {
    name: 'Manga', state: 'running', favorites: 2000, entries_saved: 2000, pages_saved: 20,
    started_at: '2026-09-06T09:00:00Z', last_saved_at: '2026-09-06T09:59:00Z',
  })
  setup(status)
  render(<Favorites {...props} />)
  const row = within((await screen.findByRole('rowheader', { name: /Manga/ })).closest('tr')!)
  expect(row.getByText('Running sync')).toBeTruthy()
  expect(row.getByText('2,000 entries saved · 20 pages')).toBeTruthy()
  expect(row.getByText('2,000')).toBeTruthy()
  expect(row.getByText(/Last save/)).toBeTruthy()
  expect(row.getByText('Never synced')).toBeTruthy()
})
