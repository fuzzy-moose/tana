import { request } from '../api'
import { collectorMessages } from './api'

export interface ReferenceImport {
  id: string
  filename: string
  status: 'processing' | 'validating' | 'completed' | 'cancelled'
  created_at: string
  completed_at?: string
  size_bytes: number
  processed_bytes: number
  references: number
  duplicates: number
  invalid: number
  known: number
  imported: number
  failed: number
  pending: number
  cancelled: number
}

const base = '/api/collector/reference-imports'
export const referenceImportPageSize = 10
export const referenceImportMaxBytes = 100 * 1024 * 1024
const messages: Record<string, string> = {
  ...collectorMessages,
  import_too_large: 'Each reference file must be 100 MiB or smaller.',
  import_not_found: 'This import is no longer available. Refresh the import history.',
  import_state_conflict: 'The import state changed. Check the refreshed history and try again.',
  invalid_upload: 'The file was not fully received. Upload it again from the beginning.',
  invalid_filename: 'The reference file needs a filename.',
}

export const listReferenceImports = (offset: number, signal: AbortSignal) => request<{ imports: ReferenceImport[] }>(
  `${base}?limit=${referenceImportPageSize + 1}&offset=${offset}`, { signal }, messages,
)

export const changeReferenceImport = (id: string, action: 'cancel' | 'retry', signal: AbortSignal) => request<ReferenceImport>(
  `${base}/${encodeURIComponent(id)}/${action}`, { method: 'POST', signal }, messages,
)

export function uploadReferenceImport(file: File, signal: AbortSignal, onProgress: (bytes: number) => void): Promise<ReferenceImport> {
  if (file.size > referenceImportMaxBytes) return Promise.reject(new Error('Each reference file must be 100 MiB or smaller.'))
  return new Promise((resolve, reject) => {
    if (signal.aborted) { reject(new DOMException('Upload aborted', 'AbortError')); return }
    const xhr = new XMLHttpRequest()
    const abort = () => xhr.abort()
    const finish = (error?: Error, result?: ReferenceImport) => {
      signal.removeEventListener('abort', abort)
      if (error) reject(error)
      else resolve(result!)
    }
    xhr.open('POST', `${base}?filename=${encodeURIComponent(file.name)}`)
    xhr.setRequestHeader('Accept', 'application/json')
    xhr.setRequestHeader('Content-Type', 'text/plain; charset=utf-8')
    xhr.upload.onprogress = (event) => onProgress(Math.min(event.loaded, file.size))
    xhr.upload.onload = () => onProgress(file.size)
    xhr.onerror = () => finish(new Error('Upload could not be confirmed. Check import history before uploading the file again.'))
    xhr.onabort = () => finish(new DOMException('Upload aborted', 'AbortError'))
    xhr.onload = () => {
      let body
      try { body = JSON.parse(xhr.responseText) } catch { /* Report malformed responses below. */ }
      if (xhr.status < 200 || xhr.status >= 300) {
        finish(new Error(messages[body?.error] ?? `Tana could not accept the file (${xhr.status}). Check import history before trying again.`))
      } else if (!body?.id || !body?.status) {
        finish(new Error('Upload could not be confirmed. Check import history before uploading the file again.'))
      } else finish(undefined, body as ReferenceImport)
    }
    signal.addEventListener('abort', abort, { once: true })
    xhr.send(file)
  })
}
