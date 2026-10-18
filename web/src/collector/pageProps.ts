import type { CollectorSummary } from './api'

export interface CollectorPageProps {
  available: boolean
  refreshKey: number
  /** The page's poll interval in milliseconds. */
  interval: number
  summary?: CollectorSummary
  /** Whether `summary` is outdated because the collector is disconnected or the last reload failed. */
  summaryStale?: boolean
  /** Reloads the summary, e.g. after an action that changes it. */
  refreshSummary?: () => void
}
