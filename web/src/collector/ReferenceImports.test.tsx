// @vitest-environment jsdom
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'
import ReferenceImports from './ReferenceImports'
import { uploadReferenceImport } from './referenceImports'
import type { ReferenceImport } from './referenceImports'

vi.mock('./referenceImports', async (importOriginal) => ({
  ...await importOriginal<typeof import('./referenceImports')>(),
  uploadReferenceImport: vi.fn(),
}))

afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.clearAllMocks() })

const validating: ReferenceImport = {
  id: 'first', filename: 'favorites.jsonl', status: 'validating', created_at: '2026-09-11T10:00:00Z',
  size_bytes: 1024, processed_bytes: 1024, references: 100, duplicates: 3, invalid: 2,
  known: 10, imported: 20, failed: 5, pending: 65, cancelled: 0,
}

test('shows independent processing and validation progress, with cancellation and failed-reference retry', async () => {
  let imports: ReferenceImport[] = [
    validating,
    { ...validating, id: 'second', filename: 'failed.jsonl', status: 'completed', failed: 70, pending: 0 },
  ]
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    if (init?.method === 'POST') {
      const cancelled = String(input).endsWith('/cancel')
      const id = cancelled ? 'first' : 'second'
      imports = imports.map((item) => item.id !== id ? item : cancelled
        ? { ...item, status: 'cancelled', cancelled: item.pending, pending: 0 }
        : { ...item, status: 'validating', pending: item.failed, failed: 0 })
      return Response.json(imports.find((item) => item.id === id))
    }
    return Response.json({ imports })
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(<ReferenceImports available refreshKey={0} />)
  const active = await screen.findByRole('article', { name: 'Import favorites.jsonl' })
  const processing = within(active).getByRole('progressbar', { name: 'File processing for favorites.jsonl' }) as HTMLProgressElement
  const validation = within(active).getByRole('progressbar', { name: 'Reference validation for favorites.jsonl' }) as HTMLProgressElement
  expect(processing.value).toBe(processing.max)
  expect(validation.value).toBe(35)
  expect(validation.max).toBe(100)
  expect(within(active).queryByRole('button', { name: 'Retry failed' })).toBeNull()
  await user.click(within(active).getByRole('button', { name: 'Cancel' }))
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/reference-imports/first/cancel', expect.objectContaining({ method: 'POST' }))
  expect(within(active).queryByRole('button', { name: 'Cancel' })).toBeNull()
  const failed = screen.getByRole('article', { name: 'Import failed.jsonl' })
  await user.click(within(failed).getByRole('button', { name: 'Retry failed' }))
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/reference-imports/second/retry', expect.objectContaining({ method: 'POST' }))
  expect(await within(failed).findByText('Validating')).toBeTruthy()
})

test('uploads every selected file independently and continues after a failed upload', async () => {
  const imports: ReferenceImport[] = []
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async () => Response.json({ imports })))
  const upload = vi.mocked(uploadReferenceImport)
  upload.mockRejectedValueOnce(new Error('Each reference file must be 100 MiB or smaller.'))
  upload.mockImplementationOnce(async (file, _signal, onProgress) => {
    onProgress(file.size)
    const accepted = { ...validating, filename: file.name, status: 'processing' as const, processed_bytes: 0 }
    imports.push(accepted)
    return accepted
  })
  const user = userEvent.setup()
  render(<ReferenceImports available refreshKey={0} />)
  const oversized = new File(['x'], 'large.jsonl')
  const valid = new File(['{"gid":123,"token":"abc"}\n'], 'small.jsonl')
  await user.upload(screen.getByLabelText('Reference files'), [oversized, valid])
  await user.click(screen.getByRole('button', { name: 'Upload files' }))
  expect(await screen.findByText('1 file accepted. Imports continue on the collector.')).toBeTruthy()
  expect(screen.getByRole('alert').textContent).toContain('large.jsonl: Each reference file must be 100 MiB or smaller.')
  expect(upload).toHaveBeenCalledTimes(2)
  expect(upload.mock.calls.map(([file]) => file)).toEqual([oversized, valid])
  expect(await screen.findByRole('article', { name: 'Import small.jsonl' })).toBeTruthy()
})
