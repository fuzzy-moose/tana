// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import App from '../App'
import PandaCard from './PandaCard'

vi.mock('../collector/FavoriteSyncAction', () => ({ default: () => null }))

vi.mock('../galleries/useGalleryLayout', () => ({
  useGalleryGrid: () => ({ pageSize: 8, cardHeight: 317, viewportRef: null, cardRef: null }),
  useGalleryLayout: () => ({ pageSize: 8, cardHeight: 317, viewportRef: null, cardRef: null }),
  replaceListingPage: (search: string, page: number, pageHref: (query: string, page: number) => string) => {
    window.history.replaceState(null, '', pageHref(search, page))
    window.dispatchEvent(new HashChangeEvent('hashchange'))
  },
}))

interface Save { url: string, init: RequestInit, answer: (response: Response) => void }

const fetchMock = vi.fn<typeof fetch>()
let verdicts: Record<number, string>
let saves: Save[]
let defaultFilter: { query: string, categories: string[] }

const pages: Record<string, number[]> = { '': [5, 3], 'next-token': [2, 1] }

beforeEach(() => {
  verdicts = {}
  saves = []
  defaultFilter = { query: '', categories: [] }
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input, init) => {
    const url = new URL(String(input), 'http://localhost')
    if (url.pathname === '/api/panda/default-filter') return Response.json(defaultFilter)
    if (url.pathname === '/api/collector/catalog/displays') return new Response(null, { status: 204 })
    if (url.pathname.startsWith('/api/collector/catalog/verdicts/')) {
      return new Promise((resolve) => { saves.push({ url: String(input), init: init ?? {}, answer: resolve }) })
    }
    if (url.pathname !== '/api/collector/catalog') return Response.json({ error: 'not_found' }, { status: 404 })
    const cursor = url.searchParams.get('cursor') ?? ''
    const unseen = url.searchParams.get('unseen') === 'true'
    const verdict = url.searchParams.get('verdict')
    const ids = (pages[cursor] ?? []).filter((id) => !verdict || verdicts[id] === verdict)
    return Response.json({
      items: ids.map((id) => ({
        gallery_id: id, title: `${verdict ?? (unseen ? 'unseen' : 'all')} ${cursor || 'first'} ${id}`, thumbnail_url: '', page_count: 20, posted_at: '2026-09-12T10:00:00Z',
        metadata_source: 'collected', url: `https://panda.example/g/${id}/token/`, summary_tags: [], verdict: verdicts[id], displayed: verdicts[id] ? true : undefined,
      })),
      page_size: 8, next_cursor: cursor ? undefined : 'next-token', previous_cursor: cursor ? 'previous-token' : undefined,
    })
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

async function renderAt(hash = '#/panda') {
  window.history.replaceState(null, '', hash)
  render(<App />)
  await screen.findByRole('list', { name: 'Panda galleries' })
}

/** The list item of the card whose title ends with gallery `id`. */
function card(id: number) {
  const items = within(screen.getByRole('list', { name: 'Panda galleries' })).getAllByRole('listitem')
  return items.find((item) => within(item).queryByRole('heading')?.textContent?.endsWith(` ${id}`))!
}

function control(id: number, name: 'Not for me' | 'Interesting') {
  return within(card(id)).getByRole('button', { name })
}

function pressed(id: number) {
  return (['Not for me', 'Interesting'] as const).filter((name) => control(id, name).getAttribute('aria-pressed') === 'true')
}

function titles() {
  return within(screen.getByRole('list', { name: 'Panda galleries' })).getAllByRole('heading').map((heading) => heading.textContent)
}

function listingRequests() {
  return fetchMock.mock.calls.filter(([input]) => String(input).startsWith('/api/collector/catalog?') || String(input) === '/api/collector/catalog/displays')
}

async function answer(save: Save, response: Response) {
  await act(async () => { save.answer(response) })
}

const noContent = () => new Response(null, { status: 204 })

test('each card has the two verdict controls after its link, pressed only for the stored verdict', async () => {
  verdicts = { 3: 'not_for_me' }
  await renderAt()
  const item = card(5)
  const buttons = within(item).getAllByRole('button')
  expect(buttons.map((button) => button.getAttribute('aria-label'))).toEqual(['Not for me', 'Interesting'])
  const link = within(item).getByRole('link')
  for (const button of buttons) {
    expect(link.contains(button)).toBe(false)
    expect(link.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  }
  expect(buttons.map((button) => button.getAttribute('aria-pressed'))).toEqual(['false', 'false'])
  expect(pressed(3)).toEqual(['Not for me'])

  cleanup()
  await renderAt('#/panda?unseen=true')
  expect(titles()).toEqual(['unseen first 5', 'unseen first 3'])
  expect(pressed(3)).toEqual(['Not for me'])
})

test('gives, changes and removes a verdict after the collector stored it, without loading the listing again', async () => {
  await renderAt()
  const before = listingRequests().length
  fireEvent.click(control(5, 'Interesting'))
  expect(saves).toHaveLength(1)
  expect(saves[0].url).toBe('/api/collector/catalog/verdicts/5')
  expect(saves[0].init.method).toBe('PUT')
  expect(saves[0].init.body).toBe('{"verdict":"interesting"}')
  expect((saves[0].init.headers as Record<string, string>)['Content-Type']).toBe('application/json')
  expect(saves[0].init.keepalive).toBe(true)

  // Pending: the card keeps its state and ignores clicks; other cards stay usable.
  expect(control(5, 'Interesting').getAttribute('aria-disabled')).toBe('true')
  expect(control(5, 'Not for me').getAttribute('aria-disabled')).toBe('true')
  expect(pressed(5)).toEqual([])
  fireEvent.click(control(5, 'Interesting'))
  fireEvent.click(control(5, 'Not for me'))
  expect(saves).toHaveLength(1)
  fireEvent.click(control(3, 'Not for me'))
  expect(saves.map((save) => save.url)).toEqual(['/api/collector/catalog/verdicts/5', '/api/collector/catalog/verdicts/3'])

  await answer(saves[0], noContent())
  await answer(saves[1], noContent())
  expect(pressed(5)).toEqual(['Interesting'])
  expect(pressed(3)).toEqual(['Not for me'])
  expect(within(card(5)).queryByRole('button', { name: 'Interesting' })?.hasAttribute('aria-disabled')).toBe(false)
  const link = within(card(5)).getByRole('link')
  expect(link.classList.contains('gallery-box-subdued')).toBe(true)
  expect(link.textContent).toContain('Seen')

  fireEvent.click(control(5, 'Not for me'))
  expect(saves[2].init.method).toBe('PUT')
  expect(saves[2].init.body).toBe('{"verdict":"not_for_me"}')
  await answer(saves[2], noContent())
  expect(pressed(5)).toEqual(['Not for me'])

  fireEvent.click(control(5, 'Not for me'))
  expect(saves[3].url).toBe('/api/collector/catalog/verdicts/5')
  expect(saves[3].init.method).toBe('DELETE')
  expect(saves[3].init.keepalive).toBe(true)
  await answer(saves[3], noContent())
  expect(pressed(5)).toEqual([])
  expect(within(card(5)).getByRole('link').classList.contains('gallery-box-subdued')).toBe(true)

  expect(listingRequests()).toHaveLength(before)
  expect(titles()).toEqual(['all first 5', 'all first 3'])
})

test('in the unseen view a judged card stays in its position with its verdict pressed', async () => {
  await renderAt('#/panda?unseen=true')
  const before = listingRequests().length
  fireEvent.click(control(5, 'Not for me'))
  await answer(saves[0], noContent())
  expect(titles()).toEqual(['unseen first 5', 'unseen first 3'])
  expect(pressed(5)).toEqual(['Not for me'])
  expect(listingRequests()).toHaveLength(before)
})

test('a failed save leaves the card as it was and shows a non-modal alert until dismissed or a later save succeeds', async () => {
  verdicts = { 5: 'interesting' }
  await renderAt()
  const before = listingRequests().length
  fireEvent.click(control(5, 'Not for me'))
  await answer(saves[0], Response.json({ error: 'collector_unavailable' }, { status: 502 }))
  expect(pressed(5)).toEqual(['Interesting'])
  expect(control(5, 'Not for me').hasAttribute('aria-disabled')).toBe(false)
  const alert = screen.getByRole('alert')
  expect(alert.textContent).toMatch(/^Could not save the verdict for all first 5\. The request could not be confirmed\./)
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(screen.queryByRole('alertdialog')).toBeNull()
  expect(listingRequests()).toHaveLength(before)
  expect(titles()).toEqual(['all first 5', 'all first 3'])

  fireEvent.click(within(alert).getByRole('button', { name: 'Dismiss' }))
  expect(screen.queryByRole('alert')).toBeNull()

  fireEvent.click(control(5, 'Not for me'))
  await answer(saves[1], Response.json({ error: 'collector_unavailable' }, { status: 502 }))
  expect(screen.getByRole('alert')).toBeTruthy()
  fireEvent.click(control(5, 'Not for me'))
  await answer(saves[2], noContent())
  expect(screen.queryByRole('alert')).toBeNull()
  expect(pressed(5)).toEqual(['Not for me'])
})

test('paging keys work with the focus on a verdict control, and the failure alert stays across pages', async () => {
  await renderAt()
  const button = control(5, 'Interesting')
  button.focus()
  fireEvent.click(button)
  await answer(saves[0], Response.json({ error: 'collector_unavailable' }, { status: 502 }))
  expect(screen.getByRole('alert')).toBeTruthy()
  fireEvent.keyDown(document.activeElement!, { key: 'd' })
  await screen.findByRole('heading', { name: 'all next-token 2' })
  await waitFor(() => expect(fetchMock.mock.calls.filter(([input]) => String(input) === '/api/collector/catalog/displays')).toHaveLength(1))
  const record = fetchMock.mock.calls.find(([input]) => String(input) === '/api/collector/catalog/displays')!
  expect(JSON.parse(String(record[1]?.body))).toEqual({ gallery_ids: [5, 3] })
  expect(screen.getByRole('alert').textContent).toContain('Could not save the verdict for all first 5.')
})

test('a save that settles after a page change still shows its failure', async () => {
  await renderAt()
  const button = control(5, 'Interesting')
  button.focus()
  fireEvent.click(button)
  fireEvent.keyDown(document.activeElement!, { key: 'd' })
  await screen.findByRole('heading', { name: 'all next-token 2' })
  await answer(saves[0], Response.json({ error: 'collector_unavailable' }, { status: 502 }))
  expect(screen.getByRole('alert').textContent).toContain('Could not save the verdict for all first 5.')
  expect(titles()).toEqual(['all next-token 2', 'all next-token 1'])
  expect(pressed(2)).toEqual([])
})

test('a card without onVerdict renders no controls', () => {
  render(<Theme><PandaCard gallery={{ gallery_id: 1, title: 'Plain', thumbnail_url: '', page_count: 1, posted_at: null, metadata_source: 'collected', url: 'https://panda.example/g/1/token/', summary_tags: [] }} /></Theme>)
  expect(screen.queryByRole('button')).toBeNull()
})

function catalogURLs() {
  return fetchMock.mock.calls.map(([input]) => String(input)).filter((input) => input.startsWith('/api/collector/catalog?'))
}

function sentWrites() {
  return fetchMock.mock.calls.filter(([input, init]) => String(input) === '/api/collector/catalog/displays' || (String(input) === '/api/panda/default-filter' && init?.method === 'PUT'))
}

/** Renders the app at `hash` and waits until the catalog answered, whether or not it listed galleries. */
async function renderView(hash: string) {
  window.history.replaceState(null, '', hash)
  render(<App />)
  await waitFor(() => expect(screen.queryByRole('status')).toBeNull())
  await waitFor(() => expect(catalogURLs().length).toBeGreaterThan(0))
}

const views = ['All', 'Unseen', 'Interesting', 'Not for me']

function viewChecked() {
  return views.filter((name) => screen.getByRole('radio', { name }).getAttribute('aria-checked') === 'true')
}

/** The text of the empty state that `heading` titles. */
function emptyState(heading: HTMLElement) {
  return heading.parentElement!.textContent
}

async function chooseView(name: string, hash: string) {
  await userEvent.click(screen.getByRole('radio', { name }))
  await waitFor(() => expect(window.location.hash).toBe(hash))
}

test('the view control offers all, unseen and the two verdict views, with all selected by default', async () => {
  await renderAt()
  expect(screen.getAllByRole('radio')).toEqual(views.map((name) => screen.getByRole('radio', { name })))
  expect(viewChecked()).toEqual(['All'])
  expect(catalogURLs().every((url) => !url.includes('verdict'))).toBe(true)
})

test('choosing a verdict view drops the cursor, puts the verdict last and records nothing', async () => {
  await renderView('#/panda?q=s&cursor=next-token')
  await chooseView('Interesting', '#/panda?q=s&verdict=interesting')
  await waitFor(() => expect(catalogURLs().at(-1)).toMatch(/[?&]q=s&.*&verdict=interesting$/))
  expect(catalogURLs().at(-1)).not.toContain('cursor=next-token')
  expect(viewChecked()).toEqual(['Interesting'])
  await chooseView('Not for me', '#/panda?q=s&verdict=not_for_me')
  await waitFor(() => expect(catalogURLs().at(-1)).toMatch(/&verdict=not_for_me$/))
  expect(sentWrites()).toEqual([])
})

test('the view control switches between the verdict views, unseen and all', async () => {
  await renderView('#/panda?verdict=interesting')
  await chooseView('Unseen', '#/panda?unseen=true')
  await waitFor(() => expect(catalogURLs().at(-1)).toMatch(/&unseen=true$/))
  await chooseView('Not for me', '#/panda?verdict=not_for_me')
  await waitFor(() => expect(catalogURLs().at(-1)).toMatch(/&verdict=not_for_me$/))
  expect(catalogURLs().at(-1)).not.toContain('unseen')
  await chooseView('All', '#/panda')
  await waitFor(() => expect(catalogURLs().at(-1)).toBe('/api/collector/catalog?q=&cursor=&page_size=8&include_expunged=false'))
})

test('a link opens its verdict view with its search and categories, prefers the verdict over unseen, and ignores an unknown verdict', async () => {
  await renderView('#/panda?q=s&category=manga&verdict=not_for_me')
  expect(viewChecked()).toEqual(['Not for me'])
  expect(catalogURLs().at(-1)).toBe('/api/collector/catalog?q=s&cursor=&page_size=8&include_expunged=false&category=manga&verdict=not_for_me')

  cleanup()
  fetchMock.mockClear()
  await renderView('#/panda?verdict=interesting&unseen=true')
  expect(viewChecked()).toEqual(['Interesting'])
  expect(catalogURLs().at(-1)).toMatch(/&verdict=interesting$/)
  expect(catalogURLs().at(-1)).not.toContain('unseen')

  cleanup()
  fetchMock.mockClear()
  await renderView('#/panda?verdict=maybe')
  expect(viewChecked()).toEqual(['All'])
  expect(catalogURLs().at(-1)).not.toContain('verdict')
})

test('a search and clearing the filters keep the verdict view', async () => {
  verdicts = { 5: 'interesting' }
  await renderView('#/panda?verdict=interesting')
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'p:fate' } })
  fireEvent.submit(screen.getByRole('search'))
  await waitFor(() => expect(window.location.hash).toBe('#/panda?q=p%3Afate&verdict=interesting'))
  await waitFor(() => expect(catalogURLs().at(-1)).toMatch(/q=p%3Afate&.*&verdict=interesting$/))
  const clear = await screen.findAllByRole('link', { name: 'Clear all filters' })
  expect(clear.map((link) => link.getAttribute('href'))).toEqual(['#/panda?bypass_default=true&verdict=interesting'])
})

