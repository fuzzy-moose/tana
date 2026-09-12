// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'
import App from '../App'
import type { ConnectionStatus, FavoriteCategory, FavoriteDownloadsStatus } from './api'
import type { DownloadJob } from './downloads'

afterEach(() => { cleanup(); vi.unstubAllGlobals(); window.history.replaceState(null, '', '/') })

function connected(): ConnectionStatus {
  return {
    configured: true, reachable: true, authenticated: true, checked_at: '2026-09-06T10:00:00Z',
    status: { available: true },
  }
}

function favorites(): { host: string, account_key: string, categories: FavoriteCategory[], downloads: FavoriteDownloadsStatus } {
  return {
    host: 'https://panda.test', account_key: 'current',
    downloads: { categories: [], baseline_state: 'ready', baseline_categories: 10 },
    categories: Array.from({ length: 10 }, (_, category) => ({
      category, name: category === 2 ? 'Manga' : '', favorites: category === 2 ? 42 : 0,
      state: 'idle', full: false, queued: false, queued_full: false,
      entries_saved: 0, pages_saved: 0,
      last_synced_at: category === 2 ? '2026-09-06T09:00:00Z' : undefined,
    })),
  }
}

function statusResponse(input: RequestInfo | URL, connection = connected(), favoriteStatus = favorites()) {
  switch (String(input)) {
    case '/api/collector/status': return Response.json(connection)
    case '/api/collector/favorites/status': return Response.json(favoriteStatus)
    case '/api/collector/inventory/status': return Response.json({ gallery_references: 150, metadata_available: 100, metadata_pending: 45, metadata_failed: 5, fetches_pending: 3, fetches_failed: 0 })
    case '/api/collector/metadata/status': return Response.json({ metadata_errors: [{ gallery_id: 123, error: 'Gallery unavailable', at: '2026-09-06T09:00:00Z' }] })
    default: throw new Error(`Unexpected collector request: ${String(input)}`)
  }
}

test('Collector overview loads only inventory and metadata statistics', async () => {
  const fetchMock = vi.fn<typeof fetch>(async (input) => statusResponse(input))
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector')
  render(<App />)
  await screen.findByText('Connected')
  expect(screen.getByRole('link', { name: 'Collector' }).getAttribute('aria-current')).toBe('page')
  expect(await screen.findByText('150')).toBeTruthy()
  expect(await screen.findByText('Gallery unavailable')).toBeTruthy()
  expect(within(screen.getByRole('navigation', { name: 'Collector navigation' })).getByRole('link', { name: 'Overview' }).getAttribute('aria-current')).toBe('page')
  expect(new Set(fetchMock.mock.calls.map(([input]) => String(input)))).toEqual(new Set([
    '/api/collector/status', '/api/collector/inventory/status', '/api/collector/metadata/status',
  ]))
})

test('Favorites page submits all-category sync and a selected full re-sync', async () => {
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => init?.method === 'POST'
    ? Response.json({ status: 'accepted' }, { status: 202 }) : statusResponse(input))
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector/favorites')
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('42 collected favorites')
  expect(within(screen.getByRole('table')).getAllByRole('row')).toHaveLength(11)
  await user.click(screen.getByRole('button', { name: 'Sync favorites' }))
  expect(await screen.findByText('Sync requested for all categories.')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/all/sync', expect.objectContaining({ method: 'POST', body: '{"full":false}' }))
  await user.selectOptions(screen.getByLabelText('Favorite category'), '2')
  await user.selectOptions(screen.getByLabelText('Sync mode'), 'full')
  await user.click(screen.getByRole('button', { name: 'Full re-sync favorites' }))
  expect(await screen.findByText('Full re-sync requested for category 2.')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/2/sync', expect.objectContaining({ method: 'POST', body: '{"full":true}' }))
})

test('configures favorite downloads and shows baseline progress', async () => {
  const result = favorites()
  result.downloads = { categories: [], baseline_state: 'not_started', baseline_categories: 0 }
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    if (init?.method === 'PUT') {
      const settings = JSON.parse(String(init.body)) as { categories: number[] }
      result.downloads.categories = settings.categories
      return Response.json(settings)
    }
    return statusResponse(input, connected(), result)
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector/favorites')
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText(/Your first sync will establish a baseline/)
  const manga = screen.getByRole('checkbox', { name: '2 · Manga' }) as HTMLInputElement
  expect(manga.checked).toBe(false)
  await user.click(manga)
  await user.click(screen.getByRole('button', { name: 'Save download categories' }))
  expect(await screen.findByText('Download categories saved.')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/download-settings', expect.objectContaining({ method: 'PUT', body: '{"categories":[2]}' }))
  cleanup()
  result.downloads.baseline_state = 'collecting'
  result.downloads.baseline_categories = 4
  render(<App />)
  await screen.findByText(/Establishing baseline: 4 of 10/)
  expect((screen.getByRole('checkbox', { name: '2 · Manga' }) as HTMLInputElement).checked).toBe(true)
})

test('preserves favorites through connection failures, disables syncing, and recovers on refresh', async () => {
  let result = connected()
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input) => statusResponse(input, result)))
  window.history.replaceState(null, '', '/#/collector/favorites')
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('42 collected favorites')
  result = { configured: true, reachable: false, authenticated: null, checked_at: '2026-09-06T10:00:05Z', error: 'collector_unreachable' }
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(await screen.findByText(/stale/i)).toBeTruthy()
  expect(screen.getByText('42 collected favorites')).toBeTruthy()
  expect((screen.getByRole('button', { name: 'Sync favorites' }) as HTMLButtonElement).disabled).toBe(true)
  expect(screen.getByText('Unreachable')).toBeTruthy()
  result = connected()
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() => expect(screen.queryByText(/stale/i)).toBeNull())
  expect((screen.getByRole('button', { name: 'Sync favorites' }) as HTMLButtonElement).disabled).toBe(false)
})

