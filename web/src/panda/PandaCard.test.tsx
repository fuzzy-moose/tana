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

test('cards of displayed galleries look subdued and say they were seen', () => {
  const { rerender } = render(<Theme><PandaCard gallery={{ ...collectedGallery, displayed: true }} /></Theme>)
  expect(screen.getByRole('link').classList.contains('gallery-box-subdued')).toBe(true)
  expect(screen.getByRole('link').textContent).toContain('Seen')
  for (const displayed of [false, undefined]) {
    rerender(<Theme><PandaCard gallery={{ ...collectedGallery, displayed }} /></Theme>)
    expect(screen.getByRole('link').classList.contains('gallery-box-subdued')).toBe(false)
    expect(screen.getByRole('link').textContent).not.toContain('Seen')
  }
})

test('details show the score, the page threshold and the strongest weights', () => {
  const scored: PandaGallery = { ...collectedGallery, score: { value: 1.374, weights: [
    { kind: 'tag', label: 'female:glasses', weight: 1.2 },
    { kind: 'uploader', label: 'Some uploader', weight: -0.456 },
  ] } }
  const { rerender } = render(<PandaDetails gallery={scored} scoreFilter={{ threshold_tenths: 5 }} />)
  expect(screen.getByText(/^Score 1\.37/).textContent).toBe('Score 1.37 · Threshold 0.5')
  expect(screen.getByText('+1.20 female:glasses')).toBeTruthy()
  expect(screen.getByText('-0.46 Uploader Some uploader')).toBeTruthy()
  rerender(<PandaDetails gallery={{ ...scored, score: { rule: 'always_hide', override: 'Some uploader' } }} scoreFilter={{}} />)
  expect(screen.queryByText(/^Score/)).toBeNull()
})

test('details name the rule that decides, the verdict first, only on a page with the score filter', () => {
  const { rerender } = render(<PandaDetails gallery={{ ...collectedGallery, verdict: 'not_for_me', score: { rule: 'never_hide', override: 'female:glasses' } }} scoreFilter={{}} />)
  expect(screen.getByText('Hidden by your "Not for me" verdict.')).toBeTruthy()
  expect(screen.queryByText(/Never hidden/)).toBeNull()
  rerender(<PandaDetails gallery={{ ...collectedGallery, verdict: 'interesting', score: { rule: 'always_hide', override: 'female:glasses' } }} scoreFilter={{}} />)
  expect(screen.queryByText(/Hidden/)).toBeNull()
  rerender(<PandaDetails gallery={{ ...collectedGallery, score: { rule: 'always_hide', override: 'Doujinshi' } }} scoreFilter={{}} />)
  expect(screen.getByText('Hidden by the override for Doujinshi.')).toBeTruthy()
  rerender(<PandaDetails gallery={{ ...collectedGallery, score: { rule: 'never_hide', override: 'female:glasses' } }} scoreFilter={{}} />)
  expect(screen.getByText('Never hidden: female:glasses.')).toBeTruthy()
  rerender(<PandaDetails gallery={{ ...collectedGallery, score: { value: -2, rule: 'below_threshold' } }} scoreFilter={{ threshold_tenths: -5 }} />)
  expect(screen.getByText('Hidden: the score is below the threshold.')).toBeTruthy()
  rerender(<PandaDetails gallery={{ ...collectedGallery, verdict: 'not_for_me' }} />)
  expect(screen.queryByText(/Hidden/)).toBeNull()
})
