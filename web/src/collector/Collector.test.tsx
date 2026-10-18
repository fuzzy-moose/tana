// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'
import App from '../App'
import type { CollectorSummary, ConnectionStatus, FavoriteCategory, FavoriteDownloadsStatus } from './api'
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

function quiet(): CollectorSummary {
  return {
    downloads: { active: 0, failed: 0 }, favorites: { syncing: false }, metadata: { main_background_paused: false },
    sitemap: { state: 'idle', children_total: 0, children_processed: 0 }, feeds: { failed_captures: 0 },
    reference_imports: { active: 0 }, proxies: { enabled: false, channels_in_cooldown: 0 },
  }
}

function statusResponse(input: RequestInfo | URL, connection = connected(), favoriteStatus = favorites()) {
  switch (String(input)) {
    case '/api/collector/status': return Response.json(connection)
    case '/api/collector/summary': return Response.json(quiet())
    case '/api/libraries': return Response.json([])
    case '/api/library-deliveries': return Response.json({ batches: [] })
    case '/api/collector/favorites/status': return Response.json(favoriteStatus)
    case '/api/collector/inventory/status': return Response.json({ gallery_references: 150, metadata_available: 100, metadata_pending: 45, metadata_failed: 5, fetches_pending: 3, fetches_failed: 0 })
    case '/api/collector/metadata/status': return Response.json({ main_background_paused: false, feed_metadata_delay_hours: 12, metadata_errors: [{ gallery_id: 123, error: 'Gallery unavailable', at: '2026-09-06T09:00:00Z' }] })
    default: throw new Error(`Unexpected collector request: ${String(input)}`)
  }
}

test('Collector overview shows attention items, service status and inventory from the summary', async () => {
  const summary: CollectorSummary = {
    ...quiet(),
    downloads: { active: 0, failed: 3, latest_error: 'timed out', storage: { paused: false, available_bytes: 412 * 1024 ** 3, pause_below_bytes: 20 * 1024 ** 3, resume_at_bytes: 30 * 1024 ** 3 } },
    favorites: { syncing: false, authenticated_cooldown_until: '2099-01-01T00:00:00Z' },
  }
  const fetchMock = vi.fn<typeof fetch>(async (input) => String(input) === '/api/collector/summary' ? Response.json(summary) : statusResponse(input))
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector')
  render(<App />)
  const attention = within(await screen.findByRole('region', { name: 'Needs attention · 2' }))
  expect(attention.getByText('Authenticated cooldown')).toBeTruthy()
  expect(attention.getByText(/3 downloads failed/)).toBeTruthy()
  expect(attention.getByRole('link', { name: 'Review failed' }).getAttribute('href')).toBe('#/collector/downloads?state=failed')
  const status = within(screen.getByRole('region', { name: 'Service status' }))
  expect(await status.findByText('Running')).toBeTruthy()
  expect(status.getByText('No cooldown')).toBeTruthy()
  expect(status.getByText('Cooldown')).toBeTruthy()
  expect(status.getByText('412.0 GiB free')).toBeTruthy()
  expect(status.getByText('Pauses below 20.0 GiB')).toBeTruthy()
  expect(await screen.findByText('150')).toBeTruthy()
  expect(screen.getByText('Explicit fetch requests: 3 pending · 0 failed')).toBeTruthy()
  expect(screen.getByRole('img').getAttribute('aria-label')).toBe('66.7% available, 30.0% pending, 3.3% failed')
  expect(await screen.findByText('Gallery unavailable')).toBeTruthy()
  expect(within(screen.getByRole('navigation', { name: 'Collector navigation' })).getByRole('link', { name: /^Overview/ }).getAttribute('aria-current')).toBe('page')
})

test('Collector overview hides the attention section when nothing needs attention', async () => {
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input) => statusResponse(input)))
  window.history.replaceState(null, '', '/#/collector')
  render(<App />)
  await within(await screen.findByRole('region', { name: 'Service status' })).findAllByText('No cooldown')
  expect(screen.queryByText(/Needs attention/)).toBeNull()
})

test('sidebar groups the pages, marks the current one and shows summary-driven status labels', async () => {
  const summary: CollectorSummary = {
    downloads: { active: 2, failed: 3, latest_error: 'timed out' }, favorites: { syncing: true, authenticated_cooldown_until: '2099-01-01T00:00:00Z' },
    metadata: { main_background_paused: false }, sitemap: { state: 'running', children_total: 200, children_processed: 50 },
    feeds: { failed_captures: 1 }, reference_imports: { active: 4 }, proxies: { enabled: true, channels_in_cooldown: 0 },
  }
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input) => String(input) === '/api/collector/summary' ? Response.json(summary) : statusResponse(input)))
  window.history.replaceState(null, '', '/#/collector/sitemap')
  render(<App />)
  const sidebar = within(await screen.findByRole('navigation', { name: 'Collector navigation' }))
  for (const group of ['Acquire', 'References', 'Settings']) expect(sidebar.getByText(group)).toBeTruthy()
  const link = (name: RegExp) => sidebar.getByRole('link', { name })
  await waitFor(() => expect(link(/^Downloads/).textContent).toContain('2 active'))
  expect(link(/^Sitemap/).getAttribute('aria-current')).toBe('page')
  expect(link(/^Overview/).getAttribute('aria-current')).toBeNull()
  expect(link(/Overview.*3 need attention/)).toBeTruthy()
  expect(link(/^Favorites/).textContent).toContain('Syncing')
  expect(link(/^Sitemap/).textContent).toContain('25%')
  expect(within(link(/^Raw feeds/)).getByLabelText('Has failures')).toBeTruthy()
  expect(link(/Reference imports.*4 active imports/)).toBeTruthy()
  expect(link(/^Metadata proxies/).textContent).toContain('On')
  expect(link(/^Reference imports/).getAttribute('href')).toBe('#/collector/imports')
  expect(sidebar.getByText('This page updates every 5 s')).toBeTruthy()
})

