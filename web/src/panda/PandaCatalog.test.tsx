// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import App from '../App'

vi.mock('../collector/FavoriteSyncAction', () => ({ default: () => null }))

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
  window.history.replaceState(null, '', '#/panda?unseen=false')
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
      items: [{ gallery_id: page, title: `${query || 'Upload'} ${catalogVersion} page ${page}`, thumbnail_url: 'https://covers.example/cover.jpg', page_count: 20, posted_at: '2026-09-12T10:00:00Z', url: `https://panda.example/g/${page}/token/`, summary_tags: [] }],
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
  expect(window.location.hash).toBe('#/panda?q=p%3Afate&cursor=next-token&include_expunged=true&unseen=false')
  expect(screen.getByRole('button', { name: 'Next' }).hasAttribute('disabled')).toBe(true)
  // Cursor links work after reloading, without an in-memory history of pages.
  cleanup()
  render(<App />)
  await screen.findByRole('heading', { name: 'p:fate 1 page 2' })
  fireEvent.click(screen.getByRole('link', { name: 'Previous' }))
  await screen.findByRole('heading', { name: 'p:fate 1 page 1' })
  expect(window.location.hash).toBe('#/panda?q=p%3Afate&cursor=previous-token&include_expunged=true&unseen=false')
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

test('links the score model page from the Options menu', async () => {
  render(<App />)
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  await userEvent.click(screen.getByRole('button', { name: 'Options' }))
  expect(screen.getByRole('menuitem', { name: 'Score model' }).getAttribute('href')).toBe('#/panda/score')
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
  window.history.replaceState(null, '', '#/panda?q=artist%3Aname&cursor=next-token&category=doujinshi&unseen=false')
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
  window.history.replaceState(null, '', '#/panda?category=manga&category=doujinshi&include_expunged=true&unseen=false')
  render(<App />)
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  await userEvent.click(screen.getByRole('link', { name: 'Bypass default' }))
  await screen.findByRole('button', { name: 'Default filter Bypassed' })
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'new search' } })
  fireEvent.submit(screen.getByRole('search'))
  await screen.findByRole('heading', { name: 'new search 1 page 1' })
  fireEvent.keyDown(window, { key: 'ArrowRight' })
  await screen.findByRole('heading', { name: 'new search 1 page 2' })
  expect(window.location.hash).toBe('#/panda?q=new+search&cursor=next-token&include_expunged=true&category=manga&category=doujinshi&bypass_default=true&unseen=false')
  act(() => window.history.back())
  await waitFor(() => expect(window.location.hash).toBe('#/panda?q=new+search&include_expunged=true&category=manga&category=doujinshi&bypass_default=true&unseen=false'))
  await screen.findByRole('heading', { name: 'new search 1 page 1' })
  await userEvent.click(screen.getByRole('link', { name: 'Clear all filters' }))
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  expect(window.location.hash).toBe('#/panda?bypass_default=true&unseen=false')
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

test('a plain click on Next pages in place, and a modified click keeps the native link', async () => {
  render(<App />)
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  const next = screen.getByRole('link', { name: 'Next' })
  expect(next.getAttribute('href')).toBe('#/panda?cursor=next-token&unseen=false')
  expect(fireEvent.click(next)).toBe(false)
  await screen.findByRole('heading', { name: 'Upload 1 page 2' })
  expect(window.location.hash).toBe('#/panda?cursor=next-token&unseen=false')
  fireEvent.click(screen.getByRole('link', { name: 'Previous' }))
  await screen.findByRole('heading', { name: 'Upload 1 page 1' })
  expect(fireEvent.click(screen.getByRole('link', { name: 'Next' }), { ctrlKey: true })).toBe(true)
  // The native navigation lands after the click; let it finish here instead of in the next test.
  await waitFor(() => expect(window.location.hash).toBe('#/panda?cursor=next-token&unseen=false'))
})

const pageIDs: Record<string, number[]> = { '': [5, 3, 4], 'previous-token': [5, 3, 4], 'next-token': [2, 1], 'empty-token': [] }
const nextCursors: Record<string, string> = { '': 'next-token', 'previous-token': 'next-token', 'next-token': 'last-token', 'empty-token': 'next-token' }

