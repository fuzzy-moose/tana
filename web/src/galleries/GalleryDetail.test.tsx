// @vitest-environment jsdom
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import GalleryDetail from './GalleryDetail'

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

test('displays namespace groups between metadata and pages with read-only tag values', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({
    id: 'g', title: 'Manga', page_count: 1,
    tags: [
      { namespace: 'artist', value: 'alpha' },
      { namespace: 'artist', value: 'zeta' },
      { namespace: 'custom', value: 'alpha' },
      { namespace: 'other', value: 'all ages' },
    ],
  })))
  render(<GalleryDetail id="g" />)
  const tags = await screen.findByRole('region', { name: 'Tags' })
  expect(within(tags).getAllByRole('term').map((term) => term.textContent)).toEqual(['artist', 'custom', 'Other'])
  expect(within(tags).getAllByRole('listitem').map((item) => item.textContent)).toEqual(['alpha', 'zeta', 'alpha', 'all ages'])
  expect(within(tags).queryByRole('link')).toBeNull()
  expect(within(tags).queryByRole('button')).toBeNull()
  expect(screen.getByRole('link', { name: 'Read gallery' }).compareDocumentPosition(tags) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(tags.compareDocumentPosition(screen.getByRole('heading', { name: 'Pages' })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

test('hides the tag section when the gallery has no tags', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ id: 'g', title: 'Manga', page_count: 0, tags: [] })))
  render(<GalleryDetail id="g" />)
  await screen.findByRole('heading', { name: 'Manga' })
  expect(screen.queryByRole('region', { name: 'Tags' })).toBeNull()
})
