// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import App from '../App'

vi.mock('../galleries/useGalleryLayout', () => ({
  useGalleryGrid: () => ({ pageSize: 8, cardHeight: 317, viewportRef: null, cardRef: null }),
  useGalleryLayout: () => ({ pageSize: 8, cardHeight: 317, viewportRef: null, cardRef: null }),
  replaceListingPage: (search: string, page: number, pageHref: (query: string, page: number) => string) => {
    window.history.replaceState(null, '', pageHref(search, page))
    window.dispatchEvent(new HashChangeEvent('hashchange'))
  },
}))

const fetchMock = vi.fn<typeof fetch>()
let catalogVersion: number
let offline: boolean
let defaultFilter: { query: string, categories: string[] }

function catalogRequests() {
  return fetchMock.mock.calls.filter(([input]) => String(input).startsWith('/api/collector/catalog?'))
}

beforeEach(() => {
  window.history.replaceState(null, '', '#/panda')
  catalogVersion = 1
  offline = false
  defaultFilter = { query: '', categories: [] }
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input, init) => {
    const url = new URL(String(input), 'http://localhost')
    if (url.pathname === '/api/panda/default-filter') {
      if (init?.method === 'PUT') defaultFilter = JSON.parse(String(init.body))
      return Response.json(defaultFilter)
    }
    if (offline) return Response.json({ error: 'collector_unreachable' }, { status: 502 })
    if (url.pathname === '/api/collector/catalog/completions') return Response.json({ start: 0, end: 5, items: [
      { namespace: 'parody', value: 'fate/stay night', term: 'parody:"fate/stay night$"' },
    ] })
    const page = url.searchParams.get('cursor') === 'next-token' ? 2 : 1
    const query = url.searchParams.get('q')
    if (query === 'title:blue$') return Response.json({ error: 'invalid_query' }, { status: 400 })
    return Response.json({
      items: [{ gallery_id: page, title: `${query || 'Upload'} ${catalogVersion} page ${page}`, thumbnail_url: 'https://covers.example/cover.jpg', page_count: 20, posted_at: '2026-09-12T10:00:00Z', url: `https://panda.example/g/${page}/token/` }],
      page_size: 8, next_cursor: page === 1 ? 'next-token' : undefined, previous_cursor: page === 2 ? 'previous-token' : undefined,
    })
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers() })

test('browses collected covers with local search, expunged filter, and cursor pagination', async () => {
  render(<App />)
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  expect(screen.getByRole('link', { name: 'Panda', current: 'page' })).toBeTruthy()
  const gallery = within(screen.getByRole('list', { name: 'Panda galleries' })).getByRole('link')
  expect(gallery.getAttribute('href')).toBe('https://panda.example/g/1/token/')
  expect(gallery.getAttribute('target')).toBe('_blank')
  expect(gallery.querySelector('img')?.getAttribute('src')).toBe('https://covers.example/cover.jpg')
  expect(gallery.querySelector('time')?.getAttribute('datetime')).toBe('2026-09-12T10:00:00Z')
  expect(screen.getByRole('button', { name: 'Previous' }).hasAttribute('disabled')).toBe(true)
  expect(catalogRequests()[0][0]).toBe('/api/collector/catalog?q=&cursor=&page_size=8&include_expunged=false')

  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'p:fate' } })
  fireEvent.submit(screen.getByRole('search'))
  await screen.findByRole('heading', { name: 'p:fate 1 page 1' })
  await userEvent.click(screen.getByRole('button', { name: 'Options' }))
  await userEvent.click(screen.getByRole('menuitemcheckbox', { name: 'Include expunged' }))
  await waitFor(() => expect(catalogRequests().at(-1)?.[0]).toBe('/api/collector/catalog?q=p%3Afate&cursor=&page_size=8&include_expunged=true'))
  fireEvent.click(screen.getByRole('link', { name: 'Next' }))
  await screen.findByRole('heading', { name: 'p:fate 1 page 2' })
  expect(window.location.hash).toBe('#/panda?q=p%3Afate&cursor=next-token&include_expunged=true')
  expect(screen.getByRole('button', { name: 'Next' }).hasAttribute('disabled')).toBe(true)
  // Cursor links work after reloading, without an in-memory history of pages.
  cleanup()
  render(<App />)
  await screen.findByRole('heading', { name: 'p:fate 1 page 2' })
  fireEvent.click(screen.getByRole('link', { name: 'Previous' }))
  await screen.findByRole('heading', { name: 'p:fate 1 page 1' })
  expect(window.location.hash).toBe('#/panda?q=p%3Afate&cursor=previous-token&include_expunged=true')
  fireEvent.change(screen.getByRole('combobox'), { target: { value: '-language:translated' } })
  fireEvent.submit(screen.getByRole('search'))
  await screen.findByRole('heading', { name: '-language:translated 1 page 1' })
  expect(new URLSearchParams(window.location.hash.split('?')[1]).has('cursor')).toBe(false)
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
  expect(catalogRequests().map(([input]) => String(input))).toEqual(['/api/collector/catalog?q=&cursor=&page_size=8&include_expunged=false'])
  expect(screen.queryByRole('textbox', { name: 'Look up gallery' })).toBeNull()
  catalogVersion = 2
  await userEvent.click(screen.getByRole('button', { name: 'Options' }))
  expect(screen.getByRole('menuitem', { name: 'Look up gallery' }).getAttribute('href')).toBe('#/panda/lookup')
  expect(screen.getByRole('menuitem', { name: 'Collect feeds' }).getAttribute('href')).toBe('#/collector/feeds')
  await userEvent.click(screen.getByRole('menuitem', { name: 'Reload results' }))
  await screen.findByRole('heading', { name: 'Upload 2 page 1' })
  expect(catalogRequests()).toHaveLength(2)
})