function displayRequests() {
  return fetchMock.mock.calls.filter(([input]) => String(input) === '/api/collector/catalog/displays')
}

function recordedIDs() {
  return displayRequests().map(([, init]) => JSON.parse(String(init?.body)).gallery_ids as number[])
}

/** Serves multi-gallery pages and hands each record request to `record`. */
function serveRecords(record: () => Promise<Response>) {
  fetchMock.mockImplementation(async (input) => {
    const url = new URL(String(input), 'http://localhost')
    if (url.pathname === '/api/panda/default-filter') return Response.json(defaultFilter)
    if (url.pathname === '/api/collector/catalog/displays') return record()
    if (url.pathname !== '/api/collector/catalog') return Response.json({ error: 'not_found' }, { status: 404 })
    const cursor = url.searchParams.get('cursor') ?? ''
    const query = url.searchParams.get('q') || 'Upload'
    const ids = pageIDs[cursor] ?? []
    return Response.json({
      items: ids.map((id) => ({ gallery_id: id, title: `${query} ${cursor || 'first'} ${id}`, thumbnail_url: '', page_count: 20, posted_at: '2026-09-12T10:00:00Z', metadata_source: 'collected', url: `https://panda.example/g/${id}/token/`, summary_tags: [] })),
      page_size: 8, next_cursor: nextCursors[cursor], previous_cursor: cursor ? 'previous-token' : undefined,
    })
  })
}

/** Renders the catalog at `hash` once a native link click of an earlier test has finished navigating. */
async function renderAt(hash = '#/panda?unseen=false') {
  await new Promise((resolve) => setTimeout(resolve, 0))
  window.history.replaceState(null, '', hash)
  render(<App />)
}

function holdRecords() {
  const pending: { resolve: (response: Response) => void }[] = []
  serveRecords(() => new Promise((resolve) => { pending.push({ resolve }) }))
  return pending
}

test('Next and its shortcuts record the displayed galleries in page order without waiting', async () => {
  const pending = holdRecords()
  await renderAt()
  await screen.findByRole('heading', { name: 'Upload first 4' })
  fireEvent.click(screen.getByRole('link', { name: 'Next' }))
  expect(window.location.hash).toBe('#/panda?cursor=next-token&unseen=false')
  await screen.findByRole('heading', { name: 'Upload next-token 1' })
  expect(pending).toHaveLength(1)
  expect(displayRequests()[0][1]).toMatchObject({ method: 'POST', keepalive: true, headers: { 'Content-Type': 'application/json' } })
  expect(recordedIDs()).toEqual([[5, 3, 4]])
  await act(async () => { pending[0].resolve(new Response(null, { status: 204 })) })

  fireEvent.click(screen.getByRole('link', { name: 'Previous' }))
  await screen.findByRole('heading', { name: 'Upload previous-token 5' })
  fireEvent.keyDown(window, { key: 'ArrowRight' })
  await screen.findByRole('heading', { name: 'Upload next-token 2' })
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  await screen.findByRole('heading', { name: 'Upload previous-token 5' })
  fireEvent.keyDown(window, { key: 'd' })
  await screen.findByRole('heading', { name: 'Upload next-token 2' })
  fireEvent.keyDown(window, { key: 'a' })
  await screen.findByRole('heading', { name: 'Upload previous-token 5' })
  expect(recordedIDs()).toEqual([[5, 3, 4], [5, 3, 4], [5, 3, 4]])

  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'p:fate' } })
  fireEvent.submit(screen.getByRole('search'))
  await screen.findByRole('heading', { name: 'p:fate first 5' })
  fireEvent.click(screen.getByRole('link', { name: 'Next' }))
  expect(window.location.hash).toBe('#/panda?q=p%3Afate&cursor=next-token&unseen=false')
  await screen.findByRole('heading', { name: 'p:fate next-token 2' })
  fireEvent.click(screen.getByRole('link', { name: 'Next' }))
  await waitFor(() => expect(window.location.hash).toBe('#/panda?q=p%3Afate&cursor=last-token&unseen=false'))
  expect(recordedIDs()).toEqual([[5, 3, 4], [5, 3, 4], [5, 3, 4], [5, 3, 4], [2, 1]])
})

