// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'
import Downloads from './Downloads'
import type { LibraryDeliveryBatch, LibraryDeliveryItem } from './libraryDeliveries'

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

const item = (gallery_id: number, state: LibraryDeliveryItem['state']): LibraryDeliveryItem => ({ gallery_id, state, cleanup_attempts: 0 })
const batch = (state: LibraryDeliveryBatch['state'], items: LibraryDeliveryItem[]): LibraryDeliveryBatch => ({
  id: 1, library_id: 2, state, stop_requested: false, items,
})
const libraries = [{ id: 2, name: 'Manga', path: '/manga', availability: 'available', last_checked_at: null }]
const jobs = [7, 8].map((gallery_id) => ({
  gallery_id, state: 'completed', created_at: '2026-09-11T08:00:00Z', updated_at: '2026-09-11T09:00:00Z', failures: 0, size_bytes: 1024,
}))

function setup(initial: LibraryDeliveryBatch[] = []) {
  let batches = initial
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input)
    if (path === '/api/libraries') return Response.json(libraries)
    if (path.startsWith('/api/collector/downloads')) return Response.json({
      jobs: path.includes('state=queued') ? [] : jobs,
      counts: { queued: 0, running: 0, completed: 2, failed: 0, cancelled: 0, deleting: 0 },
    })
    if (path === '/api/library-deliveries' && init?.method === 'POST') {
      const body = JSON.parse(String(init.body))
      batches = [batch('running', (body.all ? [7, 8] : [body.gallery_id]).map((id: number) => item(id, 'queued')))]
      return Response.json(batches[0], { status: 202 })
    }
    if (path === '/api/library-deliveries') return Response.json({ batches })
    if (path.endsWith('/stop')) batches = [{ ...batches[0], state: batches[0].state === 'paused' ? 'stopped' : batches[0].state, stop_requested: true }]
    else if (path.endsWith('/resume') || path.endsWith('/retry')) batches = [{ ...batches[0], state: 'running', error: undefined }]
    else if (path.endsWith('/retry-cleanup')) batches = [{ ...batches[0], items: batches[0].items.map((value) => ({ ...value, cleanup_attempts: 0 })) }]
    else throw new Error(`Unexpected request ${path}`)
    return Response.json(batches[0])
  })
  vi.stubGlobal('fetch', fetchMock)
  return { fetchMock, update: (next: LibraryDeliveryBatch[]) => { batches = next } }
}

test('delivers one completed gallery to the selected library and blocks competing actions', async () => {
  const { fetchMock } = setup()
  const user = userEvent.setup()
  render(<Downloads available refreshKey={0} />)
  const row = within((await screen.findByRole('rowheader', { name: 'Gallery 7' })).closest('tr')!)
  expect((row.getByRole('button', { name: 'Add to library' }) as HTMLButtonElement).disabled).toBe(true)
  await user.selectOptions(screen.getByLabelText('Destination library'), '2')
  await user.click(row.getByRole('button', { name: 'Add to library' }))
  await screen.findByText('Delivery 1 · Manga')
  expect(fetchMock).toHaveBeenCalledWith('/api/library-deliveries', expect.objectContaining({ method: 'POST', body: '{"library_id":2,"gallery_id":7}' }))
  expect((screen.getByRole('button', { name: 'Add all' }) as HTMLButtonElement).disabled).toBe(true)
  expect((row.getByRole('button', { name: 'Delete' }) as HTMLButtonElement).disabled).toBe(true)
  await user.click(screen.getByRole('button', { name: 'Stop after current archive' }))
  await screen.findByText('Stopping after current archive')
  expect((screen.getByRole('button', { name: 'Stop after current archive' }) as HTMLButtonElement).disabled).toBe(true)
  expect(fetchMock).toHaveBeenCalledWith('/api/library-deliveries/1/stop', expect.objectContaining({ method: 'POST' }))
})