test('shows durable progress and collected favorites before the first sync completes', async () => {
  const result = favorites()
  Object.assign(result.categories[2], {
    state: 'running', favorites: 2000, entries_saved: 2000, pages_saved: 20,
    started_at: '2026-09-06T09:00:00Z', last_saved_at: '2026-09-06T09:59:00Z', last_synced_at: undefined,
  })
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input) => statusResponse(input, connected(), result)))
  window.history.replaceState(null, '', '/#/collector/favorites')
  render(<App />)
  await screen.findByText('2,000 collected favorites')
  const row = screen.getByRole('rowheader', { name: /Manga/ }).closest('tr')!
  expect(within(row).getByText('Running sync')).toBeTruthy()
  expect(within(row).getByText('2,000 entries saved · 20 pages')).toBeTruthy()
  expect(within(row).getByText('2,000')).toBeTruthy()
  expect(within(row).getByText(/Last save/)).toBeTruthy()
  expect(within(row).getByText('Never synced')).toBeTruthy()
})

test('inventory failure does not prevent navigating to downloads or managing a download', async () => {
  let download: DownloadJob = {
    gallery_id: 7, state: 'running', failures: 0, size_bytes: 0,
    created_at: '2026-09-11T08:00:00Z', updated_at: '2026-09-11T09:00:00Z',
  }
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input)
    if (path === '/api/collector/inventory/status') return Response.json({ error: 'collector_unavailable' }, { status: 504 })
    if (path === '/api/collector/downloads/7/cancel' && init?.method === 'POST') {
      download = { ...download, state: 'cancelled' }
      return Response.json(download)
    }
    if (path.startsWith('/api/collector/downloads?')) return Response.json({
      jobs: [download],
      counts: { queued: 0, running: download.state === 'running' ? 1 : 0, completed: 0, failed: 0, cancelled: download.state === 'cancelled' ? 1 : 0, deleting: 0 },
    })
    return statusResponse(input)
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector')
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('alert')
  expect(screen.getByText('Connected')).toBeTruthy()
  const callsBeforeNavigation = fetchMock.mock.calls.length
  await user.click(within(screen.getByRole('navigation', { name: 'Collector navigation' })).getByRole('link', { name: 'Downloads' }))
  await screen.findByRole('rowheader', { name: 'Gallery 7' })
  expect(window.location.hash).toBe('#/collector/downloads')
  expect(screen.queryByRole('alert')).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(within(screen.getByRole('rowheader', { name: 'Gallery 7' }).closest('tr')!).getByText('Cancelled')).toBeTruthy())
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/downloads/7/cancel', expect.objectContaining({ method: 'POST' }))
  expect(fetchMock.mock.calls.slice(callsBeforeNavigation).every(([input]) =>
    String(input) === '/api/collector/status' || String(input).startsWith('/api/collector/downloads'),
  )).toBe(true)
})
