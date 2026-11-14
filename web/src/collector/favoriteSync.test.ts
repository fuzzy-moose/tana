import { expect, test } from 'vitest'
import type { FavoriteCategory, FavoriteDownloadsStatus, FavoritesStatus } from './api'
import { favoriteSyncView } from './favoriteSync'

const category = (id: number, name = ''): FavoriteCategory => ({
  category: id, name, favorites: 0, state: 'idle', full: false, queued: false, queued_full: false, entries_saved: 0, pages_saved: 0,
})
const status = (downloads?: FavoriteDownloadsStatus): FavoritesStatus => ({
  host: 'https://panda.test', account_key: 'current',
  categories: [category(0, 'Reading list'), category(1, 'Manga'), ...[2, 3, 4, 5, 6, 7, 8, 9].map((id) => category(id))],
  downloads,
})
const ready = (categories: number[]): FavoriteDownloadsStatus => ({ categories, baseline_state: 'ready', baseline_categories: 10 })

test.each([
  ['no downloads status', status(), { label: 'Sync favorites', title: undefined, startsBaseline: false, noCategories: true }],
  ['no saved categories', status(ready([])), { label: 'Sync favorites', title: undefined, startsBaseline: false, noCategories: true }],
  ['one named category', status(ready([1])), { label: 'Sync Manga', title: 'Manga', startsBaseline: false, noCategories: false }],
  ['one unnamed category', status(ready([3])), { label: 'Sync Category 3', title: 'Category 3', startsBaseline: false, noCategories: false }],
  ['several categories', status(ready([0, 1])), { label: 'Sync favorites', title: 'Reading list, Manga', startsBaseline: false, noCategories: false }],
  ['baseline not started', status({ categories: [1], baseline_state: 'not_started', baseline_categories: 0 }), { label: 'Sync Manga', title: 'Manga', startsBaseline: true, noCategories: false }],
  ['baseline collecting', status({ categories: [0, 1], baseline_state: 'collecting', baseline_categories: 4 }), { label: 'Sync favorites', title: 'Reading list, Manga', startsBaseline: true, noCategories: false }],
  ['baseline without categories', status({ categories: [], baseline_state: 'collecting', baseline_categories: 4 }), { label: 'Sync favorites', title: undefined, startsBaseline: false, noCategories: true }],
])('%s', (_, input, expected) => {
  const view = favoriteSyncView(input)
  expect({ label: view.label, title: view.title, startsBaseline: view.startsBaseline, noCategories: view.noCategories }).toEqual(expected)
  expect(view.categories.map((item) => item.category)).toEqual(input.downloads?.categories ?? [])
})

const withCategories = (downloads: FavoriteDownloadsStatus, overrides: Record<number, Partial<FavoriteCategory>>): FavoritesStatus => {
  const base = status(downloads)
  return { ...base, categories: base.categories.map((item) => ({ ...item, ...overrides[item.category] })) }
}
const synced = (at: string): Partial<FavoriteCategory> => ({ last_synced_at: at, last_outcome: 'success' })

test.each([
  ['a running and a failed category', withCategories(ready([0, 1]), { 0: { state: 'running' }, 1: { last_outcome: 'failed', last_error: 'timeout' } }), { kind: 'syncing' }],
  ['a waiting and a queued category', withCategories(ready([0, 1]), { 0: { state: 'waiting_cooldown', retry_at: '2026-11-14T12:00:00Z' }, 1: { state: 'queued', queued: true } }), { kind: 'waiting', at: '2026-11-14T12:00:00Z' }],
  ['the earliest retry among waiting categories', withCategories(ready([0, 1]), { 0: { state: 'waiting_cooldown', retry_at: '2026-11-14T13:00:00Z' }, 1: { state: 'waiting_cooldown', retry_at: '2026-11-14T12:30:00Z' } }), { kind: 'waiting', at: '2026-11-14T12:30:00Z' }],
  ['a queued category', withCategories(ready([0, 1]), { 0: { state: 'queued', queued: true }, 1: synced('2026-11-14T10:00:00Z') }), { kind: 'queued' }],
  ['a category never synced', withCategories(ready([0, 1]), { 0: synced('2026-11-14T10:00:00Z') }), { kind: 'never' }],
  ['all synced', withCategories(ready([0, 1]), { 0: synced('2026-11-14T10:00:00Z'), 1: synced('2026-11-13T10:00:00Z') }), { kind: 'synced', at: '2026-11-13T10:00:00Z' }],
  ['no saved category before the baseline', withCategories({ categories: [], baseline_state: 'not_started', baseline_categories: 0 }, {}), undefined],
  ['activity of an untracked category', withCategories(ready([1]), { 0: { state: 'running' }, 1: synced('2026-11-14T10:00:00Z') }), { kind: 'synced', at: '2026-11-14T10:00:00Z' }],
])('activity with %s', (_, input, expected) => {
  expect(favoriteSyncView(input).activity).toEqual(expected)
})

