// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'
import Downloads from './Downloads'
import type { DownloadJob } from './downloads'

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

const job = (id: number, state: DownloadJob['state']): DownloadJob => ({
  gallery_id: id, state, created_at: '2026-09-11T08:00:00Z', updated_at: '2026-09-11T09:00:00Z', failures: 0, size_bytes: 0,
})
const listing = (jobs: DownloadJob[], page = jobs) => {
  const counts = { queued: 0, running: 0, completed: 0, failed: 0, cancelled: 0, deleting: 0 }
  for (const item of jobs) counts[item.state]++
  return { jobs: page, counts }
}
const row = (id: number) => within(screen.getByRole('rowheader', { name: `Gallery ${id}` }).closest('tr')!)
const props = { available: true, refreshKey: 0 }

test('submits a URL, reuses an existing job, cancels and explicitly retries it', async () => {
  let jobs: DownloadJob[] = []
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    if (init?.method === 'POST') {
      const path = String(input)
      jobs = [job(42, path.endsWith('/cancel') ? 'cancelled' : 'queued')]
      return Response.json(jobs[0], { status: path.endsWith('/downloads') ? 202 : 200 })
    }
    return Response.json(listing(jobs))
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(<Downloads {...props} />)
  await screen.findByText(/No downloads yet/)
  const input = screen.getByLabelText('Panda gallery URL') as HTMLInputElement
  for (let i = 0; i < 2; i++) {
    await user.type(input, 'https://panda.test/g/42/gallery-token/?p=1')
    await user.click(screen.getByRole('button', { name: 'Add download' }))
    await screen.findByText('Gallery 42: Queued.')
    expect(input.value).toBe('')
    await waitFor(() => expect(row(42).getByText('Queued')).toBeTruthy())
  }
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/downloads', expect.objectContaining({ method: 'POST', body: '{"gid":42,"token":"gallery-token"}' }))
  expect(screen.getAllByRole('rowheader')).toHaveLength(1)
  await user.click(row(42).getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(row(42).getByText('Cancelled')).toBeTruthy())
  await user.click(row(42).getByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(row(42).getByText('Queued')).toBeTruthy())
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/downloads/42/retry', expect.objectContaining({ method: 'POST' }))
})

test('shows retry diagnostics, offers retained ZIP retrieval, and deletes only after confirmation', async () => {
  let jobs = [
    { ...job(7, 'completed'), size_bytes: 2097152 },
    { ...job(8, 'queued'), failures: 2, error: 'transfer_failed', retry_at: '2026-09-11T09:05:00Z' },
    { ...job(9, 'failed'), failures: 5, error: 'invalid_zip' },
  ]
  const fetchMock = vi.fn<typeof fetch>(async (_input, init) => {
    if (init?.method === 'DELETE') {
      jobs = jobs.filter((item) => item.gallery_id !== 7)
      return new Response(null, { status: 204 })
    }
    return Response.json(listing(jobs))
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(<Downloads {...props} />)
  await screen.findByText('Available on collector')
  expect(row(7).getByText('2.0 MiB')).toBeTruthy()
  expect(row(7).getByRole('link', { name: 'Save ZIP' }).getAttribute('href')).toBe('/api/collector/downloads/7/file')
  expect(row(8).getByText('Waiting to retry')).toBeTruthy()
  expect(row(8).getByText('2 failed attempts')).toBeTruthy()
  expect(row(8).getByText(/Retry after/)).toBeTruthy()
  expect(row(9).getByText('The downloaded file was not a valid ZIP archive.')).toBeTruthy()
  expect(row(9).getByRole('button', { name: 'Retry' })).toBeTruthy()
  await user.click(row(7).getByRole('button', { name: 'Delete' }))
  expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'DELETE')).toBe(false)
  await user.click(row(7).getByRole('button', { name: 'Keep download' }))
  await user.click(row(7).getByRole('button', { name: 'Delete' }))
  await user.click(row(7).getByRole('button', { name: 'Delete download' }))
  await waitFor(() => expect(screen.queryByRole('rowheader', { name: 'Gallery 7' })).toBeNull())
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/downloads/7', expect.objectContaining({ method: 'DELETE' }))
})

test('paginates and returns to the preceding page after deleting the last job', async () => {
  let jobs = Array.from({ length: 26 }, (_, i) => job(26 - i, 'cancelled'))
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input, init) => {
    if (init?.method === 'DELETE') {
      jobs = jobs.filter((item) => item.gallery_id !== 1)
      return new Response(null, { status: 204 })
    }
    const url = new URL(String(input), 'http://tana.test')
    const offset = Number(url.searchParams.get('offset'))
    return Response.json(listing(jobs, jobs.slice(offset, offset + 26)))
  }))
  const user = userEvent.setup()
  render(<Downloads {...props} />)
  await screen.findByRole('rowheader', { name: 'Gallery 26' })
  expect(screen.getAllByRole('rowheader')).toHaveLength(25)
  await user.click(screen.getByRole('button', { name: 'Next downloads' }))
  await screen.findByRole('rowheader', { name: 'Gallery 1' })
  expect(screen.getAllByRole('rowheader')).toHaveLength(1)
  await user.click(row(1).getByRole('button', { name: 'Delete' }))
  await user.click(row(1).getByRole('button', { name: 'Delete download' }))
  await screen.findByRole('rowheader', { name: 'Gallery 26' })
  expect(screen.queryByRole('navigation', { name: 'Download pages' })).toBeNull()
})

