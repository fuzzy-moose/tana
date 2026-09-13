import { request } from '../api'

export interface RefreshCandidate {
  source_id: number
  library_id: number
  library_name: string
  path: string
  galleries: { id: number, title: string, deleted: boolean, pages_removed: number }[]
}

export interface RefreshPlan {
  plan_id: string
  candidates: RefreshCandidate[]
  skipped: { library_id: number, library_name: string, source_id?: number, path: string, reason: string }[]
}

export interface RefreshResult {
  removed: number[]
  failed: { source_id: number, reason: string }[]
}

const messages: Record<string, string> = {
  refresh_preview_invalid: 'This preview is no longer valid. Check again before removing sources.',
  not_found: 'This library no longer exists. Return to Libraries to see current libraries.',
  operation_timeout: 'The operation timed out. Check again before removing sources.',
  request_canceled: 'The request was canceled. Check again before removing sources.',
}

export function previewRefresh(libraryID: number | undefined, signal: AbortSignal) {
  return request<RefreshPlan>(`/api/libraries${libraryID === undefined ? '' : `/${libraryID}`}/refresh`, { signal }, messages)
}

export function removeMissingSources(planID: string, sourceIDs: number[]) {
  return request<RefreshResult>('/api/libraries/refresh', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ plan_id: planID, source_ids: sourceIDs }),
  }, messages)
}