test.each([
  ['one tracked category failed', withCategories(ready([1]), { 1: { last_outcome: 'failed', last_error: 'timeout' } }), 'Failed: timeout'],
  ['one of several failed', withCategories(ready([0, 1]), { 1: { last_outcome: 'failed', last_error: 'timeout' } }), 'Manga failed: timeout'],
  ['several failed', withCategories(ready([0, 1, 3]), { 3: { last_outcome: 'failed', last_error: 'later' }, 1: { last_outcome: 'failed', last_error: 'first' } }), '2 categories failed: first'],
  ['the failed category queued again', withCategories(ready([0, 1]), { 1: { last_outcome: 'failed', last_error: 'timeout', state: 'queued', queued: true } }), undefined],
  ['the failed category running again', withCategories(ready([0, 1]), { 1: { last_outcome: 'failed', last_error: 'timeout', state: 'running' } }), undefined],
  ['no category is saved before the baseline', withCategories({ categories: [], baseline_state: 'not_started', baseline_categories: 0 }, { 1: { last_outcome: 'failed', last_error: 'timeout' } }), undefined],
  ['an untracked category failed', withCategories(ready([1]), { 0: { last_outcome: 'failed', last_error: 'timeout' } }), undefined],
])('failure when %s', (_, input, expected) => {
  expect(favoriteSyncView(input).failure).toBe(expected)
})

test('tracks all ten categories and shows progress before the baseline is ready', () => {
  const collecting = favoriteSyncView(status({ categories: [1], baseline_state: 'collecting', baseline_categories: 4 }))
  expect(collecting.tracked.map((item) => item.category)).toEqual([0, 1, 2, 3, 4, 5, 6, 7, 8, 9])
  expect(collecting.progress).toBe('Baseline: 4 of 10 categories')
  const notStarted = favoriteSyncView(status({ categories: [], baseline_state: 'not_started', baseline_categories: 0 }))
  expect(notStarted.tracked).toHaveLength(10)
  expect(notStarted.progress).toBeUndefined()
  const done = favoriteSyncView(status(ready([1])))
  expect(done.tracked.map((item) => item.category)).toEqual([1])
  expect(done.progress).toBeUndefined()
})

test.each([
  ['a tracked category running', withCategories(ready([1]), { 1: { state: 'running' } }), true],
  ['a tracked category queued', withCategories(ready([1]), { 1: { state: 'queued', queued: true } }), true],
  ['a tracked waiting and a tracked queued category', withCategories(ready([0, 1]), { 0: { state: 'waiting_cooldown' }, 1: { state: 'queued', queued: true } }), false],
  ['an untracked waiting and a tracked queued category', withCategories(ready([1]), { 0: { state: 'waiting_cooldown' }, 1: { state: 'queued', queued: true } }), false],
  ['only an untracked category active', withCategories(ready([1]), { 0: { state: 'running' } }), false],
  ['nothing active', status(ready([1])), false],
])('fast poll with %s', (_, input, expected) => {
  expect(favoriteSyncView(input).fastPoll).toBe(expected)
})
