// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import Libraries from '../libraries/Libraries'
import type { ScanStatus } from './useScan'

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

test('starts per-library and global scans; recovers active status after navigation and shows failures', async () => {
  let status: ScanStatus = { phase: 'idle', libraries_total: 1, discovered: 0, imported: 0, failed_sources: 0, skipped: 0, discovery_errors: 0, galleries_created: 0, started_at: null, finished_at: null }
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    if (String(input) === '/api/libraries') return Response.json([{ id: 1, name: 'Manga', path: '/manga', availability: 'available', last_checked_at: null }])
    if (init?.method === 'POST') {
      status = { ...status, phase: 'discovering', library_id: JSON.parse(init.body as string).library_id, started_at: '2026-09-05T10:00:00Z' }
      return Response.json({ status: 'accepted' }, { status: 202 })
    }
    return Response.json(status)
  })
  vi.stubGlobal('fetch', fetchMock)
  const first = render(<Libraries />)
  const scan = await screen.findByRole('button', { name: 'Scan' })
  await waitFor(() => expect((scan as HTMLButtonElement).disabled).toBe(false))
  fireEvent.click(scan)
  await screen.findByText('Discovering sources…')
  expect(fetchMock).toHaveBeenCalledWith('/api/scans', expect.objectContaining({ method: 'POST', body: '{"library_id":1}' }))
  expect((screen.getByRole('button', { name: 'Scan all' }) as HTMLButtonElement).disabled).toBe(true)
  first.unmount()
  render(<Libraries />)
  await screen.findByText('Discovering sources…')
  status = { ...status, phase: 'completed_with_errors', discovered: 3, imported: 2, galleries_created: 2, failed_sources: 1, finished_at: '2026-09-05T10:01:00Z' }
  await screen.findByText('Scan complete with errors', {}, { timeout: 2500 })
  expect(within(screen.getByLabelText('Library scanning')).getByText('Failed sources').nextElementSibling?.textContent).toBe('1')
  fireEvent.click(screen.getByRole('button', { name: 'Scan all' }))
  await screen.findByText('Discovering sources…')
  expect(fetchMock).toHaveBeenCalledWith('/api/scans', expect.objectContaining({ method: 'POST', body: '{}' }))
})

test('recovers the running scan after another client wins a start race', async () => {
  let active = false
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input, init) => {
    if (String(input) === '/api/libraries') return Response.json([{ id: 1, name: 'Manga', path: '/manga', availability: 'available', last_checked_at: null }])
    if (init?.method === 'POST') { active = true; return Response.json({ error: 'scan_active' }, { status: 409 }) }
    return Response.json({ phase: active ? 'discovering' : 'idle', library_id: undefined, discovered: 0, imported: 0, galleries_created: 0, failed_sources: 0, discovery_errors: 0, skipped: 0 })
  }))
  render(<Libraries />)
  const scan = await screen.findByRole('button', { name: 'Scan' })
  await waitFor(() => expect((scan as HTMLButtonElement).disabled).toBe(false))
  fireEvent.click(scan)
  expect((await screen.findByRole('alert')).textContent).toContain('already running')
  await screen.findByText('Discovering sources…')
  expect((scan as HTMLButtonElement).disabled).toBe(true)
})
