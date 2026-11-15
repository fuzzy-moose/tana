// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import App from '../App'
import type { PandaCatalogScoreFilter } from './api'

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
let scoreFilter: PandaCatalogScoreFilter | undefined
let empty: boolean
let defaultFilter: { query: string, categories: string[] }
const verdictSaves: ((response: Response) => void)[] = []

beforeEach(() => {
  scoreFilter = undefined
  empty = false
  defaultFilter = { query: '', categories: [] }
  verdictSaves.length = 0
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input) => {
    const url = new URL(String(input), 'http://localhost')
    if (url.pathname === '/api/panda/default-filter') return Response.json(defaultFilter)
    if (url.pathname === '/api/collector/catalog/displays') return new Response(null, { status: 204 })
    if (url.pathname === '/api/collector/catalog/catch-up') return new Response(null, { status: 204 })
    if (url.pathname.startsWith('/api/collector/catalog/verdicts/')) return new Promise((resolve) => { verdictSaves.push(resolve) })
    if (url.pathname !== '/api/collector/catalog') return Response.json({ error: 'not_found' }, { status: 404 })
    const cursor = url.searchParams.get('cursor') ?? ''
    const ids = empty ? [] : cursor ? [2, 1] : [5, 3]
    return Response.json({
      items: ids.map((id) => ({
        gallery_id: id, title: `${cursor || 'first'} ${id}`, thumbnail_url: '', page_count: 20, posted_at: '2026-09-12T10:00:00Z',
        metadata_source: 'collected', url: `https://panda.example/g/${id}/token/`, summary_tags: [],
      })),
      page_size: 8, next_cursor: cursor || empty ? undefined : 'next-token', previous_cursor: cursor ? 'previous-token' : undefined,
      score_filter: scoreFilter,
    })
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

function catalogURLs() {
  return fetchMock.mock.calls.map(([input]) => String(input)).filter((input) => input.startsWith('/api/collector/catalog?'))
}

function displayBodies() {
  return fetchMock.mock.calls.filter(([input]) => String(input) === '/api/collector/catalog/displays').map(([, init]) => String(init?.body))
}

/** Renders the app at `hash` and waits until the catalog answered, whether or not it listed galleries. */
async function renderAt(hash: string) {
  window.history.replaceState(null, '', hash)
  render(<App />)
  await waitFor(() => expect(catalogURLs().length).toBeGreaterThan(0))
  await waitFor(() => expect(screen.queryByRole('status')).toBeNull())
  await waitFor(() => expect(screen.queryByLabelText('Loading results')).toBeNull())
}

function scoreControl() {
  return screen.queryByRole('radiogroup', { name: 'Score filter' })
}

const states = ['Filter on', 'Filter off', 'Hidden only']

function scoreChecked() {
  return states.filter((name) => within(scoreControl()!).getByRole('radio', { name }).getAttribute('aria-checked') === 'true')
}

async function waitForHash(hash: string) {
  await waitFor(() => expect(window.location.hash).toBe(hash))
}

test('the link holds the state, the request carries it outside verdict views, and the control shows it', async () => {
  await renderAt('#/panda?score_filter=hidden&unseen=false')
  expect(new URL(catalogURLs().at(-1)!, 'http://localhost').searchParams.get('score_filter')).toBe('hidden')
  expect(scoreChecked()).toEqual(['Hidden only'])

  cleanup()
  fetchMock.mockClear()
  await renderAt('#/panda?unseen=false')
  expect(catalogURLs().at(-1)).not.toContain('score_filter')
  expect(scoreControl()).toBeNull()

  cleanup()
  fetchMock.mockClear()
  scoreFilter = {}
  await renderAt('#/panda?score_filter=all&verdict=interesting')
  expect(catalogURLs().at(-1)).toMatch(/&verdict=interesting$/)
  expect(catalogURLs().at(-1)).not.toContain('score_filter')
  expect(scoreControl()).toBeNull()
})

test('the control shows with "Filter on" in the views All and Unseen when the filter is in force', async () => {
  scoreFilter = { threshold_tenths: -5 }
  await renderAt('#/panda?unseen=false')
  expect(scoreChecked()).toEqual(['Filter on'])
  expect(within(scoreControl()!).getAllByRole('radio')).toEqual(states.map((name) => within(scoreControl()!).getByRole('radio', { name })))
  cleanup()
  await renderAt('#/panda')
  expect(scoreChecked()).toEqual(['Filter on'])
})

test('choosing a state drops the cursor and keeps the search and view', async () => {
  scoreFilter = {}
  await renderAt('#/panda?q=x&cursor=c')
  await userEvent.click(within(scoreControl()!).getByRole('radio', { name: 'Filter off' }))
  await waitForHash('#/panda?q=x&score_filter=all')
  await waitFor(() => expect(catalogURLs().at(-1)).toMatch(/score_filter=all&unseen=true$/))
})

test('the "Hidden only" state survives search, filters, paging and the view switch, but not a verdict view', async () => {
  defaultFilter = { query: '-l:chinese$', categories: [] }
  await renderAt('#/panda?score_filter=hidden&unseen=false')

  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'p:fate' } })
  fireEvent.submit(screen.getByRole('search'))
  await waitForHash('#/panda?q=p%3Afate&score_filter=hidden&unseen=false')

  window.location.hash = '#/panda?score_filter=hidden&unseen=false'
  await waitFor(() => expect(screen.getByRole('link', { name: 'Bypass default' })).toBeTruthy())
  expect(screen.getByRole('link', { name: 'Bypass default' }).getAttribute('href')).toBe('#/panda?bypass_default=true&score_filter=hidden&unseen=false')
  window.location.hash = '#/panda?q=s&score_filter=hidden&unseen=false'
  const clear = await screen.findByRole('link', { name: 'Clear all filters' })
  expect(clear.getAttribute('href')).toBe('#/panda?bypass_default=true&score_filter=hidden&unseen=false')

  await userEvent.click(screen.getByRole('button', { name: /Categories|Category/ }))
  const option = (await screen.findAllByRole('menuitemcheckbox'))[0]
  await userEvent.click(option)
  await waitFor(() => expect(window.location.hash).toMatch(/^#\/panda\?q=s&category=[^&]+&score_filter=hidden&unseen=false$/))
  await userEvent.keyboard('{Escape}')

  window.location.hash = '#/panda?score_filter=hidden&unseen=false'
  await renderedNext()
  await userEvent.click(screen.getByRole('button', { name: /^Options/ }))
  await userEvent.click(await screen.findByRole('menuitemcheckbox', { name: 'Include expunged' }))
  await waitForHash('#/panda?include_expunged=true&score_filter=hidden&unseen=false')

  await userEvent.click(screen.getByRole('radio', { name: 'Unseen' }))
  await waitForHash('#/panda?include_expunged=true&score_filter=hidden')
  await userEvent.click(screen.getByRole('radio', { name: 'All' }))
  await waitForHash('#/panda?include_expunged=true&score_filter=hidden&unseen=false')
  await renderedNext()
  fireEvent.click(screen.getByRole('link', { name: 'Next' }))
  await waitForHash('#/panda?cursor=next-token&include_expunged=true&score_filter=hidden&unseen=false')
  await screen.findByRole('heading', { name: 'next-token 2' })
  await userEvent.click(screen.getByRole('radio', { name: 'Not for me' }))
  await waitForHash('#/panda?include_expunged=true&verdict=not_for_me')
})

async function renderedNext() {
  await waitFor(() => expect(screen.getByRole('link', { name: 'Next' })).toBeTruthy())
}

test('Next records the displays as no evidence only in the "Hidden only" state', async () => {
  for (const [hash, body] of [
    ['#/panda?score_filter=hidden&unseen=false', '{"gallery_ids":[5,3],"score_filter":"hidden"}'],
    ['#/panda?score_filter=all&unseen=false', '{"gallery_ids":[5,3]}'],
    ['#/panda?unseen=false', '{"gallery_ids":[5,3]}'],
  ]) {
    scoreFilter = {}
    await renderAt(hash)
    await renderedNext()
    fireEvent.click(screen.getByRole('link', { name: 'Next' }))
    await waitFor(() => expect(displayBodies()).toEqual([body]))
    cleanup()
    fetchMock.mockClear()
  }
})

test('a "Not for me" verdict in the state "Filter on" leaves the card in its position and loads nothing', async () => {
  scoreFilter = { threshold_tenths: 0 }
  await renderAt('#/panda?unseen=false')
  const before = catalogURLs().length
  const list = screen.getByRole('list', { name: 'Panda galleries' })
  const first = within(list).getAllByRole('listitem')[0]
  fireEvent.click(within(first).getByRole('button', { name: 'Not for me' }))
  await act(async () => { verdictSaves[0](new Response(null, { status: 204 })) })
  const items = within(list).getAllByRole('listitem')
  expect(items.map((item) => within(item).getByRole('heading').textContent)).toEqual(['first 5', 'first 3'])
  expect(within(items[0]).getByRole('button', { name: 'Not for me' }).getAttribute('aria-pressed')).toBe('true')
  expect(catalogURLs()).toHaveLength(before)
})

function emptyState() {
  return screen.getByRole('heading', { level: 2 }).parentElement!
}

test('an empty unseen page without a cursor says that the score filter hides the rest and offers "Show hidden only"', async () => {
  empty = true
  scoreFilter = { hidden_unseen: true }
  await renderAt('#/panda')
  expect(screen.getByRole('heading', { level: 2 }).textContent).toBe('No unseen Panda galleries.')
  expect(screen.getByText('The score filter hides the unseen galleries that remain.')).toBeTruthy()
  expect(screen.queryByText(/You have seen every Panda gallery/)).toBeNull()
  const links = within(emptyState()).getAllByRole('link').map((link) => [link.textContent, link.getAttribute('href')])
  expect(links).toEqual([['Show hidden only', '#/panda?score_filter=hidden'], ['Show all galleries', '#/panda?unseen=false']])

  cleanup()
  scoreFilter = {}
  await renderAt('#/panda')
  expect(screen.getByText(/You have seen every Panda gallery/)).toBeTruthy()
  expect(screen.queryByRole('link', { name: 'Show hidden only' })).toBeNull()

  cleanup()
  scoreFilter = { hidden_unseen: true }
  await renderAt('#/panda?q=x')
  expect(screen.getByRole('heading', { level: 2 }).textContent).toBe('No unseen Panda galleries match.')
  expect(screen.getByText('The score filter hides the unseen galleries that remain.')).toBeTruthy()
  expect(screen.queryByText('Try different titles, tags, or categories, or show all galleries.')).toBeNull()
  expect(screen.getByRole('link', { name: 'Show hidden only' }).getAttribute('href')).toBe('#/panda?q=x&score_filter=hidden')
})

test('an empty unseen page at a cursor keeps its text, whatever the score filter says', async () => {
  empty = true
  scoreFilter = { hidden_unseen: true }
  await renderAt('#/panda?cursor=c')
  expect(screen.getByText('Galleries that arrived during this visit wait with the newest unseen galleries.')).toBeTruthy()
  expect(screen.getByRole('link', { name: 'Newest unseen galleries' })).toBeTruthy()
  expect(screen.queryByRole('link', { name: 'Show hidden only' })).toBeNull()
  expect(screen.queryByText(/score filter/)).toBeNull()
})

test('an empty "Hidden only" listing says that the score filter hides nothing here and offers "Filter on"', async () => {
  empty = true
  for (const [hash, filterOn] of [['#/panda?score_filter=hidden&unseen=false', '#/panda?unseen=false'], ['#/panda?score_filter=hidden', '#/panda']]) {
    await renderAt(hash)
    expect(screen.getByRole('heading', { level: 2 }).textContent).toBe('No hidden Panda galleries.')
    expect(screen.getByText('The score filter hides no gallery that matches this view, search and filters.')).toBeTruthy()
    expect(within(emptyState()).getByRole('link', { name: 'Filter on' }).getAttribute('href')).toBe(filterOn)
    cleanup()
  }
})

test('the catch-up confirmation names the galleries of the score filter that it also covers', async () => {
  for (const [hash, filter, phrase] of [
    ['#/panda', {}, 'galleries that the score filter hides'],
    ['#/panda?score_filter=hidden', {}, 'galleries that the score filter does not hide'],
    ['#/panda', undefined, ''],
  ] as const) {
    scoreFilter = filter
    await renderAt(hash)
    await userEvent.click(screen.getByRole('button', { name: 'Catch up…' }))
    const text = (await screen.findByRole('alertdialog')).textContent
    if (phrase) expect(text).toContain(phrase)
    else expect(text).not.toMatch(/score filter/)
    if (phrase.endsWith('hides')) expect(text).not.toContain('does not hide')
    cleanup()
  }
})
