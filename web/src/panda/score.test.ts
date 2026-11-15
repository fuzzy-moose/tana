import { expect, test } from 'vitest'
import { currentHiddenShare, hiddenShare, recentHiddenShare } from './score'
import type { HeldOutClass, RecentCatalog, ScorePreview } from './score'

const effect: HeldOutClass = { total: 10, always_hidden: 1, never_hidden: 1, scores: { first_bucket: -2, counts: [2, 2, 4] } }

test.each([
  [-3, 0.1], [-2, 0.1], [-1, 0.3], [0, 0.5], [1, 0.9], [5, 0.9],
])('hiddenShare at %i tenths is %f', (threshold, share) => {
  expect(hiddenShare(effect, threshold)).toBeCloseTo(share)
})

test('hiddenShare is 0 without galleries', () => {
  expect(hiddenShare({ total: 0, always_hidden: 0, never_hidden: 0, scores: { first_bucket: 0, counts: [] } }, 5)).toBe(0)
})

const recent: RecentCatalog = { total: 20, interesting: 1, not_for_me: 2, never_hidden: 1, always_hidden: 3, candidate: null, active: null }

test.each([
  [{ first_bucket: -2, counts: [2, 2, 4] }, -2, 5 / 20],
  [{ first_bucket: -2, counts: [2, 2, 4] }, 0, 9 / 20],
  [{ first_bucket: -2, counts: [2, 2, 4] }, 3, 13 / 20],
  [null, 3, 5 / 20],
])('recentHiddenShare of %j at %i tenths is %f', (histogram, threshold, share) => {
  expect(recentHiddenShare(recent, histogram, threshold)).toBeCloseTo(share)
})

test('recentHiddenShare is 0 without galleries', () => {
  expect(recentHiddenShare({ ...recent, total: 0 }, null, 0)).toBe(0)
})

function preview(overrides: Partial<ScorePreview>): ScorePreview {
  return {
    favorites: false,
    evidence: { interesting: 0, not_for_me: 0, neutral: 0, counted_favorites: 0, favorites_synced_at: null, new_verdicts: null, few_verdicts: false },
    candidate: null,
    active: null,
    overrides: [],
    pins: [],
    author_bonus: null,
    favorite_author_count: 0,
    favorite_authors: [],
    recent: { ...recent, active: { first_bucket: -2, counts: [2, 2, 4] } },
    ...overrides,
  }
}

test.each([
  ['no active model and no overrides', preview({}), 0],
  ['overrides without an active model', preview({ overrides: [{ id: 1, kind: 'tag', label: 'a:b', rule: 'always_hide' }] }), 5 / 20],
  ['an active model', preview({ active: { calculated_at: '2026-11-01T10:00:00Z', favorites: false, threshold_tenths: -1 } }), 7 / 20],
])('currentHiddenShare with %s', (_, value, share) => {
  expect(currentHiddenShare(value)).toBeCloseTo(share)
})
