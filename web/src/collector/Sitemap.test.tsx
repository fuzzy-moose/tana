// @vitest-environment jsdom
import { useState } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'
import Sitemap from './Sitemap'
import type { SitemapStatus } from './api'

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

const idle: SitemapStatus = {
  state: 'idle', force: false, children_total: 0, children_completed: 0, children_skipped: 0, children_failed: 0,
  references_found: 0, references_imported: 0, invalid_locations: 0,
}

function Collection({ initial = idle, available = true }: { initial?: SitemapStatus, available?: boolean }) {
  const [status, setStatus] = useState(initial)
  return <Sitemap status={status} available={available} onChanged={setStatus} />
}

test.each([false, true])('manual sitemap collection sends force=%s and shows reference import progress separately from metadata', async (force) => {
  const fetchMock = vi.fn<typeof fetch>(async () => Response.json({
    ...idle, state: 'running', force, children_total: 5, children_completed: 1, children_skipped: 2,
    references_found: 120, references_imported: 100, invalid_locations: 3,
  }, { status: 202 }))
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(<Collection />)
  if (force) await user.selectOptions(screen.getByLabelText('Collection mode'), 'force')
  await user.click(screen.getByRole('button', { name: 'Collect sitemap' }))
  expect(await screen.findByText('Sitemap collection requested.')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/sitemap/start', expect.objectContaining({ method: 'POST', body: JSON.stringify({ force }) }))
  expect(screen.getByText('3 of 5 child sitemaps processed')).toBeTruthy()
  const progress = screen.getByRole('progressbar', { name: 'Sitemap collection progress' }) as HTMLProgressElement
  expect(progress.value).toBe(3)
  expect(progress.max).toBe(5)
  expect(screen.getByText('120')).toBeTruthy()
  expect(screen.getByText('100')).toBeTruthy()
  expect(screen.getByText(/Metadata fetching continues independently/)).toBeTruthy()
  expect((screen.getByRole('button', { name: 'Collect sitemap' }) as HTMLButtonElement).disabled).toBe(true)
})

test('cancels active work, resumes cancelled work, and retries incomplete work', async () => {
  const initial: SitemapStatus = {
    ...idle, state: 'running', children_total: 4, children_completed: 1,
    references_found: 100, references_imported: 90,
  }
  const fetchMock = vi.fn<typeof fetch>(async (input) => Response.json({
    ...initial, state: String(input).endsWith('/cancel') ? 'cancelled' : 'running',
  }, { status: 202 }))
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(<Collection initial={initial} />)
  await user.click(screen.getByRole('button', { name: 'Cancel sitemap collection' }))
  expect(await screen.findByText('Cancelled')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/sitemap/cancel', expect.objectContaining({ method: 'POST', body: '{}' }))
  expect(screen.getByText('90')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Resume sitemap collection' }))
  expect(await screen.findByText('Sitemap retry requested.')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/sitemap/retry', expect.objectContaining({ method: 'POST', body: '{}' }))
  cleanup()
  render(<Collection initial={{ ...initial, state: 'incomplete', children_failed: 1, last_error: 'invalid_sitemap' }} />)
  expect(screen.getByText('Sitemap collection: A sitemap could not be parsed.')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Retry sitemap collection' }))
  expect(await screen.findByText('Sitemap retry requested.')).toBeTruthy()
})
