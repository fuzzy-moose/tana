import { expect, test } from 'vitest'
import type { CollectorSummary } from './api'
import { attentionItems } from './attention'

function summary(overrides: Partial<CollectorSummary> = {}): CollectorSummary {
  return {
    downloads: { active: 0, failed: 0 },
    favorites: { syncing: false },
    metadata: { main_background_paused: false },
    sitemap: { state: 'idle', children_total: 0, children_processed: 0 },
    feeds: { failed_captures: 0 },
    reference_imports: { active: 0 },
    proxies: { enabled: false, channels_in_cooldown: 0 },
    ...overrides,
  }
}

const storage = { paused: true, reason: 'Free space is below 20 GiB', available_bytes: 1, pause_below_bytes: 2, resume_at_bytes: 3 }

test('a healthy or missing summary has no attention items', () => {
  expect(attentionItems(undefined)).toEqual([])
  expect(attentionItems(summary())).toEqual([])
})

test('each condition produces its item with the specified action', () => {
  const until = '2026-10-18T14:32:00Z'
  expect(attentionItems(summary({ favorites: { syncing: false, authenticated_cooldown_until: until } }))).toEqual([{
    tone: 'amber', title: 'Authenticated cooldown',
    detail: `Favorites and archive preparation paused until ${new Date(until).toLocaleString()}`,
    action: { label: 'Reset cooldown', onReset: 'authenticated' },
  }])
  expect(attentionItems(summary({ metadata: { main_background_paused: false, upstream_cooldown_until: until } }))[0]).toMatchObject({
    tone: 'amber', title: 'Main cooldown', action: { onReset: 'main' },
  })
  expect(attentionItems(summary({ downloads: { active: 0, failed: 0, storage } }))[0]).toMatchObject({
    tone: 'amber', detail: 'Free space is below 20 GiB', action: { href: '#/collector/downloads' },
  })
  expect(attentionItems(summary({ downloads: { active: 0, failed: 3, latest_error: 'timed out' } }))).toEqual([{
    tone: 'red', title: '3 downloads failed', detail: 'Latest: timed out',
    action: { label: 'Review failed', href: '#/collector/downloads?state=failed' },
  }])
  expect(attentionItems(summary({ feeds: { failed_captures: 1, latest_error: 'bad XML' } }))).toEqual([{
    tone: 'red', title: '1 raw feed capture failed', detail: 'bad XML',
    action: { label: 'Open raw feeds', href: '#/collector/feeds?state=failed' },
  }])
})

test('items keep the specified order', () => {
  const until = '2026-10-18T14:32:00Z'
  const items = attentionItems(summary({
    downloads: { active: 0, failed: 1, storage },
    favorites: { syncing: false, authenticated_cooldown_until: until },
    metadata: { main_background_paused: false, upstream_cooldown_until: until },
    feeds: { failed_captures: 2 },
  }))
  expect(items.map((item) => item.title)).toEqual(['Authenticated cooldown', 'Main cooldown', 'Downloads paused', '1 download failed', '2 raw feed captures failed'])
})
