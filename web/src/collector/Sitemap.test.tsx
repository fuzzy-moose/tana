// @vitest-environment jsdom
import { cleanup, render as testingRender, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Theme } from '@radix-ui/themes'
import type { ReactNode } from 'react'

import { afterEach, expect, test, vi } from 'vitest'
import Sitemap from './Sitemap'
import type { SitemapStatus } from './api'

const render = (ui: ReactNode) => testingRender(ui, { wrapper: Theme })

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

const idle: SitemapStatus = {
  state: 'idle', force: false, children_total: 0, children_completed: 0, children_skipped: 0, children_failed: 0,
  references_found: 0, references_imported: 0, invalid_locations: 0,
}

// Serves the status endpoint with `initial` until a POST replaces it with what `next` returns.
function stubCollector(initial: SitemapStatus, next: (action: string) => SitemapStatus) {
  let current = initial
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input)
    if (init?.method === 'POST') { current = next(path.split('/').pop()!); return Response.json(current, { status: 202 }) }
    return Response.json(current)
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

const page = <Sitemap available refreshKey={0} interval={60000} />

test.each([false, true])('manual sitemap collection sends force=%s and shows reference import progress separately from metadata', async (force) => {
  const fetchMock = stubCollector(idle, () => ({
    ...idle, state: 'running', force, children_total: 5, children_completed: 1, children_skipped: 2,
    references_found: 120, references_imported: 100, invalid_locations: 3,
  }))
  const user = userEvent.setup()
  render(page)
  if (force) {
    await user.click(await screen.findByRole('combobox', { name: 'Start a collection' }))
    await user.click(screen.getByRole('option', { name: 'Force refresh all sitemaps' }))
  }
  await user.click(await screen.findByRole('button', { name: 'Collect sitemap' }))
  expect(await screen.findByText('Sitemap collection requested.')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/sitemap/start', expect.objectContaining({ method: 'POST', body: JSON.stringify({ force }) }))
  expect(screen.getByText(/3 of 5 child sitemaps processed/)).toBeTruthy()
  expect(screen.getByText('60%')).toBeTruthy()
  const progress = screen.getByRole('progressbar', { name: 'Sitemap collection progress' })
  expect(progress.getAttribute('aria-valuenow')).toBe('3')
  expect(progress.getAttribute('aria-valuemax')).toBe('5')
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
  const fetchMock = stubCollector(initial, (action) => ({ ...initial, state: action === 'cancel' ? 'cancelled' : 'running' }))
  const user = userEvent.setup()
  render(page)
  await user.click(await screen.findByRole('button', { name: 'Cancel collection' }))
  expect(await screen.findByText('Cancelled')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/sitemap/cancel', expect.objectContaining({ method: 'POST', body: '{}' }))
  expect(screen.getByText('90')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Resume collection' }))
  expect(await screen.findByText('Sitemap retry requested.')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/sitemap/retry', expect.objectContaining({ method: 'POST', body: '{}' }))
  cleanup()
  stubCollector({ ...initial, state: 'incomplete', children_failed: 1, last_error: 'invalid_sitemap' }, () => initial)
  render(page)
  expect((await screen.findByRole('status')).textContent).toContain('A sitemap could not be parsed.')
  await user.click(screen.getByRole('button', { name: 'Retry failed' }))
  expect(await screen.findByText('Sitemap retry requested.')).toBeTruthy()
})
