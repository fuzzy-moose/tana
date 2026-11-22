import { expect, test } from 'vitest'
import { currentHiddenShare, hiddenShare, recentHiddenShare } from './score'
import type { HeldOut, RecentCatalog, ScorePreview } from './score'

const heldOut: HeldOut = { total: 10, scores: { first_bucket: -2, counts: [2, 2, 6] } }

test.each([
  [-3, 0], [-2, 0], [-1, 0.2], [0, 0.4], [1, 1], [5, 1],
])('hiddenShare at %i tenths is %f', (threshold, share) => {
  expect(hiddenShare(heldOut, threshold)).toBeCloseTo(share)
})

test('hiddenShare is 0 without held-out favorites', () => {
  expect(hiddenShare({ total: 0, scores: { first_bucket: 0, counts: [] } }, 5)).toBe(0)
})

const recent: RecentCatalog = { total: 20, candidate: null, active: null }

test.each([
  [{ first_bucket: -2, counts: [2, 2, 16] }, -2, 0],
  [{ first_bucket: -2, counts: [2, 2, 16] }, 0, 4 / 20],
  [{ first_bucket: -2, counts: [2, 2, 16] }, 3, 1],
  [null, 3, 0],
])('recentHiddenShare of %j at %i tenths is %f', (histogram, threshold, share) => {
  expect(recentHiddenShare(recent, histogram, threshold)).toBeCloseTo(share)
})

test('recentHiddenShare is 0 without galleries', () => {
  expect(recentHiddenShare({ ...recent, total: 0 }, null, 0)).toBe(0)
})

function preview(fields: Partial<ScorePreview>): ScorePreview {
  return {
    evidence: { favorites: 0, favorites_synced_at: null, new_favorites: null, few_favorites: false },
    candidate: null,
    active: null,
    pins: [],
    author_bonus: null,
    favorite_author_count: 0,
    favorite_authors: [],
    recent: { ...recent, active: { first_bucket: -2, counts: [2, 2, 16] } },
    ...fields,
  }
}

test.each([
  ['no active model', preview({}), 0],
  ['an active model', preview({ active: { calculated_at: '2026-11-01T10:00:00Z', threshold_tenths: -1 } }), 2 / 20],
])('currentHiddenShare with %s', (_, value, share) => {
  expect(currentHiddenShare(value)).toBeCloseTo(share)
})
