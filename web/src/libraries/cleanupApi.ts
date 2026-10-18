import { request } from '../api'

export interface CleanupSource {
  id: number
  library_id: number
  library_name: string
  path: string
  panda_id: number
  title: string
}

export interface CleanupCandidate {
  source: CleanupSource
  replacement: CleanupSource
  size_bytes: number
}

export interface CleanupPlan {
  plan_id: string
  candidates: CleanupCandidate[]
  skipped: { source_id: number, path: string, reason: string }[]
}

export interface CleanupResult {
  deleted: number[]
  failed: { source_id: number, reason: string }[]
}

const messages: Record<string, string> = {
  cleanup_preview_invalid: 'This preview is no longer valid. Refresh the preview before deleting.',
  collector_unavailable: 'Collected Panda metadata is unavailable. Check the collector and refresh the preview.',
  collector_not_configured: 'Connect a Panda collector before checking for older versions.',
  operation_timeout: 'The operation timed out. Refresh the preview to check the current sources.',
  request_canceled: 'The request was canceled. Refresh the preview to check the current sources.',
}

export function previewCleanup(signal: AbortSignal) {
  return request<CleanupPlan>('/api/source-cleanup', { signal }, messages)
}

export function deleteSupersededSources(planID: string, sourceIDs: number[]) {
  return request<CleanupResult>('/api/source-cleanup', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ plan_id: planID, source_ids: sourceIDs }),
  }, messages)
}