test('the paging keys work with the focus on a card and record the page', async () => {
  serveRecords(async () => new Response(null, { status: 204 }))
  await renderAt()
  const card = await screen.findByRole('link', { name: /Upload first 5/ })
  card.focus()
  fireEvent.keyDown(card, { key: 'ArrowRight' })
  await screen.findByRole('heading', { name: 'Upload next-token 2' })
  expect(recordedIDs()).toEqual([[5, 3, 4]])
  const next = screen.getByRole('link', { name: /Upload next-token 2/ })
  next.focus()
  fireEvent.keyDown(next, { key: 'a' })
  await screen.findByRole('heading', { name: 'Upload previous-token 5' })
})

test('only going to the next page records', async () => {
  serveRecords(async () => new Response(null, { status: 204 }))
  await renderAt('#/panda?cursor=next-token&unseen=false')
  await screen.findByRole('heading', { name: 'Upload next-token 2' })
  fireEvent.click(screen.getByRole('link', { name: 'Previous' }))
  await screen.findByRole('heading', { name: 'Upload previous-token 5' })
  await userEvent.click(screen.getByRole('button', { name: 'Options' }))
  await userEvent.click(screen.getByRole('menuitem', { name: 'Reload results' }))
  await waitFor(() => expect(catalogRequests()).toHaveLength(3))
  await screen.findByRole('link', { name: 'Next' })
  const card = within(screen.getByRole('list', { name: 'Panda galleries' })).getAllByRole('link')[0]
  card.addEventListener('click', (event) => event.preventDefault())
  fireEvent.click(card)
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'p:fate' } })
  fireEvent.submit(screen.getByRole('search'))
  await screen.findByRole('heading', { name: 'p:fate first 5' })
  await userEvent.click(screen.getByRole('button', { name: 'Categories' }))
  await userEvent.click(screen.getByRole('menuitemcheckbox', { name: 'Manga' }))
  await waitFor(() => expect(window.location.hash).toContain('category=manga'))
  await userEvent.keyboard('{Escape}')
  await userEvent.click(screen.getByRole('button', { name: /^Options/ }))
  await userEvent.click(screen.getByRole('menuitemcheckbox', { name: 'Include expunged' }))
  await waitFor(() => expect(window.location.hash).toContain('include_expunged=true'))
  // Modified and middle clicks are left to the browser's native link handling.
  for (const init of [{ ctrlKey: true }, { metaKey: true }, { shiftKey: true }, { button: 1 }]) {
    fireEvent.click(await screen.findByRole('link', { name: 'Next' }), init)
  }
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(displayRequests()).toHaveLength(0)
})

test('Next on a page without galleries moves on without a record', async () => {
  serveRecords(async () => new Response(null, { status: 204 }))
  await renderAt('#/panda?cursor=empty-token&unseen=false')
  fireEvent.click(await screen.findByRole('link', { name: 'Next' }))
  await screen.findByRole('heading', { name: 'Upload next-token 2' })
  expect(displayRequests()).toHaveLength(0)
})

test('a failed record shows an alert until a later record succeeds, in settle order', async () => {
  const pending = holdRecords()
  await renderAt()
  await screen.findByRole('heading', { name: 'Upload first 5' })
  fireEvent.click(screen.getByRole('link', { name: 'Next' }))
  await screen.findByRole('heading', { name: 'Upload next-token 2' })
  await act(async () => { pending[0].resolve(Response.json({ error: 'collector_unavailable' }, { status: 502 })) })
  const alert = await screen.findByRole('alert')
  expect(alert.textContent).toContain('Could not mark a page as seen. Its galleries stay unseen.')
  expect(alert.textContent).toContain('The request could not be confirmed. Check collector status and try again.')
  expect(within(alert).queryByRole('button')).toBeNull()

  fireEvent.click(await screen.findByRole('link', { name: 'Previous' }))
  await screen.findByRole('heading', { name: 'Upload previous-token 5' })
  fireEvent.click(await screen.findByRole('link', { name: 'Next' }))
  await screen.findByRole('heading', { name: 'Upload next-token 2' })
  await act(async () => { pending[1].resolve(new Response(null, { status: 204 })) })
  await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())

  // The left page's record fails after the following page's record succeeded.
  fireEvent.click(await screen.findByRole('link', { name: 'Previous' }))
  await screen.findByRole('heading', { name: 'Upload previous-token 5' })
  fireEvent.click(await screen.findByRole('link', { name: 'Next' }))
  await screen.findByRole('heading', { name: 'Upload next-token 2' })
  fireEvent.click(await screen.findByRole('link', { name: 'Next' }))
  await waitFor(() => expect(pending).toHaveLength(4))
  await act(async () => { pending[3].resolve(new Response(null, { status: 204 })) })
  await act(async () => { pending[2].resolve(Response.json({ error: 'collector_unavailable' }, { status: 502 })) })
  expect((await screen.findByRole('alert')).textContent).toContain('Could not mark a page as seen.')
})

