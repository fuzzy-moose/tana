// @vitest-environment jsdom
import { cleanup, render as testingRender, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Theme } from '@radix-ui/themes'
import type { ReactNode } from 'react'

import { afterEach, expect, test, vi } from 'vitest'
import Favorites from './Favorites'
import FavoriteSyncAction from './FavoriteSyncAction'
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
    if (path.endsWith('/sync')) {
      control.status = { ...control.status, categories: control.status.categories.map((item) => item.category === 0 ? { ...item, state: 'queued', queued: true } : item) }
      return Response.json({ status: 'accepted' }, { status: 202 })
    }
    if (path.endsWith('/missing-downloads/preview')) return Response.json({
      plan_id: 'p', category: 0, total: 12, present: 2, missing: 10, new_downloads: 10, existing_jobs: 0, retained_archives: 0, failed: 0, cancelled: 0, deleting: 0,
    })
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  return { fetchMock, control }
}

test('shows summary cards and lists every category with its own sync controls', async () => {
  setup()
  render(<Favorites {...props} />)
  await screen.findByRole('rowheader', { name: /Reading list/ })
  expect(screen.getByText('15')).toBeTruthy()
  expect(screen.getByText('0 running · 0 queued')).toBeTruthy()
  expect(screen.getByRole('columnheader', { name: 'Status' })).toBeTruthy()
  expect(screen.queryByRole('combobox', { name: 'Favorite category' })).toBeNull()
  expect(screen.queryByRole('radio')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Sync favorites' })).toBeNull()
  expect(screen.queryByRole('button', { name: /empty/ })).toBeNull()
  expect(screen.queryByRole('rowheader', { name: /Categories 2–4/ })).toBeNull()
  for (const name of ['Reading list', 'Manga', 'Favorites 2', 'Favorites 3', 'Favorites 4']) {
    const row = within(screen.getByRole('rowheader', { name: new RegExp(name) }).closest('tr')!)
    expect(row.getByRole('button', { name: 'Sync' })).toBeTruthy()
    expect(row.getByRole('button', { name: `More actions for ${name}` })).toBeTruthy()
  }
})

test('syncs one category, requests its full re-sync, and syncs all ten', async () => {
  const { fetchMock } = setup()
  const user = userEvent.setup()
  render(<Favorites {...props} />)
  const manga = within((await screen.findByRole('rowheader', { name: /Manga/ })).closest('tr')!)
  await user.click(manga.getByRole('button', { name: 'Sync' }))
  await screen.findByText('Sync requested for Manga.')
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/1/sync', expect.objectContaining({ method: 'POST', body: '{"full":false}' }))
  await user.click(manga.getByRole('button', { name: 'More actions for Manga' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Full re-sync' }))
  await screen.findByText('Full re-sync requested for Manga.')
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/1/sync', expect.objectContaining({ method: 'POST', body: '{"full":true}' }))
  await user.click(screen.getByRole('button', { name: 'Sync all' }))
  await screen.findByText('Sync requested for all ten categories.')
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/all/sync', expect.objectContaining({ method: 'POST', body: '{"full":false}' }))
  expect(fetchMock).not.toHaveBeenCalledWith('/api/collector/favorites/all/sync', expect.objectContaining({ body: '{"full":true}' }))
})

test.each([
  ['queued incremental', { state: 'queued', queued: true }, ' It replaces the queued sync.'],
  ['running incremental', { state: 'running' }, ' It starts after the running sync.'],
  ['incremental waiting for cooldown', { state: 'waiting_cooldown', retry_at: '2026-09-13T10:00:00Z' }, ' It starts after the running sync.'],
] as const)('a full re-sync over a %s sync says how it combines', async (_, overrides, suffix) => {
  const status = makeStatus()
  status.categories[1] = category(1, { name: 'Manga', favorites: 3, ...overrides })
  setup(status)
  const user = userEvent.setup()
  render(<Favorites {...props} />)
  const manga = within((await screen.findByRole('rowheader', { name: /Manga/ })).closest('tr')!)
  await user.click(manga.getByRole('button', { name: 'More actions for Manga' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Full re-sync' }))
  expect(await screen.findByText(`Full re-sync requested for Manga.${suffix}`)).toBeTruthy()
})

test('every sync control requests the baseline before it is ready', async () => {
  const status = makeStatus()
  status.downloads = { categories: [], baseline_state: 'collecting', baseline_categories: 4 }
  const { fetchMock } = setup(status)
  const user = userEvent.setup()
  render(<Favorites {...props} />)
  expect(await screen.findByText('Until the baseline is complete, any sync collects all ten categories without downloading favorites.')).toBeTruthy()
  const reading = within(screen.getByRole('rowheader', { name: /Reading list/ }).closest('tr')!)
  const notice = 'Baseline sync requested for all ten categories.'
  await user.click(reading.getByRole('button', { name: 'Sync' }))
  await screen.findByText(notice)
  await user.click(reading.getByRole('button', { name: 'More actions for Reading list' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Full re-sync' }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/0/sync', expect.objectContaining({ body: '{"full":true}' })))
  await screen.findByText(notice)
  await user.click(screen.getByRole('button', { name: 'Sync all' }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/all/sync', expect.anything()))
  expect(await screen.findByText(notice)).toBeTruthy()
})

test('shows a failed sync request as an alert', async () => {
  const { fetchMock } = setup()
  const base = fetchMock.getMockImplementation()!
  fetchMock.mockImplementation(async (input, init) => String(input).endsWith('/sync')
    ? Response.json({ error: 'collector_unavailable' }, { status: 502 })
    : base(input, init))
  const user = userEvent.setup()
  render(<Favorites {...props} />)
  await screen.findByRole('rowheader', { name: /Reading list/ })
  await user.click(screen.getByRole('button', { name: 'Sync all' }))
  expect((await screen.findByRole('alert')).textContent).toBe('The request could not be confirmed. Check collector status and try again.')
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
  const controls = () => [
    screen.getByRole('button', { name: 'Sync all' }),
    ...screen.getAllByRole('button', { name: 'Sync' }),
    ...screen.getAllByRole('button', { name: /^More actions for / }),
  ] as HTMLButtonElement[]
  expect(controls().every((button) => button.disabled)).toBe(true)
  view.rerender(<Favorites {...props} refreshKey={1} />)
  await waitFor(() => expect(screen.queryByText(/Favorite statistics are stale/)).toBeNull())
  expect(controls().every((button) => !button.disabled)).toBe(true)
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

test('the quick action follows saved download categories, not unsaved checkbox changes', async () => {
  setup()
  const user = userEvent.setup()
  render(<><FavoriteSyncAction /><Favorites {...props} /></>)
  const action = within(await screen.findByRole('group', { name: 'Favorite sync' }))
  await action.findByRole('button', { name: 'Sync Reading list' })
  await user.click(await screen.findByRole('checkbox', { name: 'Auto-download Manga' }))
  expect(action.getByRole('button', { name: 'Sync Reading list' })).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Save download categories' }))
  expect(await action.findByRole('button', { name: 'Sync favorites' })).toBeTruthy()
})

test('a sync request on the Favorites page refreshes the quick action at once', async () => {
  setup()
  const user = userEvent.setup()
  render(<><FavoriteSyncAction /><Favorites {...props} interval={60_000} /></>)
  const action = within(await screen.findByRole('group', { name: 'Favorite sync' }))
  await action.findByRole('button', { name: 'Sync Reading list' })
  expect(action.queryByText('Queued')).toBeNull()
  await user.click(await screen.findByRole('button', { name: 'Sync all' }))
  expect(await action.findByText('Queued')).toBeTruthy()
})
