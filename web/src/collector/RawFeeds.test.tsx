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

test('raw feeds route exposes capture errors and original downloads across filters and pages', async () => {
  const fetchMock = vi.fn<typeof fetch>(async (input) => {
    const path = String(input)
    if (path === '/api/collector/status') return Response.json({
      configured: true, reachable: true, authenticated: true, checked_at: '2026-09-13T10:00:00Z', status: { available: true },
    })
    const url = new URL(path, 'http://tana.test')
    if (url.pathname !== '/api/collector/feed/captures') throw new Error(`Unexpected request: ${path}`)
    if (url.searchParams.get('failed_only') === 'true') return Response.json({ captures: [failed], has_more: false })
    const first = url.searchParams.get('offset') === '0'
    return Response.json({ captures: first ? [pending, failed] : [{ ...pending, id: 314 }], has_more: first })
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector/feeds')
  const user = userEvent.setup()
  render(<App />)

  const row = (await screen.findByRole('rowheader', { name: 'Capture 339' })).closest('tr')!
  expect(within(screen.getByRole('navigation', { name: 'Collector navigation' })).getByRole('link', { name: 'Raw feeds' }).getAttribute('aria-current')).toBe('page')
  expect(within(row).getByText(failed.error!)).toBeTruthy()
  const download = within(row).getByRole('link', { name: 'Download raw feed 339' })
  expect(download.getAttribute('href')).toBe('/api/collector/feed/captures/339/file')
  expect(download.hasAttribute('download')).toBe(true)
  expect(screen.getByRole('link', { name: 'Download raw feed 340' })).toBeTruthy()

  await user.click(screen.getByRole('button', { name: 'Next captures' }))
  await screen.findByRole('rowheader', { name: 'Capture 314' })
  expect(screen.getByText('Page 2')).toBeTruthy()
  await user.selectOptions(screen.getByLabelText('Show captures'), 'failed')
  await screen.findByRole('rowheader', { name: 'Capture 339' })
  expect(screen.queryByRole('rowheader', { name: 'Capture 314' })).toBeNull()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/feed/captures?failed_only=true&limit=25&offset=0', expect.anything())
  expect(screen.queryByRole('button', { name: 'Next captures' })).toBeNull()
})