test('leaving the Panda catalog drops the record-failure alert', async () => {
  serveRecords(async () => Response.json({ error: 'collector_unavailable' }, { status: 502 }))
  await renderAt()
  await screen.findByRole('heading', { name: 'Upload first 5' })
  fireEvent.click(screen.getByRole('link', { name: 'Next' }))
  expect((await screen.findByRole('alert')).textContent).toContain('Could not mark a page as seen.')
  act(() => { window.location.hash = '#/panda/lookup' })
  await screen.findByRole('heading', { name: 'Gallery lookup' })
  act(() => { window.location.hash = '#/panda?cursor=next-token&unseen=false' })
  await screen.findByRole('heading', { name: 'Upload next-token 2' })
  expect(screen.queryByRole('alert')).toBeNull()
})

/** Serves both views: `q=none` and `unseenEmpty` give empty pages, and `last-token` is the empty end of every listing. */
function serveViews({ unseenEmpty = false, unreachable = false } = {}) {
  fetchMock.mockImplementation(async (input, init) => {
    const url = new URL(String(input), 'http://localhost')
    if (url.pathname === '/api/panda/default-filter') {
      if (init?.method === 'PUT') defaultFilter = JSON.parse(String(init.body))
      return Response.json(defaultFilter)
    }
    if (url.pathname === '/api/collector/catalog/displays') return new Response(null, { status: 204 })
    if (url.pathname !== '/api/collector/catalog') return Response.json({ error: 'not_found' }, { status: 404 })
    if (unreachable || offline) return Response.json({ error: 'collector_unreachable' }, { status: 502 })
    const cursor = url.searchParams.get('cursor') ?? ''
    const query = url.searchParams.get('q') || 'Upload'
    const unseen = url.searchParams.get('unseen') === 'true'
    const ids = query === 'none' || (unseen && unseenEmpty) ? [] : ({ '': [5, 3], 'next-token': [2, 1] } as Record<string, number[]>)[cursor] ?? []
    return Response.json({
      items: ids.map((id) => ({ gallery_id: id, title: `${query} ${unseen ? 'unseen' : 'all'} ${cursor || 'first'} ${id}`, thumbnail_url: '', page_count: 20, posted_at: '2026-09-12T10:00:00Z', metadata_source: 'collected', url: `https://panda.example/g/${id}/token/`, summary_tags: [] })),
      page_size: 8, next_cursor: ids.length ? (cursor ? 'last-token' : 'next-token') : undefined,
    })
  })
}

