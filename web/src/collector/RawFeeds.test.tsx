// @vitest-environment jsdom
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'
import App from '../App'
import type { FeedCapture } from './api'

afterEach(() => { cleanup(); vi.unstubAllGlobals(); window.history.replaceState(null, '', '/') })

const failed: FeedCapture = {
  id: 339, captured_at: '2026-09-13T09:41:26Z', size_bytes: 45000, state: 'failed',
  error: 'panda: parse feed: XML syntax error on line 938: illegal character code U+001D',
}
const pending: FeedCapture = { ...failed, id: 340, state: 'pending', error: undefined }
const feedStatus = { capture_active: false, processing_pending: 1, continuity: 'unknown', possible_gaps: 0 }

test('raw feeds route exposes capture errors and original downloads across filters and pages', async () => {
  const fetchMock = vi.fn<typeof fetch>(async (input) => {
    const path = String(input)
    if (path === '/api/collector/status') return Response.json({
      configured: true, reachable: true, authenticated: true, checked_at: '2026-09-13T10:00:00Z', status: { available: true },
    })
    if (path === '/api/collector/feed/status') return Response.json(feedStatus)
    const url = new URL(path, 'http://tana.test')
    if (url.pathname !== '/api/collector/feed/captures') throw new Error(`Unexpected request: ${path}`)
    if (url.searchParams.get('failed_only') === 'true') return Response.json({ captures: [failed], total: 1 })
    const first = url.searchParams.get('offset') === '0'
    return Response.json({ captures: first ? [pending, failed] : [{ ...pending, id: 314 }], total: 30 })
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector/feeds')
  const user = userEvent.setup()
  render(<App />)

  const row = (await screen.findByRole('rowheader', { name: '339' })).closest('tr')!
  expect(within(screen.getByRole('navigation', { name: 'Collector navigation' })).getByRole('link', { name: 'Raw feeds' }).getAttribute('aria-current')).toBe('page')
  expect(within(row).getByText(failed.error!)).toBeTruthy()
  const download = within(row).getByRole('link', { name: 'Download raw feed 339' })
  expect(download.getAttribute('href')).toBe('/api/collector/feed/captures/339/file')
  expect(download.hasAttribute('download')).toBe(true)
  expect(screen.getByRole('link', { name: 'Download raw feed 340' })).toBeTruthy()
  expect(within(row).getByText('43.9 KiB')).toBeTruthy()

  await user.click(screen.getByRole('link', { name: 'Page 2' }))
  await screen.findByRole('rowheader', { name: '314' })
  expect(window.location.hash).toBe('#/collector/feeds?page=2')
  expect(screen.getByLabelText('Page 2').getAttribute('aria-current')).toBe('page')
  await user.click(screen.getByRole('link', { name: /^Failed/ }))
  await screen.findByRole('rowheader', { name: '339' })
  expect(screen.queryByRole('rowheader', { name: '314' })).toBeNull()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/feed/captures?failed_only=true&limit=25&offset=0', expect.anything())
  expect(window.location.hash).toBe('#/collector/feeds?state=failed')
  expect(screen.queryByRole('navigation', { name: 'Feed capture pages' })).toBeNull()
})

test('captures a feed and refreshes retained captures independently of inventory', async () => {
  let captures: FeedCapture[] = []
  let captured = false
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input)
    if (path === '/api/collector/status') return Response.json({ configured: true, reachable: true, authenticated: true, status: { available: true } })
    if (path === '/api/collector/feed/refresh' && init?.method === 'POST') {
      captures = [pending]
      captured = true
      return Response.json({ ...feedStatus, last_captured_at: pending.captured_at })
    }
    if (path === '/api/collector/feed/status') return Response.json({ ...feedStatus, last_captured_at: captured ? pending.captured_at : undefined })
    if (path.startsWith('/api/collector/feed/captures?')) return Response.json({ captures, total: captures.length })
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector/feeds')
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('No feed captures.')
  await user.click(screen.getByRole('button', { name: 'Refresh feed' }))
  expect(await screen.findByRole('rowheader', { name: '340' })).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/feed/refresh', expect.objectContaining({ method: 'POST', body: '{}' }))
  expect(screen.getByText(/Last captured/)).toBeTruthy()
  expect(fetchMock.mock.calls.some(([input]) => String(input) === '/api/collector/inventory/status')).toBe(false)
  expect(screen.queryByRole('alert')).toBeNull()
})

test('shows the feed processing error', async () => {
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input) => {
    const path = String(input)
    if (path === '/api/collector/status') return Response.json({ configured: true, reachable: true, authenticated: true, status: { available: true } })
    if (path === '/api/collector/feed/status') return Response.json({ ...feedStatus, possible_gaps: 1, processing_error: 'capture 340 contains malformed XML' })
    if (path.startsWith('/api/collector/feed/captures?')) return Response.json({ captures: [], total: 0 })
    throw new Error(`Unexpected request: ${path}`)
  }))
  window.history.replaceState(null, '', '/#/collector/feeds')
  render(<App />)
  const stopped = await screen.findByText('Feed processing stopped')
  expect(stopped.closest('[role="status"]')!.textContent).toContain('capture 340 contains malformed XML')
  expect(screen.getByText('1 possible feed gap')).toBeTruthy()
  expect(screen.getByText('1 feed awaiting processing')).toBeTruthy()
})
