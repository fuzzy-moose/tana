// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import App from './App'

vi.mock('./galleries/useGalleryLayout', () => ({
  useGalleryLayout: () => ({ pageSize: 24, cardHeight: 317, viewportRef: null, cardRef: null }),
}))

beforeEach(() => { window.history.replaceState(null, '', '/') })
afterEach(() => { cleanup(); vi.unstubAllGlobals() })

test('defaults to galleries, searches and paginates, then opens gallery details and thumbnail links', async () => {
  const fetchMock = vi.fn<typeof fetch>(async (input) => {
    const url = new URL(String(input), 'http://localhost')
    if (url.pathname === '/api/galleries/g') return Response.json({ id: 'g', title: 'Manga', page_count: 3 })
    const page = Number(url.searchParams.get('page'))
    return Response.json({ items: [{ id: 'g', title: page === 2 ? 'Manga Two' : 'Manga', page_count: 3 }], total: 25, page, page_size: 24 })
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(<App />)
  expect(await screen.findByRole('heading', { name: 'Galleries' })).toBeTruthy()
  expect(await screen.findByRole('heading', { name: 'Manga' })).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Scan all' })).toBeNull()
  await user.type(screen.getByLabelText('Search titles and tags'), 'Manga')
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/galleries?q=Manga&page=1&page_size=24', expect.anything()))
  await user.click(await screen.findByRole('link', { name: 'Next' }))
  await screen.findByRole('heading', { name: 'Manga Two' })
  expect(window.location.hash).toBe('#/galleries?q=Manga&page=2')
  expect(screen.getByRole('link', { name: 'Page 1' }).getAttribute('href')).toBe('#/galleries?q=Manga')
  await user.click(within(screen.getByRole('list', { name: 'Galleries' })).getByRole('link'))
  await screen.findByRole('link', { name: 'Read gallery' })
  expect(screen.getByRole('link', { name: 'Read from page 3' }).getAttribute('href')).toBe('#/galleries/g/read?page=3')
  expect(fetchMock.mock.calls.some(([url]) => String(url).startsWith('/api/scans'))).toBe(false)
})

test('keeps empty galleries visible and disables reading in their detail view', async () => {
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input) => Response.json(String(input).includes('?')
    ? { items: [{ id: 'empty', title: 'Empty gallery', page_count: 0 }], total: 1, page: 1, page_size: 24 }
    : { id: 'empty', title: 'Empty gallery', page_count: 0 })))
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('No pages')
  await user.click(within(screen.getByRole('list', { name: 'Galleries' })).getByRole('link'))
  await screen.findByText('This gallery has no pages to read.')
  expect(screen.queryByRole('link', { name: 'Read gallery' })).toBeNull()
})

test('invalid submitted syntax shows a generic error and preserves the query for correction', async () => {
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input) => {
    const url = new URL(String(input), 'http://localhost')
    if (url.pathname === '/api/gallery-search/completions') return Response.json({ start: 0, end: 0, items: [] })
    if (url.searchParams.get('q')) return Response.json({ error: 'invalid_query' }, { status: 400 })
    return Response.json({ items: [], total: 0, page: 1, page_size: 24 })
  }))
  const user = userEvent.setup()
  render(<App />)
  await user.type(screen.getByRole('combobox'), 'title:blue$')
  expect(screen.queryByRole('alert')).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Search' }))
  expect((await screen.findByRole('alert')).textContent).toContain('Invalid search query.')
  expect((screen.getByRole('combobox') as HTMLInputElement).value).toBe('title:blue$')
})
