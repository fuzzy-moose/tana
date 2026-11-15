// @vitest-environment jsdom
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
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
  expect(fetchMock).not.toHaveBeenCalledWith('/api/collector/catalog/lookup', expect.anything())
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
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/collector/catalog/lookup', expect.objectContaining({ method: 'POST', body: '{"gid":42}' })))
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

const tagsPath = '/api/collector/catalog/score/tags'
const pinsPath = '/api/collector/catalog/score/pins'
const zetaPin = { id: 7, kind: 'tag', label: 'artist:zeta', weight: -1.2345 }
const weights = {
  active: true,
  tags: [
    { tag: 'artist:alpha', known: true, weight: 0.4234 },
    { tag: 'artist:zeta', known: true, weight: -1.2345, pin: zetaPin },
    { tag: 'custom:alpha', known: false, weight: null },
    { tag: 'other:all ages', known: true, weight: null },
  ],
}

/** Answers the gallery, the tag weights by `answer`, and pin changes by `change`. */
function stubWeights(answer: () => Response | Promise<Response>, change: (init?: RequestInit) => Response = () => new Response(null, { status: 204 })) {
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input)
    if (path === '/api/galleries/1') return Response.json(gallery)
    if (path === tagsPath) return answer()
    return change(init)
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

const tagRequests = (fetchMock: ReturnType<typeof stubWeights>) => fetchMock.mock.calls.filter(([input]) => String(input) === tagsPath)

test('requests the weights of all tags without delaying the page and shows them beside the tags', async () => {
  let finish!: (response: Response) => void
  const fetchMock = stubWeights(() => new Promise<Response>((resolve) => { finish = resolve }))
  render(<GalleryDetail id={1} />)
  await screen.findByRole('heading', { name: 'Manga' })
  const tags = screen.getByRole('region', { name: 'Tags' })
  expect(within(tags).getAllByRole('listitem').map((item) => item.textContent)).toEqual(['alpha', 'zeta', 'alpha', 'all ages'])
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(tagsPath, expect.objectContaining({
    method: 'POST', body: '{"tags":["artist:alpha","artist:zeta","custom:alpha","other:all ages"]}',
  })))
  expect(tagRequests(fetchMock)).toHaveLength(1)
  await act(async () => finish(Response.json(weights)))
  expect(within(tags).getByRole('button', { name: 'Weight of artist:alpha: +0.42' }).textContent).toBe('+0.42')
  const zeta = within(tags).getByRole('button', { name: 'Pinned weight of artist:zeta: -1.23' })
  expect(zeta.querySelector('svg')).toBeTruthy()
  expect(within(tags).getByRole('button', { name: 'Weight of other:all ages: –' })).toBeTruthy()
  expect(within(tags).queryByRole('button', { name: /custom:alpha/ })).toBeNull()
  const unknown = within(screen.getByRole('list', { name: 'custom tags' })).getByRole('listitem')
  expect(unknown.textContent).toContain('not in the Panda catalog')
  expect(screen.getAllByRole('button', { name: /eight of/ })).toHaveLength(3)
  expect(screen.queryByText(/No score model is active/)).toBeNull()
})

test('sends no tag weights request for a gallery without tags', async () => {
  const fetchMock = vi.fn(async () => Response.json({ id: 1, title: 'Manga', page_count: 0, tags: [] }))
  vi.stubGlobal('fetch', fetchMock)
  render(<GalleryDetail id={1} />)
  await screen.findByRole('heading', { name: 'Manga' })
  expect(fetchMock).toHaveBeenCalledTimes(1)
})

test('says that pins have no effect without an active model', async () => {
  stubWeights(() => Response.json({ ...weights, active: false }))
  render(<GalleryDetail id={1} />)
  const tags = await screen.findByRole('region', { name: 'Tags' })
  expect(await within(tags).findByText('No score model is active: pinned weights have no effect.')).toBeTruthy()
})

test('starts the pin editor at the pin, the rounded weight or 0', async () => {
  stubWeights(() => Response.json(weights))
  const user = userEvent.setup()
  render(<GalleryDetail id={1} />)
  await user.click(await screen.findByRole('button', { name: /^Weight of artist:alpha/ }))
  let editor = screen.getByRole('dialog')
  expect((within(editor).getByRole('spinbutton', { name: 'Weight' }) as HTMLInputElement).value).toBe('0.42')
  expect(within(editor).queryByRole('button', { name: 'Remove pin' })).toBeNull()
  await user.keyboard('{Escape}')
  await user.click(screen.getByRole('button', { name: /^Pinned weight of artist:zeta/ }))
  editor = screen.getByRole('dialog')
  expect((within(editor).getByRole('spinbutton', { name: 'Weight' }) as HTMLInputElement).value).toBe('-1.2345')
  expect(within(editor).getByRole('button', { name: 'Remove pin' })).toBeTruthy()
  await user.keyboard('{Escape}')
  await user.click(screen.getByRole('button', { name: /^Weight of other:all ages/ }))
  editor = screen.getByRole('dialog')
  expect((within(editor).getByRole('spinbutton', { name: 'Weight' }) as HTMLInputElement).value).toBe('0')
  expect(within(editor).queryByRole('button', { name: 'Remove pin' })).toBeNull()
})

