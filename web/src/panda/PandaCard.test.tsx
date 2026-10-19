// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, expect, test } from 'vitest'
import PandaCard, { PandaDetails } from './PandaCard'
import type { PandaGallery } from './api'

afterEach(cleanup)

const feedGallery: PandaGallery = {
  gallery_id: 1, title: 'Feed gallery', thumbnail_url: '', url: 'https://example.test/g/1/token/',
  page_count: null, posted_at: null, first_seen_at: '2026-09-16T10:00:00Z',
  metadata_source: 'feed', metadata_state: 'waiting', fetch_eligible_at: '2026-09-16T22:00:00Z', summary_tags: [],
}
const collectedGallery: PandaGallery = { ...feedGallery, metadata_source: 'collected', metadata_state: undefined, fetch_eligible_at: undefined, page_count: 1, posted_at: '2026-09-16T09:00:00Z' }

test('cards mark provisional feed metadata with a status badge', () => {
  const { rerender } = render(<Theme><PandaCard gallery={feedGallery} /></Theme>)
  expect(screen.getByText('Waiting for metadata')).toBeTruthy()
  rerender(<Theme><PandaCard gallery={{ ...feedGallery, metadata_state: 'failed' }} /></Theme>)
  expect(screen.getByText('Collection failed')).toBeTruthy()
  rerender(<Theme><PandaCard gallery={collectedGallery} /></Theme>)
  expect(document.querySelector('.gallery-page-count')?.textContent).toBe('1 page')
  expect(document.querySelector('.gallery-cover-status')).toBeNull()
})

test('details distinguish unknown dates and collection outcomes from collected data', () => {
  const { rerender } = render(<PandaDetails gallery={feedGallery} />)
  expect(screen.getByText(/Pages unknown · First seen/)).toBeTruthy()
  expect(screen.getByText('Feed metadata · Waiting for collection')).toBeTruthy()
  expect(screen.getByText(/Eligible from/).querySelector('time')?.dateTime).toBe(feedGallery.fetch_eligible_at)
  expect(screen.queryByText(/Invalid Date/)).toBeNull()
  rerender(<PandaDetails gallery={{ ...feedGallery, metadata_state: 'failed' }} />)
  expect(screen.getByText('Feed metadata · Collection failed')).toBeTruthy()
  rerender(<PandaDetails gallery={collectedGallery} />)
  expect(screen.getByText(/^Posted/)).toBeTruthy()
  expect(screen.queryByText(/Feed metadata|First seen|Eligible from/)).toBeNull()
})
