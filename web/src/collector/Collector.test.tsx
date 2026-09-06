// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'
import App from '../App'
import type { ConnectionStatus } from './api'

afterEach(() => { cleanup(); vi.unstubAllGlobals(); window.history.replaceState(null, '', '/') })

function connected(): ConnectionStatus {
  return {
    configured: true, reachable: true, authenticated: true, checked_at: '2026-09-06T10:00:00Z',
    status: {
      favorites: {
        host: 'https://panda.test', account_key: 'current',
        categories: Array.from({ length: 10 }, (_, category) => ({
          category, name: category === 2 ? 'Manga' : '', favorites: category === 2 ? 42 : 0,
          state: 'idle', full: false, queued: false, queued_full: false,
          entries_saved: 0, pages_saved: 0,
          last_synced_at: category === 2 ? '2026-09-06T09:00:00Z' : undefined,
        })),
      },
      inventory: { gallery_references: 150, metadata_available: 100, metadata_pending: 45, metadata_failed: 5, fetches_pending: 3, fetches_failed: 0 },
      metadata_errors: [{ gallery_id: 123, error: 'Gallery unavailable', at: '2026-09-06T09:00:00Z' }],
    },
  }
}

test('Collector route shows statistics and submits all-category sync and a selected full re-sync', async () => {
  const fetchMock = vi.fn<typeof fetch>(async (_input, init) => init?.method === 'POST'
    ? Response.json({ status: 'accepted' }, { status: 202 }) : Response.json(connected()))
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector')
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('Connected')
  expect(screen.getByRole('link', { name: 'Collector' }).getAttribute('aria-current')).toBe('page')
  expect(screen.getByText('150')).toBeTruthy()
  expect(screen.getByText('42 collected favorites')).toBeTruthy()
  expect(screen.getByText('Gallery unavailable')).toBeTruthy()
  expect(within(screen.getByRole('table')).getAllByRole('row')).toHaveLength(11)
  await user.click(screen.getByRole('button', { name: 'Sync favorites' }))
  expect(await screen.findByText('Sync requested for all categories.')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/all/sync', expect.objectContaining({ method: 'POST', body: '{"full":false}' }))
  await user.selectOptions(screen.getByLabelText('Favorite category'), '2')
  await user.selectOptions(screen.getByLabelText('Sync mode'), 'full')
  await user.click(screen.getByRole('button', { name: 'Full re-sync favorites' }))
  expect(await screen.findByText('Full re-sync requested for category 2.')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/favorites/2/sync', expect.objectContaining({ method: 'POST', body: '{"full":true}' }))
})

test('preserves statistics through connection failures, disables syncing, and recovers on refresh', async () => {
  let result = connected()
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async () => Response.json(result)))
  window.history.replaceState(null, '', '/#/collector')
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('Connected')
  result = { configured: true, reachable: false, authenticated: null, checked_at: '2026-09-06T10:00:05Z', error: 'collector_unreachable' }
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(await screen.findByText(/Statistics are stale/)).toBeTruthy()
  expect(screen.getByText('150')).toBeTruthy()
  expect((screen.getByRole('button', { name: 'Sync favorites' }) as HTMLButtonElement).disabled).toBe(true)
  expect(screen.getByText('Unreachable')).toBeTruthy()
  result = connected()
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() => expect(screen.queryByText(/Statistics are stale/)).toBeNull())
  expect((screen.getByRole('button', { name: 'Sync favorites' }) as HTMLButtonElement).disabled).toBe(false)
})

test('shows durable progress and collected favorites before the first sync completes', async () => {
  const result = connected()
  Object.assign(result.status!.favorites.categories[2], {
    state: 'running', favorites: 2000, entries_saved: 2000, pages_saved: 20,
    started_at: '2026-09-06T09:00:00Z', last_saved_at: '2026-09-06T09:59:00Z', last_synced_at: undefined,
  })
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async () => Response.json(result)))
  window.history.replaceState(null, '', '/#/collector')
  render(<App />)
  await screen.findByText('Connected')
  const row = screen.getByRole('rowheader', { name: /Manga/ }).closest('tr')!
  expect(within(row).getByText('Running sync')).toBeTruthy()
  expect(within(row).getByText('2,000 entries saved · 20 pages')).toBeTruthy()
  expect(within(row).getByText('2,000')).toBeTruthy()
  expect(within(row).getByText(/Last save/)).toBeTruthy()
  expect(within(row).getByText('Never synced')).toBeTruthy()
})