test('Refresh now re-checks the connection and the summary', async () => {
  const fetchMock = vi.fn<typeof fetch>(async (input) => statusResponse(input))
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector/sitemap')
  const user = userEvent.setup()
  render(<App />)
  const count = (path: string) => fetchMock.mock.calls.filter(([input]) => String(input) === path).length
  await waitFor(() => expect(count('/api/collector/summary')).toBe(1))
  await waitFor(() => expect((screen.getByRole('button', { name: 'Refresh now' }) as HTMLButtonElement).disabled).toBe(false))
  await user.click(screen.getByRole('button', { name: 'Refresh now' }))
  await waitFor(() => expect(count('/api/collector/status')).toBe(2))
  await waitFor(() => expect(count('/api/collector/summary')).toBe(2))
})

test('an unconfigured collector replaces the page with setup guidance', async () => {
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input) => statusResponse(input, { configured: false, reachable: false, authenticated: null, checked_at: '2026-09-06T10:00:00Z' })))
  window.history.replaceState(null, '', '/#/collector')
  render(<App />)
  expect(await screen.findByText(/Configure Tana’s collector URL/)).toBeTruthy()
  expect(screen.getAllByText('Not configured').length).toBeGreaterThan(0)
  expect(screen.queryByText('Collector inventory')).toBeNull()
})

test('on phones the sidebar expands from a toggle named after the current page', async () => {
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input) => statusResponse(input)))
  window.history.replaceState(null, '', '/#/collector/downloads')
  const user = userEvent.setup()
  render(<App />)
  const toggle = await screen.findByRole('button', { name: 'Collector · Downloads' })
  expect(toggle.getAttribute('aria-expanded')).toBe('false')
  await user.click(toggle)
  expect(toggle.getAttribute('aria-expanded')).toBe('true')
  await user.click(toggle)
  expect(toggle.getAttribute('aria-expanded')).toBe('false')
})

test('Collector overview pauses and resumes main background metadata collection', async () => {
  let paused = false
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input)
    if (init?.method === 'POST' && ['/api/collector/metadata/pause', '/api/collector/metadata/resume'].includes(path)) {
      paused = path.endsWith('/pause')
      return new Response(null, { status: 204 })
    }
    if (path === '/api/collector/metadata/status') return Response.json({ main_background_paused: paused, feed_metadata_delay_hours: 12, metadata_errors: [] })
    return statusResponse(input)
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector')
  const user = userEvent.setup()
  render(<App />)
  const toggle = await screen.findByRole('switch', { name: 'Background collection' })
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  await user.click(toggle)
  expect(await screen.findByText('Paused')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/metadata/pause', expect.objectContaining({ method: 'POST', body: '{}' }))
  cleanup()
  render(<App />)
  await user.click(await screen.findByRole('switch', { name: 'Background collection' }))
  expect(await screen.findByText('Running')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/metadata/resume', expect.objectContaining({ method: 'POST', body: '{}' }))
})

test('Collector overview resets the main Panda cooldown from the attention list', async () => {
  let banned = true
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input)
    if (init?.method === 'POST' && path === '/api/collector/bans/main/reset') {
      banned = false
      return new Response(null, { status: 204 })
    }
    if (path === '/api/collector/summary') return Response.json({ ...quiet(), metadata: { main_background_paused: false, ...(banned ? { upstream_cooldown_until: '2099-01-01T00:00:00Z' } : {}) } })
    return statusResponse(input)
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector')
  const user = userEvent.setup()
  render(<App />)
  await user.click(await screen.findByRole('button', { name: 'Reset cooldown' }))
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/bans/main/reset', expect.objectContaining({ method: 'POST', body: '{}' }))
  await waitFor(() => expect(screen.queryByRole('button', { name: 'Reset cooldown' })).toBeNull())
})

test('saves the shared feed delay, including zero, across overview visits', async () => {
  let hours = 12
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    if (String(input) === '/api/collector/metadata/settings') {
      hours = JSON.parse(String(init?.body)).feed_metadata_delay_hours
      return Response.json({ feed_metadata_delay_hours: hours })
    }
    if (String(input) === '/api/collector/metadata/status') return Response.json({ main_background_paused: false, feed_metadata_delay_hours: hours, metadata_errors: [] })
    return statusResponse(input)
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector')
  const user = userEvent.setup()
  render(<App />)
  const input = await screen.findByRole('spinbutton', { name: 'Feed metadata delay (hours)' }) as HTMLInputElement
  expect(input.value).toBe('12')
  await user.clear(input)
  await user.type(input, '0')
  await user.click(screen.getByRole('button', { name: 'Save feed metadata delay' }))
  expect(await screen.findByText('Feed metadata delay saved.')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/metadata/settings', expect.objectContaining({ method: 'PUT', body: '{"feed_metadata_delay_hours":0}' }))
  cleanup()
  render(<App />)
  expect((await screen.findByRole('spinbutton', { name: 'Feed metadata delay (hours)' }) as HTMLInputElement).value).toBe('0')
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
    ['/api/collector/status', '/api/collector/summary', '/api/libraries', '/api/library-deliveries'].includes(String(input)) || String(input).startsWith('/api/collector/downloads'),
  )).toBe(true)
})
