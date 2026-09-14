// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
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

function catalogRequests() {
  return fetchMock.mock.calls.filter(([input]) => String(input).startsWith('/api/collector/catalog?'))
}

beforeEach(() => {
  window.history.replaceState(null, '', '#/panda')
  catalogVersion = 1
  offline = false
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input) => {
    const url = new URL(String(input), 'http://localhost')
    if (offline) return Response.json({ error: 'collector_unreachable' }, { status: 502 })
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
  expect(catalogRequests()[0][0]).toBe('/api/collector/catalog?q=&page=1&page_size=8&include_expunged=false')

  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'p:fate' } })
  fireEvent.submit(screen.getByRole('search'))
  await screen.findByRole('heading', { name: 'p:fate 1 page 1' })
  await userEvent.click(screen.getByRole('button', { name: 'Options' }))
  await userEvent.click(screen.getByRole('menuitemcheckbox', { name: 'Include expunged' }))
  await waitFor(() => expect(catalogRequests().at(-1)?.[0]).toBe('/api/collector/catalog?q=p%3Afate&page=1&page_size=8&include_expunged=true'))
  fireEvent.click(screen.getByRole('link', { name: 'Next' }))
  await screen.findByRole('heading', { name: 'p:fate 1 page 2' })
  expect(window.location.hash).toBe('#/panda?q=p%3Afate&page=2&include_expunged=true')
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

test('reloads only on request and keeps collection tools on dedicated pages', async () => {
  render(<App />)
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  expect(fetchMock.mock.calls.map(([input]) => String(input))).toEqual(['/api/collector/catalog?q=&page=1&page_size=8&include_expunged=false'])
  expect(screen.queryByRole('textbox', { name: 'Look up gallery' })).toBeNull()
  catalogVersion = 2
  await userEvent.click(screen.getByRole('button', { name: 'Options' }))
  expect(screen.getByRole('menuitem', { name: 'Look up gallery' }).getAttribute('href')).toBe('#/panda/lookup')
  expect(screen.getByRole('menuitem', { name: 'Collect feeds' }).getAttribute('href')).toBe('#/collector/feeds')
  await userEvent.click(screen.getByRole('menuitem', { name: 'Reload results' }))
  await screen.findByRole('heading', { name: 'Upload 2 page 1' })
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
  const retryResponse = await fetchMock.getMockImplementation()!('/api/collector/catalog?q=new+search&page=1')
  let resolveRetry!: (response: Response) => void
  fetchMock.mockImplementationOnce(() => new Promise((resolve) => { resolveRetry = resolve }))
  fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(screen.getByRole('alert').textContent).toContain('Results are stale.')
  await act(async () => { resolveRetry(retryResponse) })
  await screen.findByRole('heading', { name: 'new search 1 page 1' })
  expect(screen.queryByRole('alert')).toBeNull()
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
