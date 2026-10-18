// @vitest-environment jsdom
import { cleanup, render as testingRender, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Theme } from '@radix-ui/themes'
import type { ReactNode } from 'react'

import { afterEach, expect, test, vi } from 'vitest'
import App from '../App'
import ReferenceImports from './ReferenceImports'
import { uploadReferenceImport } from './referenceImports'
import type { ReferenceImport } from './referenceImports'

vi.mock('./referenceImports', async (importOriginal) => ({
  ...await importOriginal<typeof import('./referenceImports')>(),
  uploadReferenceImport: vi.fn(),
}))

const render = (ui: ReactNode) => testingRender(ui, { wrapper: Theme })
const page = (props: Partial<{ available: boolean }> = {}) => <ReferenceImports available refreshKey={0} interval={5000} {...props} />

afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.clearAllMocks(); window.history.replaceState(null, '', '/') })

const validating: ReferenceImport = {
  id: 'first', filename: 'favorites.txt', status: 'validating', paused: false, created_at: '2026-09-11T10:00:00Z',
  size_bytes: 1024, processed_bytes: 1024, references: 100, duplicates: 3, invalid: 2,
  known: 10, imported: 20, failed: 5, pending: 65, cancelled: 0,
}
const row = (name: string) => screen.getByRole('rowheader', { name: new RegExp(name) }).closest('tr')!

test.each(['processing', 'validating'] as const)('pauses and resumes a %s import without losing progress', async (status) => {
  let item: ReferenceImport = { ...validating, status, processed_bytes: status === 'processing' ? 512 : 1024 }
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    if (init?.method === 'POST') {
      item = { ...item, paused: String(input).endsWith('/pause') }
      return Response.json(item)
    }
    return Response.json({ imports: [item], total: 1 })
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(page())
  await screen.findByRole('rowheader', { name: /favorites.txt/ })
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/reference-imports?limit=10&offset=0', expect.anything())
  await user.click(within(row('favorites.txt')).getByRole('button', { name: 'Pause' }))
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/reference-imports/first/pause', expect.objectContaining({ method: 'POST' }))
  expect(await within(row('favorites.txt')).findByText('Paused')).toBeTruthy()
  expect(within(row('favorites.txt')).getByRole('button', { name: 'Cancel' })).toBeTruthy()
  expect(within(row('favorites.txt')).getByText('Progress saved. Resume to continue validation.')).toBeTruthy()
  expect(within(row('favorites.txt')).getByRole('progressbar', { name: 'Reference validation for favorites.txt' }).getAttribute('aria-valuenow')).toBe('35')
  await user.click(within(row('favorites.txt')).getByRole('button', { name: 'Resume' }))
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/reference-imports/first/resume', expect.objectContaining({ method: 'POST' }))
  expect(await within(row('favorites.txt')).findByRole('button', { name: 'Pause' })).toBeTruthy()
  expect(within(row('favorites.txt')).queryByText('Paused')).toBeNull()
})

test('offers cancellation for active imports and failed-reference retry for completed ones', async () => {
  let imports: ReferenceImport[] = [
    validating,
    { ...validating, id: 'second', filename: 'failed.txt', status: 'completed', failed: 70, pending: 0 },
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
    return Response.json({ imports, total: 2 })
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(page())
  await screen.findByRole('rowheader', { name: /favorites.txt/ })
  expect(within(row('favorites.txt')).queryByRole('button', { name: 'Retry failed' })).toBeNull()
  expect(within(row('failed.txt')).queryByRole('button', { name: 'Pause' })).toBeNull()
  await user.click(within(row('favorites.txt')).getByRole('button', { name: 'Cancel' }))
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/reference-imports/first/cancel', expect.objectContaining({ method: 'POST' }))
  expect(await within(row('favorites.txt')).findByText('Cancelled')).toBeTruthy()
  await user.click(within(row('failed.txt')).getByRole('button', { name: 'Retry failed' }))
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/reference-imports/second/retry', expect.objectContaining({ method: 'POST' }))
  expect(await within(row('failed.txt')).findByText('Validating')).toBeTruthy()
})

test('uploads every selected file independently and continues after a failed upload', async () => {
  const imports: ReferenceImport[] = []
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async () => Response.json({ imports, total: imports.length })))
  const upload = vi.mocked(uploadReferenceImport)
  upload.mockRejectedValueOnce(new Error('Each reference file must be 100 MiB or smaller.'))
  upload.mockImplementationOnce(async (file, _signal, onProgress) => {
    onProgress(file.size)
    const accepted = { ...validating, filename: file.name, status: 'processing' as const, processed_bytes: 0 }
    imports.push(accepted)
    return accepted
  })
  const user = userEvent.setup()
  render(page())
  const oversized = new File(['x'], 'large.txt')
  const valid = new File(['123,abc\n'], 'small.txt')
  await user.upload(screen.getByLabelText('Upload reference files'), [oversized, valid])
  await user.click(screen.getByRole('button', { name: 'Upload files' }))
  expect(await screen.findByText('1 file accepted. Imports continue on the collector.')).toBeTruthy()
  expect(screen.getByRole('alert').textContent).toContain('large.txt: Each reference file must be 100 MiB or smaller.')
  expect(upload.mock.calls.map(([file]) => file)).toEqual([oversized, valid])
  expect(await screen.findByRole('rowheader', { name: /small.txt/ })).toBeTruthy()
})

test('pages through the history from the URL using the response total', async () => {
  const fetchMock = vi.fn<typeof fetch>(async (input) => {
    const path = String(input)
    if (path === '/api/collector/status') return Response.json({ configured: true, reachable: true, authenticated: true, status: { available: true } })
    const url = new URL(path, 'http://tana.test')
    if (url.pathname !== '/api/collector/reference-imports') throw new Error(`Unexpected request: ${path}`)
    const second = url.searchParams.get('offset') === '10'
    return Response.json({ imports: [{ ...validating, id: second ? 'later' : 'newer', filename: second ? 'older.txt' : 'newer.txt' }], total: 25 })
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector/imports')
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('rowheader', { name: /newer.txt/ })
  expect(screen.getByRole('link', { name: 'Page 3' })).toBeTruthy()
  expect(screen.queryByRole('link', { name: 'Page 4' })).toBeNull()
  await user.click(screen.getByRole('link', { name: 'Page 2' }))
  await screen.findByRole('rowheader', { name: /older.txt/ })
  expect(window.location.hash).toBe('#/collector/imports?page=2')
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/reference-imports?limit=10&offset=10', expect.anything())
})
