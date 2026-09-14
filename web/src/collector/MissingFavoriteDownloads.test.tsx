// @vitest-environment jsdom
import { cleanup, render as testingRender, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Theme } from '@radix-ui/themes'
import type { ReactNode } from 'react'

import { afterEach, expect, test, vi } from 'vitest'
import Favorites from './Favorites'
import type { FavoritesStatus } from './api'
import type { MissingFavoriteDownloadPreview } from './missingFavoriteDownloads'

const render = (ui: ReactNode) => testingRender(ui, { wrapper: Theme })

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

const status: FavoritesStatus = {
  host: 'https://panda.test', account_key: 'current',
  categories: [2, 3].map((category) => ({
    category, name: category === 2 ? 'Manga' : 'Comics', favorites: 10,
    state: 'idle', full: false, queued: false, queued_full: false,
    entries_saved: 10, pages_saved: 1, last_synced_at: '2026-09-13T09:00:00Z',
  })),
}
const preview: MissingFavoriteDownloadPreview = {
  plan_id: 'reviewed-manga', category: 2, total: 10, present: 2, missing: 8,
  new_downloads: 3, existing_jobs: 1, retained_archives: 1, failed: 1, cancelled: 1, deleting: 1,
}
const path = '/api/collector/favorites/2/missing-downloads'
const props = { available: true, refreshKey: 0 }
const categoryRow = (name: string) => within(screen.getByRole('rowheader', { name: new RegExp(name) }).closest('tr')!)

test('previews a whole category before confirming its reviewed plan and linking to Downloads', async () => {
  const fetchMock = vi.fn<typeof fetch>(async (input) => {
    if (String(input) === '/api/collector/favorites/status') return Response.json(status)
    if (String(input) === `${path}/preview`) return Response.json(preview)
    if (String(input) === path) return Response.json({ ...preview, present: 3, missing: 7, new_downloads: 2 }, { status: 202 })
    throw new Error(`Unexpected request: ${String(input)}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(<Favorites {...props} />)
  await screen.findByText('20 collected favorites')
  await user.click(categoryRow('Manga').getByRole('button', { name: 'Download missing…' }))
  const panel = within(screen.getByRole('region', { name: 'Download missing favorites · Manga' }))
  await panel.findByText('10 collected favorites · 2 present in Tana · 8 missing')
  expect(fetchMock).toHaveBeenCalledWith(`${path}/preview`, expect.objectContaining({ method: 'POST' }))
  expect(fetchMock.mock.calls.some(([input]) => String(input) === path)).toBe(false)
  expect(panel.getByText('New downloads').nextElementSibling?.textContent).toBe('3')
  expect(panel.getByText('Queued or running jobs to reuse').nextElementSibling?.textContent).toBe('1')
  expect(panel.getByText('Retained archives to reuse').nextElementSibling?.textContent).toBe('1')
  expect(panel.getByText('Failed jobs skipped').nextElementSibling?.textContent).toBe('1')
  expect(panel.getByText('Cancelled jobs skipped').nextElementSibling?.textContent).toBe('1')
  expect(panel.getByText('Deleting jobs skipped').nextElementSibling?.textContent).toBe('1')
  expect(panel.getByText(/all registered Tana libraries, including offline libraries/)).toBeTruthy()
  await user.click(panel.getByRole('button', { name: 'Confirm downloads' }))
  await panel.findByText(/Download requests accepted/)
  expect(fetchMock).toHaveBeenCalledWith(path, expect.objectContaining({ method: 'POST', body: '{"plan_id":"reviewed-manga"}' }))
  expect(panel.getByText('10 collected favorites · 3 present in Tana · 7 missing')).toBeTruthy()
  expect(panel.getByText('New downloads queued').nextElementSibling?.textContent).toBe('2')
  expect(panel.getByRole('link', { name: 'View Downloads' }).getAttribute('href')).toBe('#/collector/downloads')
  expect(panel.queryByRole('button', { name: 'Confirm downloads' })).toBeNull()
  await user.click(panel.getByRole('button', { name: 'Close' }))
  expect(screen.queryByRole('region', { name: 'Download missing favorites · Manga' })).toBeNull()
})

test('retries uncertain acceptance with the same plan and requires a fresh preview when it expires', async () => {
  let submissions = 0
  let previews = 0
  const fetchMock = vi.fn<typeof fetch>(async (input) => {
    if (String(input) === '/api/collector/favorites/status') return Response.json(status)
    if (String(input) === `${path}/preview`) return Response.json({ ...preview, plan_id: ++previews === 1 ? 'reviewed-manga' : 'fresh-manga' })
    if (String(input) === path) {
      submissions++
      if (submissions === 1) throw new TypeError('Failed to fetch')
      if (submissions === 2) return Response.json({ error: 'missing_download_preview_invalid' }, { status: 409 })
      return Response.json(preview, { status: 202 })
    }
    throw new Error(`Unexpected request: ${String(input)}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(<Favorites {...props} />)
  await screen.findByText('20 collected favorites')
  await user.click(categoryRow('Manga').getByRole('button', { name: 'Download missing…' }))
  await user.click(await screen.findByRole('button', { name: 'Confirm downloads' }))
  expect((await screen.findByRole('alert')).textContent).toContain('Could not reach Tana')
  await user.click(screen.getByRole('button', { name: 'Retry confirmation' }))
  expect((await screen.findByRole('alert')).textContent).toContain('This preview is no longer valid')
  expect(fetchMock.mock.calls.filter(([input]) => String(input) === path).map(([, init]) => init?.body)).toEqual([
    '{"plan_id":"reviewed-manga"}', '{"plan_id":"reviewed-manga"}',
  ])
  expect(screen.queryByRole('button', { name: 'Retry confirmation' })).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Refresh preview' }))
  await user.click(await screen.findByRole('button', { name: 'Confirm downloads' }))
  await screen.findByText(/Download requests accepted/)
  expect(fetchMock).toHaveBeenLastCalledWith(path, expect.objectContaining({ body: '{"plan_id":"fresh-manga"}' }))
})