test('pins, ignores and removes a pin, then shows the new weights', async () => {
  let answer = weights
  const fetchMock = stubWeights(() => Response.json(answer), (init) => init?.method === 'POST'
    ? Response.json({ id: 9, kind: 'tag', label: 'artist:alpha', weight: 2.5 })
    : new Response(null, { status: 204 }))
  const user = userEvent.setup()
  render(<GalleryDetail id={1} />)

  await user.click(await screen.findByRole('button', { name: /^Weight of artist:alpha/ }))
  const field = within(screen.getByRole('dialog')).getByRole('spinbutton', { name: 'Weight' })
  await user.clear(field)
  expect((within(screen.getByRole('dialog')).getByRole('button', { name: 'Pin' }) as HTMLButtonElement).disabled).toBe(true)
  await user.type(field, '2.5')
  answer = { ...weights, tags: weights.tags.map((tag) => tag.tag === 'artist:alpha' ? { ...tag, weight: 2.5, pin: { id: 9, kind: 'tag', label: 'artist:alpha', weight: 2.5 } } : tag) }
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Pin' }))
  expect(await screen.findByRole('button', { name: 'Pinned weight of artist:alpha: +2.50' })).toBeTruthy()
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(fetchMock).toHaveBeenCalledWith(pinsPath, expect.objectContaining({ method: 'POST', body: '{"kind":"tag","value":"artist:alpha","weight":2.5}' }))
  expect(tagRequests(fetchMock)).toHaveLength(2)

  await user.click(screen.getByRole('button', { name: /^Weight of other:all ages/ }))
  answer = { ...weights, tags: weights.tags.map((tag) => tag.tag === 'other:all ages' ? { ...tag, weight: 0, pin: { id: 10, kind: 'tag', label: 'other:all ages', weight: 0 } } : tag) }
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Ignore' }))
  expect(await screen.findByRole('button', { name: 'Pinned weight of other:all ages: +0.00' })).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith(pinsPath, expect.objectContaining({ method: 'POST', body: '{"kind":"tag","value":"other:all ages","weight":0}' }))

  await user.click(screen.getByRole('button', { name: /^Pinned weight of artist:zeta/ }))
  answer = { ...weights, tags: weights.tags.map((tag) => tag.tag === 'artist:zeta' ? { tag: tag.tag, known: true, weight: -0.5 } : tag) }
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Remove pin' }))
  expect(await screen.findByRole('button', { name: 'Weight of artist:zeta: -0.50' })).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith(`${pinsPath}/7`, expect.objectContaining({ method: 'DELETE' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(tagRequests(fetchMock)).toHaveLength(4)
})

test('shows a failed pin change in the open editor and keeps the weights', async () => {
  const fetchMock = stubWeights(() => Response.json(weights), () => Response.json({ error: 'tag_not_found' }, { status: 404 }))
  const user = userEvent.setup()
  render(<GalleryDetail id={1} />)
  await user.click(await screen.findByRole('button', { name: /^Weight of artist:alpha/ }))
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Pin' }))
  const alert = await within(screen.getByRole('dialog')).findByRole('alert')
  expect(alert.textContent).toBe('Could not change the pin. No gallery in the Panda catalog has this tag.')
  expect(tagRequests(fetchMock)).toHaveLength(1)
  expect(screen.getByRole('button', { name: 'Weight of artist:alpha: +0.42' }).textContent).toBe('+0.42')
})

test('hides the old weights and pins when the reload after a pin change fails', async () => {
  let answer = () => Response.json(weights)
  const fetchMock = stubWeights(() => answer())
  const user = userEvent.setup()
  render(<GalleryDetail id={1} />)
  await user.click(await screen.findByRole('button', { name: /^Pinned weight of artist:zeta/ }))
  answer = () => Response.json({ error: 'collector_unavailable' }, { status: 502 })
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Remove pin' }))
  await waitFor(() => expect(screen.queryByRole('button', { name: /eight of/ })).toBeNull())
  expect(tagRequests(fetchMock)).toHaveLength(2)
  expect(screen.getByRole('region', { name: 'Tags' }).querySelector('svg')).toBeNull()
})

test('keeps the tag selection beside the weight buttons', async () => {
  stubWeights(() => Response.json(weights))
  const user = userEvent.setup()
  render(<GalleryDetail id={1} />)
  const artist = within(await screen.findByRole('list', { name: 'artist tags' })).getByRole('button', { name: 'alpha' })
  await user.click(await screen.findByRole('button', { name: /^Weight of artist:alpha/ }))
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('button', { pressed: true })).toBeNull()
  await user.click(artist)
  expect(artist.getAttribute('aria-pressed')).toBe('true')
  expect(screen.getByRole('link', { name: 'Search selected tags (1)' }).getAttribute('href')).toBe('#/galleries?q=artist%3Aalpha%24')
})

test.each([
  ['collector not configured', () => Response.json({ error: 'collector_not_configured' }, { status: 503 })],
  ['collector unavailable', () => Response.json({ error: 'collector_unavailable' }, { status: 502 })],
  ['no boolean active', () => Response.json({ active: 'yes', tags: weights.tags })],
  ['no tags array', () => Response.json({ active: false })],
])('shows no weights or pin controls when the tag weights fail (%s)', async (_, answer) => {
  let answered!: () => void
  const done = new Promise<void>((resolve) => { answered = resolve })
  stubWeights(() => { answered(); return answer() })
  render(<GalleryDetail id={1} />)
  const tags = await screen.findByRole('region', { name: 'Tags' })
  await act(async () => { await done; await new Promise((resolve) => setTimeout(resolve, 0)) })
  expect(within(tags).getAllByRole('listitem').map((item) => item.textContent)).toEqual(['alpha', 'zeta', 'alpha', 'all ages'])
  expect(screen.queryByRole('button', { name: /eight of/ })).toBeNull()
  expect(screen.queryByText(/No score model is active/)).toBeNull()
  expect(screen.queryByRole('alert')).toBeNull()
})