test('a verdict view pages like the view of all galleries, without a catch-up', async () => {
  verdicts = { 5: 'interesting', 3: 'interesting', 2: 'interesting' }
  await renderAt('#/panda?verdict=interesting')
  expect(screen.queryByRole('button', { name: 'Catch up…' })).toBeNull()
  fireEvent.click(screen.getByRole('link', { name: 'Next' }))
  await waitFor(() => expect(window.location.hash).toBe('#/panda?cursor=next-token&verdict=interesting'))
  await screen.findByRole('heading', { name: 'interesting next-token 2' })
  const record = fetchMock.mock.calls.find(([input]) => String(input) === '/api/collector/catalog/displays')!
  expect(JSON.parse(String(record[1]?.body))).toEqual({ gallery_ids: [5, 3] })
  expect(catalogURLs().at(-1)).toMatch(/cursor=next-token&.*&verdict=interesting$/)
  expect(screen.getByRole('link', { name: 'Previous' }).getAttribute('href')).toBe('#/panda?cursor=previous-token&verdict=interesting')
  expect(screen.queryByRole('button', { name: 'Catch up…' })).toBeNull()
})

test('in a verdict view a card whose verdict is removed or changed stays in its position', async () => {
  verdicts = { 5: 'interesting', 3: 'interesting' }
  await renderAt('#/panda?verdict=interesting')
  const before = catalogURLs().length
  expect(pressed(5)).toEqual(['Interesting'])
  expect(pressed(3)).toEqual(['Interesting'])

  fireEvent.click(control(5, 'Interesting'))
  expect(saves[0].init.method).toBe('DELETE')
  await answer(saves[0], noContent())
  fireEvent.click(control(3, 'Not for me'))
  expect(saves[1].init.body).toBe('{"verdict":"not_for_me"}')
  await answer(saves[1], noContent())

  expect(titles()).toEqual(['interesting first 5', 'interesting first 3'])
  expect(pressed(5)).toEqual([])
  expect(pressed(3)).toEqual(['Not for me'])
  expect(catalogURLs()).toHaveLength(before)
})

