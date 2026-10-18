// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { completeGallerySearch } from './api'
import type { SearchCompletion } from './api'
import GallerySearch from './GallerySearch'

vi.mock('./api', () => ({ completeGallerySearch: vi.fn() }))
const complete = vi.mocked(completeGallerySearch)

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  complete.mockReset()
  complete.mockResolvedValue({ start: 0, end: 6, items: [
    { namespace: 'artist', value: 'artist y', term: '-artist:"artist y$"' },
    { namespace: 'artist', value: 'artist z', term: '-artist:"artist z$"' },
  ] })
})
afterEach(cleanup)

test('selects with arrows and Enter, then requires separate submission', async () => {
  const user = userEvent.setup()
  render(<GallerySearch search="" />)
  const input = screen.getByRole('combobox') as HTMLInputElement
  await user.type(input, '-a:art')
  await screen.findByRole('listbox', { name: 'Tag suggestions' })
  expect(complete).toHaveBeenLastCalledWith('-a:art', 6, expect.any(AbortSignal))
  expect(screen.getAllByRole('option')[0].getAttribute('aria-selected')).toBe('true')
  await user.keyboard('{ArrowDown}{Enter}')
  expect(input.value).toBe('-artist:"artist z$"')
  expect(input.selectionStart).toBe(input.value.length)
  expect(screen.queryByRole('listbox')).toBeNull()
  expect(window.location.hash).toBe('')
  await user.keyboard('{Enter}')
  expect(new URLSearchParams(window.location.hash.split('?')[1]).get('q')).toBe('-artist:"artist z$"')
})

test('click replaces the term at a UTF-16 cursor and preserves surrounding terms', async () => {
  const query = 'title:"🦊" ~a:artist_y$ -red'
  const start = query.indexOf('~')
  const end = query.indexOf(' -red')
  const cursor = start + '~a:art'.length
  complete.mockResolvedValue({ start, end, items: [
    { namespace: 'artist', value: 'artist y', term: '~artist:"artist y$"' },
  ] })
  const user = userEvent.setup()
  render(<GallerySearch search={query} />)
  const input = screen.getByRole('combobox') as HTMLInputElement
  await user.click(input)
  input.setSelectionRange(cursor, cursor)
  fireEvent.select(input)
  await user.click(await screen.findByRole('option'))
  expect(complete).toHaveBeenLastCalledWith(query, cursor, expect.any(AbortSignal))
  expect(input.value).toBe('title:"🦊" ~artist:"artist y$" -red')
  expect(document.activeElement).toBe(input)
  expect(input.selectionStart).toBe(start + '~artist:"artist y$"'.length)
  expect(window.location.hash).toBe('')
})

test('Escape dismisses without clearing text; stale requests cannot restore suggestions', async () => {
  let resolve!: (result: SearchCompletion) => void
  complete.mockImplementationOnce(() => new Promise((done) => { resolve = done }))
  const user = userEvent.setup()
  render(<GallerySearch search="" />)
  const input = screen.getByRole('combobox') as HTMLInputElement
  await user.click(input)
  expect(complete).not.toHaveBeenCalled()
  await user.type(input, 'old')
  await waitFor(() => expect(complete).toHaveBeenCalledTimes(1))
  await user.clear(input)
  await user.type(input, '-a:art')
  await screen.findByRole('listbox')
  await act(async () => resolve({ start: 0, end: 3, items: [{ namespace: 'other', value: 'old', term: 'other:old$' }] }))
  expect(screen.queryByText('old')).toBeNull()
  await user.keyboard('{Escape}')
  expect(input.value).toBe('-a:art')
  expect(input.getAttribute('aria-expanded')).toBe('false')
  expect(screen.queryByRole('listbox')).toBeNull()
})

test('completion failures and IME composition do not interrupt editing', async () => {
  complete.mockRejectedValue(new Error('offline'))
  const user = userEvent.setup()
  render(<GallerySearch search="" />)
  const input = screen.getByRole('combobox') as HTMLInputElement
  await user.click(input)
  fireEvent.compositionStart(input)
  fireEvent.change(input, { target: { value: '物語' } })
  fireEvent.submit(screen.getByRole('search'))
  expect(window.location.hash).toBe('')
  expect(complete).not.toHaveBeenCalled()
  fireEvent.compositionEnd(input)
  await waitFor(() => expect(complete).toHaveBeenCalled())
  expect(screen.queryByRole('alert')).toBeNull()
  expect(input.value).toBe('物語')
  await user.click(screen.getByRole('button', { name: 'Search' }))
  expect(new URLSearchParams(window.location.hash.split('?')[1]).get('q')).toBe('物語')
})