test('the view control switches views from any page without recording, and #/panda opens unseen galleries', async () => {
  serveViews()
  await renderAt('#/panda?q=s&cursor=next-token&category=manga&unseen=false')
  await screen.findByRole('heading', { name: 's all next-token 2' })
  expect(screen.getByRole('radio', { name: 'All' }).getAttribute('aria-checked')).toBe('true')
  expect(screen.getByRole('radio', { name: 'Unseen' }).getAttribute('aria-checked')).toBe('false')
  expect(new URL(String(catalogRequests()[0][0]), 'http://localhost').searchParams.has('unseen')).toBe(false)
  expect(screen.getByRole('link', { name: 'Panda', current: 'page' }).getAttribute('href')).toBe('#/panda')

  await userEvent.click(screen.getByRole('radio', { name: 'Unseen' }))
  expect(window.location.hash).toBe('#/panda?q=s&category=manga')
  await screen.findByRole('heading', { name: 's unseen first 5' })
  expect(String(catalogRequests().at(-1)?.[0])).toMatch(/&unseen=true$/)
  expect(screen.getByRole('radio', { name: 'Unseen' }).getAttribute('aria-checked')).toBe('true')

  act(() => { window.location.hash = '#/panda?q=s&cursor=next-token&category=manga' })
  await screen.findByRole('heading', { name: 's unseen next-token 2' })
  await userEvent.click(screen.getByRole('radio', { name: 'All' }))
  expect(window.location.hash).toBe('#/panda?q=s&category=manga&unseen=false')
  await screen.findByRole('heading', { name: 's all first 5' })
  expect(displayRequests()).toHaveLength(0)
  expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)

  cleanup()
  await renderAt('#/panda')
  await screen.findByRole('heading', { name: 'Upload unseen first 5' })
  expect(screen.getByRole('radio', { name: 'Unseen' }).getAttribute('aria-checked')).toBe('true')
  const viewControl = within(screen.getByRole('radiogroup', { name: 'View' }))
  expect(viewControl.getAllByRole('radio')).toEqual(['All', 'Unseen'].map((name) => viewControl.getByRole('radio', { name })))
  expect(catalogRequests().at(-1)?.[0]).toBe('/api/collector/catalog?q=&cursor=&page_size=8&include_expunged=false&unseen=true')
})

test('every listing change in the unseen view stays in the unseen view', async () => {
  serveViews()
  defaultFilter = { query: '-l:japanese$', categories: [] }
  await renderAt('#/panda')
  await screen.findByRole('heading', { name: 'Upload unseen first 5' })
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'p:fate' } })
  fireEvent.submit(screen.getByRole('search'))
  await screen.findByRole('heading', { name: 'p:fate unseen first 5' })
  expect(window.location.hash).toBe('#/panda?q=p%3Afate')

  await userEvent.click(screen.getByRole('button', { name: 'Categories' }))
  await userEvent.click(screen.getByRole('menuitemcheckbox', { name: 'Manga' }))
  await waitFor(() => expect(window.location.hash).toBe('#/panda?q=p%3Afate&category=manga'))
  await userEvent.keyboard('{Escape}')

  await userEvent.click(screen.getByRole('link', { name: 'Bypass default' }))
  await waitFor(() => expect(window.location.hash).toBe('#/panda?q=p%3Afate&category=manga&bypass_default=true'))
  await userEvent.click(await screen.findByRole('link', { name: 'Apply default' }))
  await waitFor(() => expect(window.location.hash).toBe('#/panda?q=p%3Afate&category=manga'))

  await screen.findByRole('heading', { name: 'p:fate unseen first 5' })
  await userEvent.click(screen.getByRole('button', { name: /^Options/ }))
  await userEvent.click(screen.getByRole('menuitemcheckbox', { name: 'Include expunged' }))
  await waitFor(() => expect(window.location.hash).toBe('#/panda?q=p%3Afate&include_expunged=true&category=manga'))

  await screen.findByRole('heading', { name: 'p:fate unseen first 5' })
  fireEvent.click(await screen.findByRole('link', { name: 'Next' }))
  expect(window.location.hash).toBe('#/panda?q=p%3Afate&cursor=next-token&include_expunged=true&category=manga')
  await screen.findByRole('heading', { name: 'p:fate unseen next-token 2' })

  offline = true
  await userEvent.click(screen.getByRole('button', { name: /^Options/ }))
  await userEvent.click(screen.getByRole('menuitem', { name: 'Reload results' }))
  expect((await screen.findByRole('alert')).textContent).toContain('Results are stale.')
  expect(screen.getByRole('link', { name: 'Newest galleries' }).getAttribute('href')).toBe('#/panda?q=p%3Afate&include_expunged=true&category=manga')
  offline = false

  await userEvent.click(screen.getByRole('link', { name: 'Clear all filters' }))
  await waitFor(() => expect(window.location.hash).toBe('#/panda?bypass_default=true'))
  await screen.findByRole('heading', { name: 'Upload unseen first 5' })
})