test('filters all downloads by state, resets pagination, and updates global counts after cancellation', async () => {
  let jobs = [
    ...Array.from({ length: 25 }, (_, i) => job(28 - i, 'completed')),
    job(3, 'queued'), job(2, 'queued'), job(1, 'running'),
  ]
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    if (init?.method === 'POST') {
      jobs = jobs.map((item) => item.gallery_id === 1 ? { ...item, state: 'cancelled' } : item)
      return Response.json(jobs.find((item) => item.gallery_id === 1))
    }
    const url = new URL(String(input), 'http://tana.test')
    const offset = Number(url.searchParams.get('offset'))
    const state = url.searchParams.get('state')
    const filtered = state ? jobs.filter((item) => item.state === state) : jobs
    return Response.json(listing(jobs, filtered.slice(offset, offset + 26)))
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(<Downloads {...props} />)
  await screen.findByRole('rowheader', { name: 'Gallery 28' })
  const filters = within(screen.getByRole('group', { name: 'Filter downloads' }))
  expect(filters.getByRole('button', { name: 'All 28' }).getAttribute('aria-pressed')).toBe('true')
  expect(filters.getByRole('button', { name: 'Active 1' }).getAttribute('aria-pressed')).toBe('false')
  expect(screen.queryByRole('rowheader', { name: 'Gallery 1' })).toBeNull()

  await user.click(screen.getByRole('button', { name: 'Next downloads' }))
  await screen.findByRole('rowheader', { name: 'Gallery 1' })
  expect(screen.getByText('Page 2')).toBeTruthy()
  expect(filters.getByRole('button', { name: 'All 28' })).toBeTruthy()
  await user.click(filters.getByRole('button', { name: 'Active 1' }))
  await waitFor(() => expect(screen.getAllByRole('rowheader')).toHaveLength(1))
  expect(row(1).getByText('Running')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/downloads?limit=26&offset=0&state=running', expect.any(Object))
  expect(screen.queryByRole('navigation', { name: 'Download pages' })).toBeNull()
  expect(filters.getByRole('button', { name: 'Active 1' }).getAttribute('aria-pressed')).toBe('true')
  expect(filters.getByRole('button', { name: 'All 28' }).getAttribute('aria-pressed')).toBe('false')
  expect(filters.getByRole('button', { name: 'Completed 25' })).toBeTruthy()
  expect(filters.getByRole('button', { name: 'Pending 2' })).toBeTruthy()

  await user.click(row(1).getByRole('button', { name: 'Cancel' }))
  await screen.findByText('No active downloads.')
  expect(filters.getByRole('button', { name: 'Active 0' }).getAttribute('aria-pressed')).toBe('true')
  expect(filters.getByRole('button', { name: 'Cancelled 1' })).toBeTruthy()
  expect(filters.getByRole('button', { name: 'All 28' })).toBeTruthy()
  expect(screen.queryByRole('rowheader')).toBeNull()

  await user.click(filters.getByRole('button', { name: 'Pending 2' }))
  await screen.findByRole('rowheader', { name: 'Gallery 3' })
  expect(screen.getAllByRole('rowheader')).toHaveLength(2)
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/downloads?limit=26&offset=0&state=queued', expect.any(Object))
})

test('retains input on submission failure and retains jobs through collector failures', async () => {
  let offline = false
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (_input, init) => {
    if (init?.method === 'POST') return Response.json({ error: 'download_token_conflict' }, { status: 409 })
    if (offline) return Response.json({ error: 'collector_unauthorized' }, { status: 502 })
    return Response.json(listing([job(7, 'completed')]))
  }))
  const user = userEvent.setup()
  const view = render(<Downloads {...props} />)
  await screen.findByText('Available on collector')
  const input = screen.getByLabelText('Panda gallery URL') as HTMLInputElement
  await user.type(input, 'https://panda.test/g/7/corrected/')
  await user.click(screen.getByRole('button', { name: 'Add download' }))
  await screen.findByText(/different token/)
  expect(input.value).toBe('https://panda.test/g/7/corrected/')
  offline = true
  await user.click(screen.getByRole('button', { name: 'Refresh downloads' }))
  await screen.findByText(/Download status is stale/)
  expect(row(7).getByText('Available on collector')).toBeTruthy()
  expect((row(7).getByRole('button', { name: 'Delete' }) as HTMLButtonElement).disabled).toBe(true)
  view.rerender(<Downloads {...props} available={false} />)
  expect(input.disabled).toBe(true)
  offline = false
  view.rerender(<Downloads {...props} refreshKey={1} />)
  await waitFor(() => expect(screen.queryByText(/Download status is stale/)).toBeNull())
  expect(row(7).getByRole('link', { name: 'Save ZIP' })).toBeTruthy()
})