test('cards in a verdict view have the normal look', async () => {
  verdicts = { 5: 'interesting' }
  await renderAt('#/panda?verdict=interesting')
  let link = within(card(5)).getByRole('link')
  expect(link.classList.contains('gallery-box-subdued')).toBe(false)
  expect(link.textContent).not.toContain('Seen')

  cleanup()
  await renderAt()
  link = within(card(5)).getByRole('link')
  expect(link.classList.contains('gallery-box-subdued')).toBe(true)
  expect(link.textContent).toContain('Seen')
})

test('an empty verdict view without filters invites a verdict and offers all galleries', async () => {
  for (const [verdict, label] of [['interesting', 'Interesting'], ['not_for_me', 'Not for me']]) {
    await renderView(`#/panda?verdict=${verdict}`)
    const heading = await screen.findByRole('heading', { level: 2 })
    expect(heading.textContent).toBe('No Panda gallery has this verdict.')
    expect(screen.getByText(`Mark a gallery as ${label} on its card, and it appears here.`)).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Show all galleries' }).getAttribute('href')).toBe('#/panda')
    expect(screen.queryByText('No Panda galleries yet.')).toBeNull()
    expect(emptyState(heading)).not.toMatch(/unseen/i)
    cleanup()
  }
})

test('an empty verdict view with a search or an applied default filter offers to clear the filters', async () => {
  await renderView('#/panda?q=s&verdict=interesting')
  const heading = await screen.findByRole('heading', { level: 2 })
  expect(heading.textContent).toBe('No Panda gallery with this verdict matches.')
  expect(screen.getByText('Try different titles, tags, or categories, or clear all filters.')).toBeTruthy()
  const clear = within(heading.parentElement!).getByRole('link', { name: 'Clear all filters' })
  expect(clear.getAttribute('href')).toBe('#/panda?bypass_default=true&verdict=interesting')
  expect(emptyState(heading)).not.toMatch(/unseen/i)

  cleanup()
  defaultFilter = { query: '-l:chinese$', categories: [] }
  await renderView('#/panda?verdict=not_for_me')
  await screen.findByRole('heading', { name: 'No Panda gallery with this verdict matches.' })
  expect(screen.queryByText('No Panda galleries yet.')).toBeNull()
})

test('an empty verdict view at a cursor offers the newest galleries of that view', async () => {
  await renderView('#/panda?cursor=next-token&verdict=interesting')
  await screen.findByRole('heading', { name: 'No more matching Panda galleries.' })
  expect(screen.getByRole('link', { name: 'Newest galleries' }).getAttribute('href')).toBe('#/panda?verdict=interesting')
})