test('Next in the unseen view records the page and ends in an empty state with a way on', async () => {
  serveViews()
  await renderAt('#/panda?cursor=next-token')
  await screen.findByRole('heading', { name: 'Upload unseen next-token 2' })
  fireEvent.click(screen.getByRole('link', { name: 'Next' }))
  expect(window.location.hash).toBe('#/panda?cursor=last-token')
  await screen.findByRole('heading', { name: 'No more unseen Panda galleries.' })
  expect(recordedIDs()).toEqual([[2, 1]])
  expect(screen.getByText('Galleries that arrived during this visit wait with the newest unseen galleries.')).toBeTruthy()
  expect(screen.getByRole('link', { name: 'Newest unseen galleries' }).getAttribute('href')).toBe('#/panda')
  expect(screen.getByRole('link', { name: 'Show all galleries' }).getAttribute('href')).toBe('#/panda?unseen=false')
  expect(screen.getByRole('button', { name: 'Next' }).hasAttribute('disabled')).toBe(true)
  expect(screen.getByRole('button', { name: 'Previous' }).hasAttribute('disabled')).toBe(true)
  expect(screen.queryByRole('alert')).toBeNull()

  act(() => { window.location.hash = '#/panda?q=s&cursor=last-token&category=manga' })
  await screen.findByRole('heading', { name: 'No more unseen Panda galleries match.' })
  expect(screen.getByRole('link', { name: 'Newest unseen galleries' }).getAttribute('href')).toBe('#/panda?q=s&category=manga')
  expect(screen.getByRole('link', { name: 'Show all galleries' }).getAttribute('href')).toBe('#/panda?q=s&category=manga&unseen=false')
  expect(screen.queryByText('No Panda galleries yet.')).toBeNull()
})

test('an empty unseen view says whether a search or filter narrows it', async () => {
  serveViews({ unseenEmpty: true })
  await renderAt('#/panda')
  await screen.findByRole('heading', { name: 'No unseen Panda galleries.' })
  expect(screen.getByText('You have seen every Panda gallery. New galleries appear here once feeds are collected.')).toBeTruthy()
  expect(screen.getByRole('link', { name: 'Show all galleries' }).getAttribute('href')).toBe('#/panda?unseen=false')
  expect(screen.queryByText('No Panda galleries yet.')).toBeNull()

  act(() => { window.location.hash = '#/panda?q=s' })
  await screen.findByRole('heading', { name: 'No unseen Panda galleries match.' })
  expect(screen.getByText('Try different titles, tags, or categories, or show all galleries.')).toBeTruthy()
  expect(screen.getByRole('link', { name: 'Show all galleries' }).getAttribute('href')).toBe('#/panda?q=s&unseen=false')

  cleanup()
  defaultFilter = { query: '-l:japanese$', categories: [] }
  await renderAt('#/panda')
  await screen.findByRole('heading', { name: 'No unseen Panda galleries match.' })
  expect(screen.queryByText('No Panda galleries yet.')).toBeNull()
})

test('a fresh visit to the unseen view with the collector unreachable shows the catalog unavailable', async () => {
  serveViews({ unreachable: true })
  await renderAt('#/panda')
  expect((await screen.findByRole('alert')).textContent).toContain('Panda catalog unavailable.')
  expect(screen.queryByRole('list', { name: 'Panda galleries' })).toBeNull()
})

const newestDate = new Date('2026-09-12T10:00:00Z').toLocaleString()

function catchUpRequests() {
  return fetchMock.mock.calls.filter(([input]) => String(input) === '/api/collector/catalog/catch-up')
}

/** Serves `serveViews` pages, a failing catalog while `catalogFails` is set, and hands each catch-up request to `catchUp`. */
function serveCatchUp(catchUp: () => Promise<Response> = async () => new Response(null, { status: 204 }), state = { catalogFails: false }) {
  serveViews()
  const views = fetchMock.getMockImplementation()!
  fetchMock.mockImplementation(async (input, init) => {
    const url = new URL(String(input), 'http://localhost')
    if (url.pathname === '/api/collector/catalog/catch-up') return catchUp()
    if (url.pathname === '/api/collector/catalog' && state.catalogFails) return Response.json({ error: 'collector_unreachable' }, { status: 502 })
    return views(input, init)
  })
  return state
}

