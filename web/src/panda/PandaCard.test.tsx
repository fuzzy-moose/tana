// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, expect, test } from 'vitest'
import PandaCard from './PandaCard'
import type { PandaGallery } from './api'

afterEach(cleanup)

test('provisional cards distinguish unknown dates and collection outcomes from collected data', () => {
  const gallery: PandaGallery = {
    gallery_id: 1, title: 'Feed gallery', thumbnail_url: '', url: 'https://example.test/g/1/token/',
    page_count: null, posted_at: null, first_seen_at: '2026-09-16T10:00:00Z',
    metadata_source: 'feed', metadata_state: 'waiting', fetch_eligible_at: '2026-09-16T22:00:00Z',
  }
  const { rerender } = render(<Theme><PandaCard gallery={gallery} /></Theme>)
  expect(screen.getByText(/Pages unknown · First seen/)).toBeTruthy()
  expect(screen.getByText('Feed metadata')).toBeTruthy()
  expect(screen.getByText('Waiting')).toBeTruthy()
  expect(screen.getByText(/Eligible from/).querySelector('time')?.dateTime).toBe(gallery.fetch_eligible_at)
  expect(screen.queryByText(/Invalid Date/)).toBeNull()
  rerender(<Theme><PandaCard gallery={{ ...gallery, metadata_state: 'failed' }} /></Theme>)
  expect(screen.getByText('Collection failed')).toBeTruthy()
  rerender(<Theme><PandaCard gallery={{ ...gallery, metadata_source: 'collected', page_count: 1, posted_at: '2026-09-16T09:00:00Z' }} /></Theme>)
  expect(screen.getByText(/1 page ·/)).toBeTruthy()
  expect(screen.queryByText(/Feed metadata|First seen|Eligible from/)).toBeNull()
})
