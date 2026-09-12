// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import App from '../App'

vi.mock('../galleries/useGalleryLayout', () => ({
  useGalleryLayout: () => ({ pageSize: 8, cardHeight: 317, viewportRef: null, cardRef: null }),
  replaceListingPage: (search: string, page: number, pageHref: (query: string, page: number) => string) => {
    window.history.replaceState(null, '', pageHref(search, page))
    window.dispatchEvent(new HashChangeEvent('hashchange'))
  },
}))

const fetchMock = vi.fn<typeof fetch>()
let catalogVersion: number
let offline: boolean
let inventoryUnavailable: boolean
let captureActive: boolean

function catalogRequests() {
  return fetchMock.mock.calls.filter(([input]) => String(input).startsWith('/api/collector/catalog?'))
}

beforeEach(() => {
  window.history.replaceState(null, '', '#/panda')
  catalogVersion = 1
  offline = false
  inventoryUnavailable = false
  captureActive = false
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input, init) => {
    const url = new URL(String(input), 'http://localhost')
    if (offline) return Response.json({ error: 'collector_unreachable' }, { status: 502 })
    if (url.pathname === '/api/collector/inventory/status') {
      if (inventoryUnavailable) return Response.json({ error: 'collector_status_unavailable' }, { status: 503 })
      return Response.json({ gallery_references: 30, metadata_available: 16, metadata_pending: 12, metadata_failed: 2, fetches_pending: 0, fetches_failed: 0 })
    }
    if (url.pathname.startsWith('/api/collector/feed/')) {
      if (init?.method === 'POST') captureActive = true
      return Response.json({ capture_active: captureActive, last_captured_at: '2026-09-12T12:00:00Z', processing_pending: 0, continuity: 'overlap', possible_gaps: 0 })
    }
    if (url.pathname === '/api/collector/catalog/completions') return Response.json({ start: 0, end: 5, items: [
      { namespace: 'parody', value: 'fate/stay night', term: 'parody:"fate/stay night$"' },
    ] })
    const page = Math.min(Number(url.searchParams.get('page')), 2)
    const query = url.searchParams.get('q')
    if (query === 'title:blue$') return Response.json({ error: 'invalid_query' }, { status: 400 })
    return Response.json({
      items: [{ gallery_id: page, title: `${query || 'Upload'} ${catalogVersion} page ${page}`, thumbnail_url: 'https://covers.example/cover.jpg', page_count: 20, posted_at: '2026-09-12T10:00:00Z', url: `https://panda.example/g/${page}/token/` }],
      total: 9, page, page_size: 8, total_pages: 2,
    })
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers() })

test('browses collected covers with local search, expunged filter, and numbered pagination', async () => {
  render(<App />)
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  expect(screen.getByRole('link', { name: 'Panda', current: 'page' })).toBeTruthy()
  const gallery = within(screen.getByRole('list', { name: 'Panda galleries' })).getByRole('link')
  expect(gallery.getAttribute('href')).toBe('https://panda.example/g/1/token/')
  expect(gallery.getAttribute('target')).toBe('_blank')
  expect(gallery.querySelector('img')?.getAttribute('src')).toBe('https://covers.example/cover.jpg')
  expect(gallery.querySelector('time')?.getAttribute('datetime')).toBe('2026-09-12T10:00:00Z')
  expect(screen.getByText('9 galleries')).toBeTruthy()
  expect(screen.getByText(/12 metadata pending · 2 failed/)).toBeTruthy()
  expect(fetchMock.mock.calls.some(([url]) => url === '/api/collector/inventory/status')).toBe(true)
  expect(fetchMock.mock.calls.some(([url]) => url === '/api/collector/status')).toBe(false)
  expect(catalogRequests()[0][0]).toBe('/api/collector/catalog?q=&page=1&page_size=8&include_expunged=false')

  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'p:fate' } })
  fireEvent.submit(screen.getByRole('search'))
  await screen.findByRole('heading', { name: 'p:fate 1 page 1' })
  fireEvent.click(screen.getByRole('checkbox', { name: 'Include expunged' }))
  await waitFor(() => expect(catalogRequests().at(-1)?.[0]).toBe('/api/collector/catalog?q=p%3Afate&page=1&page_size=8&include_expunged=true'))
  fireEvent.click(screen.getByRole('link', { name: 'Next' }))
  await screen.findByRole('heading', { name: 'p:fate 1 page 2' })
  expect(window.location.hash).toBe('#/panda?q=p%3Afate&page=2&include_expunged=true')
})