function holdCatchUps() {
  const pending: { resolve: (response: Response) => void }[] = []
  serveCatchUp(() => new Promise((resolve) => { pending.push({ resolve }) }))
  return pending
}

async function openCatchUp() {
  await userEvent.click(screen.getByRole('button', { name: 'Catch up…' }))
  return screen.findByRole('alertdialog', { name: 'Catch up?' })
}

test('Catch up… opens a confirmation in the unseen view only, and Cancel sends nothing', async () => {
  serveCatchUp()
  await renderAt('#/panda')
  await screen.findByRole('heading', { name: 'Upload unseen first 5' })
  const dialog = await openCatchUp()
  expect(dialog.textContent).toContain(`This page and every older Panda gallery, back from ${newestDate}, become seen`)
  expect(dialog.textContent).toContain("This can't be undone.")
  expect(within(dialog).queryByRole('alert')).toBeNull()
  await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(catchUpRequests()).toHaveLength(0)
  expect(displayRequests()).toHaveLength(0)

  act(() => { window.location.hash = '#/panda?unseen=false' })
  await screen.findByRole('heading', { name: 'Upload all first 5' })
  expect(screen.queryByRole('button', { name: 'Catch up…' })).toBeNull()
})

test('Catch up… is disabled without galleries and while results are stale', async () => {
  serveViews({ unseenEmpty: true })
  await renderAt('#/panda')
  await screen.findByRole('heading', { name: 'No unseen Panda galleries.' })
  expect(screen.getByRole('button', { name: 'Catch up…' }).hasAttribute('disabled')).toBe(true)

  cleanup()
  const state = serveCatchUp()
  await renderAt('#/panda')
  await screen.findByRole('heading', { name: 'Upload unseen first 5' })
  expect(screen.getByRole('button', { name: 'Catch up…' }).hasAttribute('disabled')).toBe(false)
  state.catalogFails = true
  await userEvent.click(screen.getByRole('button', { name: 'Options' }))
  await userEvent.click(screen.getByRole('menuitem', { name: 'Reload results' }))
  expect((await screen.findByRole('alert')).textContent).toContain('Results are stale.')
  expect(screen.getByRole('button', { name: 'Catch up…' }).hasAttribute('disabled')).toBe(true)
})

test('the confirmation cannot catch up once the hash leaves the unseen view', async () => {
  serveCatchUp()
  await renderAt('#/panda')
  await screen.findByRole('heading', { name: 'Upload unseen first 5' })
  await openCatchUp()
  act(() => { window.location.hash = '#/panda?unseen=false' })
  await screen.findByRole('heading', { name: 'Upload all first 5', hidden: true })
  const dialog = screen.getByRole('alertdialog')
  expect(within(dialog).getByRole('button', { name: 'Catch up' }).hasAttribute('disabled')).toBe(true)
  expect(catchUpRequests()).toHaveLength(0)
})

