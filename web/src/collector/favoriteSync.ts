import type { FavoriteCategory, FavoritesStatus } from './api'
import { categoryName } from './favoriteCategories'

/** What the tracked categories are doing, by precedence: syncing, waiting, queued, never synced, synced. */
export type FavoriteSyncActivity =
  | { kind: 'syncing' | 'queued' | 'never' }
  | { kind: 'waiting', at?: string }
  | { kind: 'synced', at: string }

export interface FavoriteSyncView {
  /** The button text: the category name with one saved download category, else a general name. */
  label: string
  /** The saved download category names as a tooltip, since the narrow sidebar can truncate the label. */
  title?: string
  /** The saved download categories. Unsaved checkbox changes on the Favorites page are not part of them. */
  categories: FavoriteCategory[]
  /** The categories whose state the quick action shows: the saved ones once the baseline is ready, else all ten. */
  tracked: FavoriteCategory[]
  /** Whether a click starts or resumes the baseline sync of all ten categories. */
  startsBaseline: boolean
  /** Whether no category has auto-download saved, so the button is disabled with a hint. */
  noCategories: boolean
  /** Undefined when nothing is tracked or no category is saved. */
  activity?: FavoriteSyncActivity
  failure?: string
  progress?: string
  /**
   * Whether a tracked category is running or queued and no category waits for cooldown. The collector's
   * single favorites worker waits out a cooldown before it claims anything else, so polling fast then is useless.
   */
  fastPoll: boolean
}

const earliest = (values: (string | undefined)[]) =>
  values.filter((value): value is string => !!value).sort((a, b) => Date.parse(a) - Date.parse(b))[0] as string | undefined

function activityOf(status: FavoritesStatus, tracked: FavoriteCategory[]): FavoriteSyncActivity | undefined {
  if (tracked.length === 0) return undefined
  if (tracked.some((item) => item.state === 'running')) return { kind: 'syncing' }
  const waiting = tracked.filter((item) => item.state === 'waiting_cooldown')
  if (waiting.length > 0) return { kind: 'waiting', at: earliest(waiting.map((item) => item.retry_at)) ?? status.authenticated_cooldown_until }
  if (tracked.some((item) => item.state === 'queued')) return { kind: 'queued' }
  if (tracked.some((item) => !item.last_synced_at)) return { kind: 'never' }
  return { kind: 'synced', at: earliest(tracked.map((item) => item.last_synced_at))! }
}

function failureOf(tracked: FavoriteCategory[]): string | undefined {
  const failed = tracked.filter((item) => item.state === 'idle' && item.last_outcome === 'failed')
    .sort((a, b) => a.category - b.category)
  if (failed.length === 0) return undefined
  const error = failed[0].last_error || 'Unknown error'
  if (tracked.length === 1) return `Failed: ${error}`
  if (failed.length === 1) return `${categoryName(failed[0])} failed: ${error}`
  return `${failed.length} categories failed: ${error}`
}

export function favoriteSyncView(status: FavoritesStatus): FavoriteSyncView {
  const downloads = status.downloads
  const categories = (downloads?.categories ?? []).map((id) =>
    status.categories.find((category) => category.category === id) ?? { category: id, name: '' } as FavoriteCategory)
  const names = categories.map(categoryName)
  const ready = downloads?.baseline_state === 'ready'
  const tracked = ready ? categories : status.categories
  return {
    label: categories.length === 1 ? `Sync ${names[0]}` : 'Sync favorites',
    title: categories.length > 0 ? names.join(', ') : undefined,
    categories,
    tracked,
    startsBaseline: categories.length > 0 && !ready,
    noCategories: categories.length === 0,
    activity: categories.length > 0 ? activityOf(status, tracked) : undefined,
    failure: categories.length > 0 ? failureOf(tracked) : undefined,
    progress: downloads?.baseline_state === 'collecting' ? `Baseline: ${downloads.baseline_categories} of 10 categories` : undefined,
    fastPoll: tracked.some((item) => item.state === 'running' || item.state === 'queued')
      && !status.categories.some((item) => item.state === 'waiting_cooldown'),
  }
}

const listeners = new Set<() => void>()

/** Tells the quick action that the Favorites page saved download categories or requested a sync. */
export function notifyFavoritesChanged() {
  listeners.forEach((listener) => listener())
}

/** Calls `listener` on every `notifyFavoritesChanged`; returns the unsubscribe function. */
export function subscribeFavoritesChanged(listener: () => void) {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}
