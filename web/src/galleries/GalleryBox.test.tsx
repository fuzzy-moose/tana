// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, expect, test, vi } from 'vitest'
import GalleryBox from './GalleryBox'
import type { TagSuggestion } from './api'

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

const tags = (): TagSuggestion[] => [
  { namespace: 'artist', value: 'someone', term: 'artist:someone' },
  { namespace: 'female', value: 'glasses', term: 'female:glasses' },
]

test('a card rendered again with the same tags keeps the strip observer', () => {
  const calls: { method: string, target: Element | null }[] = []
  vi.stubGlobal('ResizeObserver', class {
    observe(target: Element) { calls.push({ method: 'observe', target }) }
    unobserve() {}
    disconnect() { calls.push({ method: 'disconnect', target: null }) }
  })
  const box = (galleryTags: TagSuggestion[]) => <Theme>
    <GalleryBox title="Gallery" cover={() => null} pageCount={null} tags={galleryTags} />
  </Theme>

  const { rerender } = render(box(tags()))
  expect(calls).toEqual([{ method: 'observe', target: screen.getByRole('list', { name: 'Tags' }) }])
  rerender(box(tags()))
  expect(calls).toHaveLength(1)
})
