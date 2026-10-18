import { request } from '../api'
import { collectorMessages } from './api'

export interface LibraryDeliveryItem {
  gallery_id: number
  state: 'queued' | 'transferring' | 'transferred' | 'saved' | 'importing' | 'cleanup_pending' | 'completed' | 'skipped' | 'failed'
  error?: string
  cleanup_attempts: number
}

export interface LibraryDeliveryBatch {
  id: number
  library_id: number
  state: 'running' | 'paused' | 'stopped' | 'completed' | 'completed_with_errors'
  stop_requested: boolean
  items: LibraryDeliveryItem[]
  error?: string
}

export type LibraryDeliveryAction = 'resume' | 'stop' | 'retry' | 'retry-cleanup'
export type LibraryDeliveryRequest = { library_id: number } & ({ gallery_id: number } | { all: true })

const messages: Record<string, string> = {
  ...collectorMessages,
  delivery_active: 'Another delivery is active. Finish or stop it before starting another.',
  delivery_not_found: 'This delivery is no longer available. Refresh the delivery history.',
  download_not_found: 'This download is no longer available. Refresh the download list.',
  download_state_conflict: 'Only completed collector downloads can be added to a library.',
  library_not_found: 'The destination library no longer exists. Choose another library.',
  library_unavailable: 'The destination library is unavailable. Restore access and try again.',
  invalid_delivery: 'Choose a destination library and completed downloads. Refresh the lists if this delivery is no longer available.',
}

const base = '/api/library-deliveries'
export const listLibraryDeliveries = (signal: AbortSignal) => request<{ batches: LibraryDeliveryBatch[] }>(base, { signal }, messages)
export const startLibraryDelivery = (body: LibraryDeliveryRequest, signal: AbortSignal) => request<LibraryDeliveryBatch>(base, {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body), signal,
}, messages)
export const changeLibraryDelivery = (id: number, action: LibraryDeliveryAction, signal: AbortSignal) => request<LibraryDeliveryBatch>(
  `${base}/${id}/${action}`, { method: 'POST', signal }, messages,
)

export const deliveryActive = (batch: LibraryDeliveryBatch) => batch.state === 'running' || batch.state === 'paused'
export const deliveryItemState = (item: LibraryDeliveryItem) => ({
  queued: 'Waiting', transferring: 'Transferring', transferred: 'Saving archive', saved: 'Ready to import',
  importing: 'Importing', cleanup_pending: 'Imported · collector cleanup pending', completed: 'Added to library',
  skipped: 'Already in library', failed: 'Failed',
})[item.state]