test('edits a shared default separately from the current query and applies it only on save', async () => {
  defaultFilter = { query: '-l:japanese$', categories: ['manga'] }
  window.history.replaceState(null, '', '#/panda?q=artist%3Aname&cursor=next-token&category=doujinshi')
  render(<App />)
  await screen.findByRole('heading', { name: 'artist:name 1 page 2' })
  expect(screen.getByRole('button', { name: 'Default filter Active' })).toBeTruthy()
  expect(screen.queryByLabelText('Default search')).toBeNull()

  await userEvent.click(screen.getByRole('button', { name: 'Default filter Active' }))
  expect((screen.getByLabelText('Default search') as HTMLInputElement).value).toBe('-l:japanese$')
  fireEvent.change(screen.getByLabelText('Default search'), { target: { value: '-l:chinese$' } })
  await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)

  await userEvent.click(screen.getByRole('button', { name: 'Default filter Active' }))
  expect((screen.getByLabelText('Default search') as HTMLInputElement).value).toBe('-l:japanese$')
  fireEvent.change(screen.getByLabelText('Default search'), { target: { value: '-l:japanese$ -l:chinese$' } })
  await userEvent.click(screen.getByRole('checkbox', { name: 'Western' }))
  await userEvent.click(screen.getByRole('button', { name: 'Save default filter' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(defaultFilter).toEqual({ query: '-l:japanese$ -l:chinese$', categories: ['manga', 'western'] })
  await waitFor(() => expect(catalogRequests()).toHaveLength(2))
  expect(catalogRequests().at(-1)?.[0]).toBe('/api/collector/catalog?q=artist%3Aname&cursor=&page_size=8&include_expunged=false&category=doujinshi')
  expect((screen.getByRole('combobox') as HTMLInputElement).value).toBe('artist:name')
})

test('preserves category and default bypass URL filters across search, paging, back, and clear', async () => {
  defaultFilter = { query: '-l:japanese$', categories: ['manga', 'doujinshi'] }
  window.history.replaceState(null, '', '#/panda?category=manga&category=doujinshi&include_expunged=true')
  render(<App />)
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  await userEvent.click(screen.getByRole('link', { name: 'Bypass default' }))
  await screen.findByRole('button', { name: 'Default filter Bypassed' })
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'new search' } })
  fireEvent.submit(screen.getByRole('search'))
  await screen.findByRole('heading', { name: 'new search 1 page 1' })
  fireEvent.keyDown(window, { key: 'ArrowRight' })
  await screen.findByRole('heading', { name: 'new search 1 page 2' })
  expect(window.location.hash).toBe('#/panda?q=new+search&cursor=next-token&include_expunged=true&category=manga&category=doujinshi&bypass_default=true')
  act(() => window.history.back())
  await waitFor(() => expect(window.location.hash).toBe('#/panda?q=new+search&include_expunged=true&category=manga&category=doujinshi&bypass_default=true'))
  await screen.findByRole('heading', { name: 'new search 1 page 1' })
  await userEvent.click(screen.getByRole('link', { name: 'Clear all filters' }))
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  expect(window.location.hash).toBe('#/panda?bypass_default=true')
  expect(catalogRequests().at(-1)?.[0]).toBe('/api/collector/catalog?q=&cursor=&page_size=8&include_expunged=false&bypass_default=true')
  expect(defaultFilter).toEqual({ query: '-l:japanese$', categories: ['manga', 'doujinshi'] })
  await userEvent.click(screen.getByRole('link', { name: 'Apply default' }))
  await screen.findByRole('button', { name: 'Default filter Active' })
  await waitFor(() => expect(catalogRequests().at(-1)?.[0]).toBe('/api/collector/catalog?q=&cursor=&page_size=8&include_expunged=false'))
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
  const retryResponse = await fetchMock.getMockImplementation()!('/api/collector/catalog?q=new+search&cursor=')
  let resolveRetry!: (response: Response) => void
  const normalFetch = fetchMock.getMockImplementation()!
  fetchMock.mockImplementation((input, init) => String(input).startsWith('/api/collector/catalog?')
    ? new Promise((resolve) => { resolveRetry = resolve }) : normalFetch(input, init))
  fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(screen.getByRole('alert').textContent).toContain('Results are stale.')
  await act(async () => { resolveRetry(retryResponse) })
  fetchMock.mockImplementation(normalFetch)
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
