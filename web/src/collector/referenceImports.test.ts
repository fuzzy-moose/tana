// @vitest-environment jsdom
import { afterEach, expect, test, vi } from 'vitest'
import { referenceImportMaxBytes, uploadReferenceImport } from './referenceImports'

afterEach(() => vi.unstubAllGlobals())

class UploadRequest {
  upload = { onprogress: (_event: { loaded: number }) => {}, onload: () => {} }
  onload = () => {}
  onerror = () => {}
  onabort = () => {}
  status = 202
  responseText = '{"id":"accepted","status":"processing"}'
  open = vi.fn()
  setRequestHeader = vi.fn()
  send = vi.fn()
  abort = vi.fn(() => this.onabort())
}

test('uploads the original file with its encoded filename and waits for acceptance after transfer', async () => {
  const xhr = new UploadRequest()
  vi.stubGlobal('XMLHttpRequest', class { constructor() { return xhr } })
  const file = new File(['123,abc\n'], '日本 & favorites.txt')
  const progress = vi.fn()
  const promise = uploadReferenceImport(file, new AbortController().signal, progress)
  expect(xhr.open).toHaveBeenCalledWith('POST', '/api/collector/reference-imports?filename=' + encodeURIComponent(file.name))
  expect(xhr.setRequestHeader).toHaveBeenCalledWith('Content-Type', 'text/plain; charset=utf-8')
  expect(xhr.send).toHaveBeenCalledWith(file)
  xhr.upload.onprogress({ loaded: 5 })
  expect(progress).toHaveBeenLastCalledWith(5)
  xhr.upload.onload()
  expect(progress).toHaveBeenLastCalledWith(file.size)
  xhr.onload()
  await expect(promise).resolves.toEqual({ id: 'accepted', status: 'processing' })
})

test('rejects oversized files before starting an upload and leaves interrupted acceptance uncertain', async () => {
  const file = new File([''], 'large.txt')
  Object.defineProperty(file, 'size', { value: referenceImportMaxBytes + 1 })
  await expect(uploadReferenceImport(file, new AbortController().signal, vi.fn())).rejects.toThrow('100 MiB')
  const xhr = new UploadRequest()
  vi.stubGlobal('XMLHttpRequest', class { constructor() { return xhr } })
  const promise = uploadReferenceImport(new File(['x'], 'small.txt'), new AbortController().signal, vi.fn())
  xhr.onerror()
  await expect(promise).rejects.toThrow('Check import history before uploading the file again.')
})
