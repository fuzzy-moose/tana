import { request } from '../api'
import { collectorMessages } from '../collector/api'

export type ScoreKind = 'tag' | 'uploader' | 'category'
export type ScoreRule = 'always_hide' | 'never_hide'
export type NoScoreModel = 'no_verdicts' | 'no_not_for_me' | 'no_acceptable'

/** The names of the score override rules, as the user reads them. */
export const scoreRuleNames: Record<ScoreRule, string> = { always_hide: 'Always hide', never_hide: 'Never hide' }

/** `counts[i]` is the number of galleries with `floor(score * 10) == first_bucket + i`. */
export interface ScoreHistogram {
  first_bucket: number
  counts: number[]
}

export interface ScoreEvidence {
  interesting: number
  not_for_me: number
  neutral: number
  counted_favorites: number
  favorites_synced_at: string | null
  new_verdicts: number | null
  few_verdicts: boolean
}

export interface ScoreWeight {
  kind: ScoreKind
  label: string
  weight: number
  interesting: number
  not_for_me: number
  neutral: number
  active_weight: number | null
}

export interface HeldOutClass {
  total: number
  always_hidden: number
  never_hidden: number
  scores: ScoreHistogram
}

export interface ScoreCandidate {
  id: number
  calculated_at: string
  favorites: boolean
  weight_count: number
  suggested_threshold_tenths: number
  weights: ScoreWeight[]
  held_out: { interesting: HeldOutClass, not_for_me: HeldOutClass, neutral: HeldOutClass }
}

export interface ActiveScoreModel {
  calculated_at: string
  favorites: boolean
  threshold_tenths: number
}

export interface ScoreOverride {
  id: number
  kind: ScoreKind
  label: string
  rule: ScoreRule
}

export interface RecentCatalog {
  total: number
  interesting: number
  not_for_me: number
  never_hidden: number
  always_hidden: number
  candidate: ScoreHistogram | null
  active: ScoreHistogram | null
}

export interface ScorePreview {
  favorites: boolean
  evidence: ScoreEvidence
  candidate: ScoreCandidate | null
  no_model?: NoScoreModel
  active: ActiveScoreModel | null
  overrides: ScoreOverride[]
  recent: RecentCatalog
}

const defaultFilterInvalid = 'The default filter is invalid. Correct it in the Panda listing.'

const messages = {
  ...collectorMessages,
  invalid_query: defaultFilterInvalid,
  invalid_category: defaultFilterInvalid,
}

/** `favorites` null calculates with the active model's setting. */
export const previewPandaScore = (favorites: boolean | null, signal: AbortSignal) => request<ScorePreview>('/api/collector/catalog/score/preview', {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ favorites }), signal,
}, messages)

const changeMessages = {
  ...collectorMessages,
  score_candidate_not_found: 'This calculation is no longer available. Check the new calculation and apply again.',
  score_model_not_active: 'No score model is active any more.',
  invalid_threshold: 'Enter a valid threshold.',
}

export const applyPandaScore = (candidateID: number, thresholdTenths: number, signal: AbortSignal) => request<void>('/api/collector/catalog/score/apply', {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ candidate_id: candidateID, threshold_tenths: thresholdTenths }), signal,
}, changeMessages)

export const savePandaScoreThreshold = (thresholdTenths: number, signal: AbortSignal) => request<void>('/api/collector/catalog/score/threshold', {
  method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ threshold_tenths: thresholdTenths }), signal,
}, changeMessages)

export const turnOffPandaScore = (signal: AbortSignal) => request<void>('/api/collector/catalog/score/model', { method: 'DELETE', signal }, changeMessages)

const overrideMessages = {
  ...collectorMessages,
  tag_not_found: 'No gallery in the Panda catalog has this tag.',
  invalid_override: 'Enter a tag as namespace:value, an uploader name, or a Panda gallery category.',
}

export const savePandaScoreOverride = (override: { kind: ScoreKind, value: string, rule: ScoreRule }, signal: AbortSignal) => request<void>('/api/collector/catalog/score/overrides', {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ kind: override.kind, value: override.value, rule: override.rule }), signal,
}, overrideMessages)

export const removePandaScoreOverride = (id: number, signal: AbortSignal) => request<void>(`/api/collector/catalog/score/overrides/${id}`, { method: 'DELETE', signal }, overrideMessages)

function countBelow(histogram: ScoreHistogram | null, thresholdTenths: number) {
  if (!histogram) return 0
  return histogram.counts.reduce((sum, count, index) => histogram.first_bucket + index < thresholdTenths ? sum + count : sum, 0)
}

/** The share of a held-out class that a threshold hides. */
export function hiddenShare(effect: HeldOutClass, thresholdTenths: number) {
  if (effect.total === 0) return 0
  return (effect.always_hidden + countBelow(effect.scores, thresholdTenths)) / effect.total
}

/** The share of the recent catalog sample that the listing would hide with a model's histogram and threshold. */
export function recentHiddenShare(recent: RecentCatalog, histogram: ScoreHistogram | null, thresholdTenths: number) {
  if (recent.total === 0) return 0
  return (recent.not_for_me + recent.always_hidden + countBelow(histogram, thresholdTenths)) / recent.total
}

/** The share of the recent catalog sample that the listing hides now: nothing while the score filter is not in force. */
export function currentHiddenShare(preview: ScorePreview) {
  const { active, overrides, recent } = preview
  if (active) return recentHiddenShare(recent, recent.active, active.threshold_tenths)
  if (overrides.length === 0) return 0
  return recentHiddenShare(recent, null, 0)
}
