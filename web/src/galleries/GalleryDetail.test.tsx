// @vitest-environment jsdom
import { act, cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'
import GalleryDetail from './GalleryDetail'

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

const gallery = {
  id: 1, title: 'Manga', page_count: 1,
  tags: [
    { namespace: 'artist', value: 'alpha', term: 'artist:alpha$' },
    { namespace: 'artist', value: 'zeta', term: 'artist:zeta$' },
    { namespace: 'custom', value: 'alpha', term: 'custom:alpha$' },
    { namespace: 'other', value: 'all ages', term: 'other:"all ages$"' },
  ],
}

test('displays namespace groups between metadata and pages with exact tag search links', async () => {
  const fetchMock = vi.fn(async () => Response.json(gallery))
  vi.stubGlobal('fetch', fetchMock)
  render(<GalleryDetail id={1} />)
  const tags = await screen.findByRole('region', { name: 'Tags' })
  expect(within(tags).getAllByRole('term').map((term) => term.textContent)).toEqual(['artist', 'custom', 'Other'])
  expect(within(tags).getAllByRole('listitem').map((item) => item.textContent)).toEqual(['alpha', 'zeta', 'alpha', 'all ages'])
  expect(within(tags).getAllByRole('button', { pressed: false }).map((tag) => tag.getAttribute('href'))).toEqual([
    '#/galleries?q=artist%3Aalpha%24', '#/galleries?q=artist%3Azeta%24',
    '#/galleries?q=custom%3Aalpha%24', '#/galleries?q=other%3A%22all+ages%24%22',
  ])
  expect(screen.queryByRole('link', { name: /Search selected tags/ })).toBeNull()
  expect(screen.queryByRole('link', { name: 'Open on Panda' })).toBeNull()
  expect(fetchMock).toHaveBeenCalledTimes(1)
  expect(screen.getByRole('link', { name: 'Read gallery' }).compareDocumentPosition(tags) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(tags.compareDocumentPosition(screen.getByRole('heading', { name: 'Pages' })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

test('toggles tags independently across namespaces and searches all selected tags', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(gallery)))
  window.history.replaceState(null, '', '#/galleries/1')
  const user = userEvent.setup()
  render(<GalleryDetail id={1} />)
  const artist = within(await screen.findByRole('list', { name: 'artist tags' })).getByRole('button', { name: 'alpha' })
  const custom = within(screen.getByRole('list', { name: 'custom tags' })).getByRole('button', { name: 'alpha' })
  await user.click(artist)
  await user.click(custom)
  await user.click(screen.getByRole('button', { name: 'all ages' }))
  expect(screen.getAllByRole('button', { pressed: true })).toHaveLength(3)
  expect(screen.getByRole('link', { name: 'Search selected tags (3)' }).getAttribute('href')).toBe(
    '#/galleries?q=artist%3Aalpha%24+custom%3Aalpha%24+other%3A%22all+ages%24%22',
  )
  expect(window.location.hash).toBe('#/galleries/1')
  await user.click(artist)
  expect(artist.getAttribute('aria-pressed')).toBe('false')
  expect(custom.getAttribute('aria-pressed')).toBe('true')
  expect(screen.getByRole('link', { name: 'Search selected tags (2)' }).getAttribute('href')).toBe(
    '#/galleries?q=custom%3Aalpha%24+other%3A%22all+ages%24%22',
  )
  artist.focus()
  await user.keyboard(' ')
  expect(artist.getAttribute('aria-pressed')).toBe('true')
  await user.click(screen.getByRole('button', { name: 'Clear' }))
  expect(screen.queryByRole('button', { pressed: true })).toBeNull()
  expect(screen.queryByRole('link', { name: /Search selected tags/ })).toBeNull()
})

test.each([200, 404, 503])('looks up an optional Panda link without delaying gallery display (status %i)', async (status) => {
  let finishLookup!: (response: Response) => void
  const lookup = new Promise<Response>((resolve) => { finishLookup = resolve })
  const fetchMock = vi.fn<typeof fetch>(async (input) => String(input) === '/api/galleries/1'
    ? Response.json({ ...gallery, panda_candidate_id: 42 })
    : lookup)
  vi.stubGlobal('fetch', fetchMock)
  render(<GalleryDetail id={1} />)
  await screen.findByRole('heading', { name: 'Manga' })
  expect(screen.getByRole('link', { name: 'Read gallery' })).toBeTruthy()
  expect(screen.queryByRole('link', { name: 'Open on Panda' })).toBeNull()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/catalog/lookup', expect.objectContaining({ method: 'POST', body: '{"gid":42}' }))
  await act(async () => finishLookup(Response.json(status === 200
    ? { gid: 42, token: '0123456789', url: 'https://panda.test/g/42/0123456789/' }
    : { error: status === 404 ? 'gallery_not_found' : 'collector_unavailable' }, { status })))
  if (status === 200) {
    const link = screen.getByRole('link', { name: 'Open on Panda' })
    expect(link.getAttribute('href')).toBe('https://panda.test/g/42/0123456789/')
    expect(link.getAttribute('target')).toBe('_blank')
  } else {
    expect(screen.queryByRole('link', { name: 'Open on Panda' })).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
  }
})

test('hides the tag section when the gallery has no tags', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ id: 1, title: 'Manga', page_count: 0, tags: [] })))
  render(<GalleryDetail id={1} />)
  await screen.findByRole('heading', { name: 'Manga' })
  expect(screen.queryByRole('region', { name: 'Tags' })).toBeNull()
})
