// @vitest-environment jsdom
import { cleanup, render as testingRender, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Theme } from '@radix-ui/themes'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'
import FavoriteSyncAction from './FavoriteSyncAction'
import type { FavoriteCategory, FavoriteDownloadsStatus, FavoritesStatus } from './api'

const render = (ui: ReactNode) => testingRender(ui, { wrapper: Theme })

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

const category = (id: number, overrides: Partial<FavoriteCategory> = {}): FavoriteCategory => ({
  category: id, name: '', favorites: 0, state: 'idle', full: false, queued: false, queued_full: false, entries_saved: 0, pages_saved: 0, ...overrides,
})
const makeStatus = (downloads: Partial<FavoriteDownloadsStatus> = {}, categories: FavoriteCategory[] = []): FavoritesStatus => ({
  host: 'https://panda.test', account_key: 'current',
  categories: Array.from({ length: 10 }, (_, id) => categories.find((item) => item.category === id) ?? category(id, { name: id === 0 ? 'Reading list' : id === 1 ? 'Manga' : '' })),
  downloads: { categories: [1], baseline_state: 'ready', baseline_categories: 10, ...downloads },
})

type Reply = () => Response | Promise<Response>
function setup(status: Reply, sync: Reply = () => Response.json({ status: 'accepted' }, { status: 202 })) {
  const fetchMock = vi.fn<typeof fetch>(async (input) => {
    const path = String(input)
    if (path === '/api/collector/favorites/status') return status()
    if (path === '/api/collector/favorites/download-categories/sync') return sync()
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}
const statusRequests = (fetchMock: ReturnType<typeof setup>) => fetchMock.mock.calls.filter(([path]) => path === '/api/collector/favorites/status').length
const posts = (fetchMock: ReturnType<typeof setup>) => fetchMock.mock.calls.filter(([, init]) => init?.method === 'POST')

test('renders nothing before the first response and without a configured collector', async () => {
  let resolve: (response: Response) => void = () => {}
  const fetchMock = setup(() => new Promise((done) => { resolve = done }))
  const { container } = render(<FavoriteSyncAction />)
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
  expect(container.textContent).toBe('')
  resolve(Response.json({ error: 'collector_not_configured' }, { status: 503 }))
  await waitFor(() => expect(container.querySelector('.favorite-sync-action')).toBeNull())
  await new Promise((done) => setTimeout(done, 0))
  expect(container.textContent).toBe('')
  expect(screen.queryByRole('button')).toBeNull()
})

test.each([
  ['collector_unauthorized', 502, 'Collector rejected access'],
  ['collector_unavailable', 502, 'Collector unreachable'],
  ['internal', 500, 'Sync status unavailable'],
])('disables the button when status fails with %s', async (error, code, reason) => {
  setup(() => Response.json({ error }, { status: code }))
  render(<FavoriteSyncAction />)
  expect(await screen.findByText(reason)).toBeTruthy()
  expect((screen.getByRole('button', { name: 'Sync favorites' }) as HTMLButtonElement).disabled).toBe(true)
})

test('disables the button with a hint when no category is saved', async () => {
  setup(() => Response.json(makeStatus({ categories: [] })))
  render(<FavoriteSyncAction />)
  const button = await screen.findByRole('button', { name: 'Sync favorites' }) as HTMLButtonElement
  expect(button.disabled).toBe(true)
  expect(screen.getByText(/Check a category for auto-download on the/)).toBeTruthy()
  expect(screen.getByRole('link', { name: 'Favorites page' }).getAttribute('href')).toBe('#/collector/favorites')
})

test.each([
  [[1], 'Sync Manga', 'Manga'],
  [[3], 'Sync Category 3', 'Category 3'],
  [[0, 1], 'Sync favorites', 'Reading list, Manga'],
])('labels saved categories %j', async (categories, label, title) => {
  setup(() => Response.json(makeStatus({ categories })))
  render(<FavoriteSyncAction />)
  const button = await screen.findByRole('button', { name: label }) as HTMLButtonElement
  expect(button.disabled).toBe(false)
  expect(button.getAttribute('title')).toBe(title)
})

test.each(['not_started', 'collecting'] as const)('says a click starts the baseline while it is %s', async (baseline_state) => {
  setup(() => Response.json(makeStatus({ categories: [1], baseline_state, baseline_categories: 3 })))
  render(<FavoriteSyncAction />)
  const button = await screen.findByRole('button', { name: 'Sync Manga' }) as HTMLButtonElement
  expect(button.disabled).toBe(false)
  expect(screen.getByText('Starts the baseline sync of all ten categories.')).toBeTruthy()
})

test('a click posts once and fetches status again, also while a saved category is busy', async () => {
  const fetchMock = setup(() => Response.json(makeStatus({ categories: [0, 1] }, [category(0, { name: 'Reading list', state: 'running' }), category(1, { name: 'Manga', state: 'queued', queued: true })])))
  const user = userEvent.setup()
  render(<FavoriteSyncAction />)
  const button = await screen.findByRole('button', { name: 'Sync favorites' }) as HTMLButtonElement
  expect(button.disabled).toBe(false)
  expect(posts(fetchMock)).toHaveLength(0)
  await user.click(button)
  await waitFor(() => expect(statusRequests(fetchMock)).toBe(2))
  expect(posts(fetchMock)).toEqual([['/api/collector/favorites/download-categories/sync', expect.objectContaining({ method: 'POST', body: '{}' })]])
  expect(screen.queryByRole('alert')).toBeNull()
})

test.each([
  ['collector_unavailable', 502, 'The request could not be confirmed. Check collector status and try again.'],
  ['no_download_categories', 409, 'No category has auto-download checked. Check one on the Favorites page.'],
])('shows the mapped error when the click fails with %s', async (error, code, message) => {
  const fetchMock = setup(() => Response.json(makeStatus()), () => Response.json({ error }, { status: code }))
  const user = userEvent.setup()
  render(<FavoriteSyncAction />)
  await user.click(await screen.findByRole('button', { name: 'Sync Manga' }))
  expect((await screen.findByRole('alert')).textContent).toBe(message)
  await waitFor(() => expect(statusRequests(fetchMock)).toBe(2))
})

test('shows the activity and failure of the tracked categories', async () => {
  setup(() => Response.json(makeStatus({ categories: [0, 1] }, [
    category(0, { name: 'Reading list', state: 'queued', queued: true }),
    category(1, { name: 'Manga', last_outcome: 'failed', last_error: 'Panda returned 500' }),
  ])))
  render(<FavoriteSyncAction />)
  expect(await screen.findByText('Queued')).toBeTruthy()
  expect(screen.getByText('Manga failed: Panda returned 500')).toBeTruthy()
})

test('shows the last sync time and the baseline progress', async () => {
  setup(() => Response.json(makeStatus({ categories: [1], baseline_state: 'collecting', baseline_categories: 4 },
    Array.from({ length: 10 }, (_, id) => category(id, { last_synced_at: '2026-11-14T10:00:00Z' })))))
  render(<FavoriteSyncAction />)
  expect(await screen.findByText(`Last synced ${new Date('2026-11-14T10:00:00Z').toLocaleString()}`)).toBeTruthy()
  expect(screen.getByText('Baseline: 4 of 10 categories')).toBeTruthy()
})
