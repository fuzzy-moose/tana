// @vitest-environment jsdom
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import App from '../App'
import SourceCleanup from './SourceCleanup'
import type { CleanupPlan, CleanupSource } from './cleanupApi'

vi.mock('../scans/useScan', () => ({ useScan: () => ({ status: null, error: '', disabled: true, start: vi.fn() }) }))

const source = (id: number): CleanupSource => ({
  id, library_id: 1, library_name: 'Manga', path: `/mnt/manga/Volume [${id}].cbz`, panda_id: id, title: `Volume ${id}`,
})
const plan: CleanupPlan = {
  plan_id: 'preview-one',
  candidates: [1, 2, 3].map((id) => ({
    source: source(id),
    replacement: { ...source(id + 10), library_id: 2, library_name: 'New Manga', path: `/nas/new/Volume [${id + 10}].zip` },
    size_bytes: 1024 * 2 ** (id - 1),
  })),
  skipped: [{ source_id: 4, path: '/mnt/manga/Unknown [4].cbz', reason: 'Required Panda metadata has not been collected.' }],
}

const fetchMock = vi.fn<typeof fetch>()

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

test('cleans up across libraries with preselection, exclusions, one confirmation, and per-source results', async () => {
  window.history.replaceState(null, '', '/#/libraries')
  fetchMock.mockResolvedValueOnce(Response.json([{ id: 1, name: 'Manga', path: '/mnt/manga', availability: 'available', last_checked_at: null }]))
    .mockResolvedValueOnce(Response.json(plan))
    .mockResolvedValueOnce(Response.json({ deleted: [1], failed: [{ source_id: 3, reason: 'The newer source is no longer present.' }] }))
    .mockResolvedValueOnce(Response.json({ plan_id: 'preview-two', candidates: [], skipped: [] }))
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('heading', { name: 'Manga' })
  await user.click(screen.getByRole('link', { name: 'Clean up older versions' }))
  expect(await screen.findByText('3 archives selected · 7.0 KiB')).toBeTruthy()
  expect(fetchMock).toHaveBeenLastCalledWith('/api/source-cleanup', expect.objectContaining({ signal: expect.any(AbortSignal) }))
  expect(screen.getAllByRole('checkbox', { checked: true })).toHaveLength(3)
  expect(screen.getByText('New Manga · Panda 11')).toBeTruthy()
  expect(screen.getByText('/nas/new/Volume [11].zip')).toBeTruthy()
  await user.click(screen.getByText('1 source skipped'))
  expect(screen.getByText(plan.skipped[0].reason)).toBeTruthy()

  await user.click(screen.getByRole('checkbox', { name: `Select ${source(2).path}` }))
  expect(screen.getByText('2 archives selected · 5.0 KiB')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Delete selected…' }))
  const confirmation = screen.getByRole('group', { name: 'Confirm permanent deletion' })
  expect(within(confirmation).getByText(/Files will not go to trash/).textContent).toContain('5.0 KiB')
  const deleting = screen.getByRole('list', { name: 'Archives to permanently delete' })
  expect(within(deleting).getByText(source(1).path)).toBeTruthy()
  expect(within(deleting).queryByText(source(2).path)).toBeNull()
  expect(within(deleting).getByText('/nas/new/Volume [13].zip')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledTimes(2)

  await user.click(within(confirmation).getByRole('button', { name: 'Permanently delete 2 archives' }))
  expect(await screen.findByText('1 archive deleted. 1 could not be deleted.')).toBeTruthy()
  expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/source-cleanup', expect.objectContaining({
    method: 'POST', body: JSON.stringify({ plan_id: 'preview-one', source_ids: [1, 3] }),
  }))
  expect(screen.getByText('Permanently deleted')).toBeTruthy()
  expect(screen.getByText('Could not delete: The newer source is no longer present.')).toBeTruthy()
  expect(screen.getByText('Kept — excluded from cleanup')).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Delete selected…' })).toBeNull()
  expect(screen.getAllByRole('checkbox').every((input) => input.hasAttribute('disabled'))).toBe(true)

  await user.click(screen.getByRole('button', { name: 'Refresh preview' }))
  expect(await screen.findByText('No eligible older archives found.')).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledTimes(4)
})

test('requires a fresh preview after an uncertain deletion response', async () => {
  fetchMock.mockResolvedValueOnce(Response.json(plan))
    .mockRejectedValueOnce(new TypeError('Failed to fetch'))
    .mockResolvedValueOnce(Response.json({ ...plan, plan_id: 'fresh-preview' }))
  const user = userEvent.setup()
  render(<SourceCleanup />)
  await user.click(await screen.findByRole('button', { name: 'Delete selected…' }))
  await user.click(screen.getByRole('button', { name: 'Permanently delete 3 archives' }))
  expect((await screen.findByRole('alert')).textContent).toContain('Could not reach Tana')
  expect(screen.queryByRole('button', { name: 'Delete selected…' })).toBeNull()
  expect(screen.getByText('Refresh the preview to check current sources before another cleanup.')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Refresh preview' }))
  expect(await screen.findByRole('button', { name: 'Delete selected…' })).toBeTruthy()
  expect(screen.queryByRole('alert')).toBeNull()
})