test('Add all requests the server snapshot independently of the visible filter', async () => {
  const { fetchMock } = setup()
  const user = userEvent.setup()
  render(<Downloads available refreshKey={0} />)
  await screen.findByRole('rowheader', { name: 'Gallery 7' })
  await user.click(screen.getByRole('button', { name: 'Pending 0' }))
  await screen.findByText('No pending downloads.')
  await user.selectOptions(screen.getByLabelText('Destination library'), '2')
  await user.click(screen.getByRole('button', { name: 'Add all' }))
  await screen.findByText('Delivery 1 · Manga')
  expect(fetchMock).toHaveBeenCalledWith('/api/library-deliveries', expect.objectContaining({ method: 'POST', body: '{"library_id":2,"all":true}' }))
  expect(screen.getByText(/0 of 2 processed/)).toBeTruthy()
})

test('restores paused progress while collector is disconnected and resumes only explicitly', async () => {
  const { fetchMock } = setup([{ ...batch('paused', [item(7, 'completed'), item(8, 'queued')]), error: 'The destination library is unavailable.' }])
  const user = userEvent.setup()
  render(<Downloads available={false} refreshKey={0} />)
  await screen.findByText('Paused')
  expect(screen.getByText(/1 of 2 processed · 1 added/)).toBeTruthy()
  expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
  expect((screen.getByRole('button', { name: 'Add all' }) as HTMLButtonElement).disabled).toBe(true)
  await user.click(screen.getByRole('button', { name: 'Resume' }))
  await screen.findByText('Delivering')
  expect(fetchMock).toHaveBeenCalledWith('/api/library-deliveries/1/resume', expect.objectContaining({ method: 'POST' }))
})

test('stops a paused batch even when stopping was already requested', async () => {
  const { fetchMock } = setup([{
    ...batch('paused', [item(7, 'saved')]), stop_requested: true, error: 'The destination library is unavailable.',
  }])
  const user = userEvent.setup()
  render(<Downloads available refreshKey={0} />)
  await user.click(await screen.findByRole('button', { name: 'Stop batch' }))
  await screen.findByText('Stopped')
  expect(fetchMock).toHaveBeenCalledWith('/api/library-deliveries/1/stop', expect.objectContaining({ method: 'POST' }))
  expect(screen.queryByText('One delivery batch at a time. Finish or stop the active batch to start another.')).toBeNull()
})

test.each<LibraryDeliveryItem['state']>(['queued', 'transferring', 'transferred', 'saved', 'importing', 'failed'])(
  'retries a stopped batch whose last archive is %s', async (state) => {
    const { fetchMock } = setup([batch('stopped', [item(7, 'completed'), item(8, state)])])
    const user = userEvent.setup()
    render(<Downloads available refreshKey={0} />)
    await user.click(await screen.findByRole('button', { name: 'Retry remaining' }))
    await screen.findByText('Delivering')
    expect(fetchMock).toHaveBeenCalledWith('/api/library-deliveries/1/retry', expect.objectContaining({ method: 'POST' }))
  },
)

test('shows skipped and failed galleries and retries cleanup separately from delivery', async () => {
  const { fetchMock } = setup([batch('completed_with_errors', [
    item(7, 'skipped'), { ...item(8, 'failed'), error: 'A destination file already exists.' },
    { ...item(9, 'cleanup_pending'), error: 'Collector deletion failed.', cleanup_attempts: 3 },
  ])])
  const user = userEvent.setup()
  render(<Downloads available refreshKey={0} />)
  await screen.findByText('Needs attention')
  await user.click(screen.getByText('Gallery results (3)'))
  expect(screen.getByText('Gallery 7 · Already in library')).toBeTruthy()
  expect(screen.getByText('A destination file already exists.')).toBeTruthy()
  expect(screen.getByText('Gallery 9 · Imported · collector cleanup pending')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Retry cleanup' }))
  await waitFor(() => expect(screen.queryByRole('button', { name: 'Retry cleanup' })).toBeNull())
  expect(fetchMock).toHaveBeenCalledWith('/api/library-deliveries/1/retry-cleanup', expect.objectContaining({ method: 'POST' }))
  expect(fetchMock.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(1)
  await user.click(screen.getByRole('button', { name: 'Retry failed' }))
  await screen.findByText('Delivering')
  expect(fetchMock).toHaveBeenCalledWith('/api/library-deliveries/1/retry', expect.objectContaining({ method: 'POST' }))
})