test('Catch up marks from the newest gallery and loads the newest unseen galleries, with or without a cursor', async () => {
  serveCatchUp()
  await renderAt('#/panda?q=s&cursor=next-token&category=manga')
  await screen.findByRole('heading', { name: 's unseen next-token 2' })
  let dialog = await openCatchUp()
  for (const key of ['{ArrowRight}', '{ArrowLeft}', 'a', 'd']) await userEvent.keyboard(key)
  expect(window.location.hash).toBe('#/panda?q=s&cursor=next-token&category=manga')
  let before = catalogRequests().length
  await userEvent.click(within(dialog).getByRole('button', { name: 'Catch up' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(catchUpRequests()).toHaveLength(1)
  expect(catchUpRequests()[0][1]?.method).toBe('POST')
  expect(JSON.parse(String(catchUpRequests()[0][1]?.body))).toEqual({ gallery_id: 2 })
  expect(window.location.hash).toBe('#/panda?q=s&category=manga')
  await screen.findByRole('heading', { name: 's unseen first 5' })
  expect(catalogRequests()[before][0]).toBe('/api/collector/catalog?q=s&cursor=&page_size=8&include_expunged=false&category=manga&unseen=true')

  dialog = await openCatchUp()
  before = catalogRequests().length
  await userEvent.click(within(dialog).getByRole('button', { name: 'Catch up' }))
  await waitFor(() => expect(catalogRequests()).toHaveLength(before + 1))
  expect(JSON.parse(String(catchUpRequests()[1][1]?.body))).toEqual({ gallery_id: 5 })
  expect(catalogRequests()[before][0]).toBe('/api/collector/catalog?q=s&cursor=&page_size=8&include_expunged=false&category=manga&unseen=true')
  expect(displayRequests()).toHaveLength(0)
})

test('a pending catch-up locks the confirmation, and a failure keeps it open with the error', async () => {
  const pending = holdCatchUps()
  await renderAt('#/panda?cursor=next-token')
  await screen.findByRole('heading', { name: 'Upload unseen next-token 2' })
  let dialog = await openCatchUp()
  await userEvent.click(within(dialog).getByRole('button', { name: 'Catch up' }))
  await waitFor(() => expect(pending).toHaveLength(1))
  expect(within(dialog).getByRole('button', { name: 'Cancel' }).hasAttribute('disabled')).toBe(true)
  expect(within(dialog).getByRole('button', { name: 'Catch up' }).hasAttribute('disabled')).toBe(true)
  await userEvent.keyboard('{Escape}')
  expect(screen.getByRole('alertdialog')).toBeTruthy()

  const before = catalogRequests().length
  await act(async () => { pending[0].resolve(Response.json({ error: 'gallery_not_found' }, { status: 404 })) })
  const alert = await within(dialog).findByRole('alert')
  expect(alert.textContent).toContain('Catch-up failed. Nothing was marked as seen.')
  expect(alert.textContent).toContain('This gallery is no longer in the Panda catalog. Reload and try again.')
  expect(window.location.hash).toBe('#/panda?cursor=next-token')
  expect(screen.getByRole('heading', { name: 'Upload unseen next-token 2', hidden: true })).toBeTruthy()
  expect(catalogRequests()).toHaveLength(before)

  // Retrying clears the alert while pending; a success then loads the newest unseen galleries.
  await userEvent.click(within(dialog).getByRole('button', { name: 'Catch up' }))
  await waitFor(() => expect(pending).toHaveLength(2))
  expect(within(dialog).queryByRole('alert')).toBeNull()
  await act(async () => { pending[1].resolve(Response.json({ error: 'collector_unreachable' }, { status: 502 })) })
  await within(dialog).findByRole('alert')
  await userEvent.click(within(dialog).getByRole('button', { name: 'Catch up' }))
  await waitFor(() => expect(pending).toHaveLength(3))
  await act(async () => { pending[2].resolve(new Response(null, { status: 204 })) })
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  await screen.findByRole('heading', { name: 'Upload unseen first 5' })
  expect(window.location.hash).toBe('#/panda')

  // A failure followed by Cancel leaves no alert for the next opening.
  dialog = await openCatchUp()
  await userEvent.click(within(dialog).getByRole('button', { name: 'Catch up' }))
  await waitFor(() => expect(pending).toHaveLength(4))
  await act(async () => { pending[3].resolve(Response.json({ error: 'collector_unreachable' }, { status: 502 })) })
  await within(dialog).findByRole('alert')
  await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  dialog = await openCatchUp()
  expect(within(dialog).queryByRole('alert')).toBeNull()
  expect(displayRequests()).toHaveLength(0)
})

test('a catch-up that succeeds after the hash changed opens the newest unseen galleries it was confirmed from', async () => {
  const pending = holdCatchUps()
  await renderAt('#/panda')
  await screen.findByRole('heading', { name: 'Upload unseen first 5' })
  const dialog = await openCatchUp()
  await userEvent.click(within(dialog).getByRole('button', { name: 'Catch up' }))
  await waitFor(() => expect(pending).toHaveLength(1))
  act(() => { window.location.hash = '#/panda?cursor=next-token' })
  await screen.findByRole('heading', { name: 'Upload unseen next-token 2', hidden: true })
  await act(async () => { pending[0].resolve(new Response(null, { status: 204 })) })
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(window.location.hash).toBe('#/panda')
  await screen.findByRole('heading', { name: 'Upload unseen first 5' })
})