test('inventory failure leaves feed status and capture available', async () => {
  inventoryUnavailable = true
  render(<App />)
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  expect((await screen.findByRole('alert')).textContent).toContain('its status API is unavailable')
  expect(screen.getByText(/Feed processing complete/)).toBeTruthy()
  expect(screen.getByText(/Latest feed overlaps earlier captures/)).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Refresh feed' }))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Capturing feed…' }).hasAttribute('disabled')).toBe(true))
})

test('Panda autocomplete uses the collected vocabulary and preserves raw tag characters', async () => {
  render(<App />)
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  const input = screen.getByRole('combobox') as HTMLInputElement
  fireEvent.focus(input)
  fireEvent.change(input, { target: { value: 'p:fat' } })
  fireEvent.click(await screen.findByRole('option'))
  expect(input.value).toBe('parody:"fate/stay night$"')
  expect(fetchMock.mock.calls.some(([url]) => String(url) === '/api/collector/catalog/completions?q=p%3Afat&cursor=5')).toBe(true)
  expect(catalogRequests()).toHaveLength(1)
})

test('feed refresh and status polling leave results unchanged until explicit reload', async () => {
  vi.useFakeTimers()
  render(<App />)
  await act(async () => {})
  expect(screen.getByRole('heading', { name: 'Upload 1 page 1' })).toBeTruthy()
  catalogVersion = 2
  fireEvent.click(screen.getByRole('button', { name: 'Refresh feed' }))
  await act(async () => {})
  expect(screen.getByRole('button', { name: 'Capturing feed…' }).hasAttribute('disabled')).toBe(true)
  expect(fetchMock.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(1)
  captureActive = false
  await act(async () => { await vi.advanceTimersByTimeAsync(5000) })
  expect(screen.getByRole('button', { name: 'Refresh feed' })).toBeTruthy()
  expect(screen.getByRole('heading', { name: 'Upload 1 page 1' })).toBeTruthy()
  expect(catalogRequests()).toHaveLength(1)
  fireEvent.click(screen.getByRole('button', { name: 'Reload results' }))
  await act(async () => {})
  expect(screen.getByRole('heading', { name: 'Upload 2 page 1' })).toBeTruthy()
  expect(catalogRequests()).toHaveLength(2)
})

test('retains results during outages and failed searches, retries, and shows unavailable on a fresh visit', async () => {
  render(<App />)
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  offline = true
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'new search' } })
  fireEvent.submit(screen.getByRole('search'))
  expect((await screen.findByRole('alert')).textContent).toContain('Results are stale.')
  expect(screen.getByRole('heading', { name: 'Upload 1 page 1' })).toBeTruthy()
  offline = false
  fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
  await screen.findByRole('heading', { name: 'new search 1 page 1' })
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'title:blue$' } })
  fireEvent.submit(screen.getByRole('search'))
  expect((await screen.findByRole('alert')).textContent).toContain('Invalid search query.')
  expect((screen.getByRole('combobox') as HTMLInputElement).value).toBe('title:blue$')
  expect(screen.getByRole('heading', { name: 'new search 1 page 1' })).toBeTruthy()
  cleanup()
  offline = true
  render(<App />)
  expect((await screen.findByRole('alert')).textContent).toContain('Panda catalog unavailable.')
  expect(screen.queryByRole('list', { name: 'Panda galleries' })).toBeNull()
})
