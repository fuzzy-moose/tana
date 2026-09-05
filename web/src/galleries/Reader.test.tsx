// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import App from '../App'
import { galleryLink, readerLink } from '../navigation'
import Reader from './Reader'

let wide: Set<number>
let failed: Set<number>
let imagePages: Map<string, number>
let sequence = 0
const fetchMock = vi.fn<typeof fetch>()
const revoke = vi.fn()

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  wide = new Set()
  failed = new Set()
  imagePages = new Map()
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input) => {
    const match = String(input).match(/pages\/(\d+)\/image/)
    if (!match) return Response.json({ id: 1, title: 'Test manga', page_count: 8 })
    const page = Number(match[1])
    if (failed.has(page)) return new Response(null, { status: 422 })
    const blob = Object.assign(new Blob(), { page })
    return Object.assign(new Response(), { blob: async () => blob })
  })
  vi.stubGlobal('fetch', fetchMock)
  vi.stubGlobal('Image', class {
    src = ''
    naturalHeight = 1000
    get naturalWidth() { return wide.has(imagePages.get(this.src)!) ? 1600 : 700 }
    decode() { return Promise.resolve() }
  })
  vi.stubGlobal('URL', Object.assign(class extends URL {}, {
    createObjectURL: (blob: Blob & { page: number }) => {
      const url = `blob:test-${++sequence}`
      imagePages.set(url, blob.page)
      return url
    },
    revokeObjectURL: revoke,
  }))
})

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

async function position(text: string) {
  await waitFor(() => expect(screen.getByText(text)).toBeTruthy())
  await waitFor(() => expect(screen.getByLabelText('Reading spread').getAttribute('aria-busy')).toBe('false'))
}

test('keyboard and mouse navigate right-to-left; one-page turns retain shifted pairing', async () => {
  render(<Reader id={1} initialPage={1} />)
  await position('Page 1 of 8')
  expect(window.location.hash).toBe('#/galleries/1/page/1')
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  await position('Page 2–3 of 8')
  expect(window.location.hash).toBe('#/galleries/1/page/2-3')
  expect(screen.getAllByRole('img').map((image) => image.getAttribute('alt'))).toEqual(['Page 2', 'Page 3'])
  fireEvent.keyDown(window, { key: 'ArrowLeft', shiftKey: true })
  await position('Page 3–4 of 8')
  expect(window.location.hash).toBe('#/galleries/1/page/3-4')
  fireEvent.click(screen.getByRole('button', { name: 'Next spread' }))
  await position('Page 5–6 of 8')
  fireEvent.click(screen.getByRole('button', { name: 'Previous spread' }))
  await position('Page 3–4 of 8')
  fireEvent.click(screen.getByRole('button', { name: '1 page →' }))
  await position('Page 2–3 of 8')
})

test('handles wide boundaries, a selected start page, and the end of a gallery', async () => {
  wide.add(3)
  render(<Reader id={1} initialPage={2} />)
  await position('Page 2 of 8')
  await screen.findByRole('img', { name: 'Page 2' })
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  await position('Page 3 of 8')
  expect(window.location.hash).toBe('#/galleries/1/page/3')
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  await position('Page 4–5 of 8')
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  await position('Page 6–7 of 8')
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  await screen.findByText('End of gallery')
  expect((screen.getByRole('button', { name: '← Next' }) as HTMLButtonElement).disabled).toBe(true)
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  expect(screen.getByRole('img', { name: 'Page 8' })).toBeTruthy()
  fireEvent.keyDown(window, { key: 'Escape' })
  expect(window.location.hash).toBe('#/galleries/1')
})

test('retries a missing image and does not skip its page', async () => {
  failed.add(2)
  render(<Reader id={1} initialPage={2} />)
  await screen.findByRole('group', { name: 'Page 2 unavailable' })
  expect(screen.getByRole('img', { name: 'Page 3' })).toBeTruthy()
  failed.delete(2)
  fireEvent.click(screen.getByRole('button', { name: 'Retry page 2' }))
  expect(await screen.findByRole('img', { name: 'Page 2' })).toBeTruthy()
  expect(screen.getByText('Page 2–3 of 8')).toBeTruthy()
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  await position('Page 4–5 of 8')
})

test('preserves a selected odd pairing when reversing past the cover and advancing again', async () => {
  render(<Reader id={1} initialPage={3} />)
  await position('Page 3–4 of 8')
  await waitFor(() => expect((screen.getByRole('button', { name: 'Previous →' }) as HTMLButtonElement).disabled).toBe(false))
  fireEvent.keyDown(window, { key: 'ArrowRight' })
  await position('Page 2 of 8')
  expect(screen.queryByRole('img', { name: 'Page 3' })).toBeNull()
  fireEvent.keyDown(window, { key: 'ArrowRight' })
  await position('Page 1 of 8')
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  await position('Page 2 of 8')
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  await position('Page 3–4 of 8')
})

test('progress includes the visible spread and supports page selection with the keyboard', async () => {
  render(<Reader id={1} initialPage={1} />)
  await position('Page 1 of 8')
  const progress = screen.getByRole('slider', { name: 'Reading progress' })
  expect(progress.style.getPropertyValue('--progress')).toBe('12.5%')
  fireEvent.keyDown(progress, { key: 'ArrowLeft' })
  await position('Page 2–3 of 8')
  expect(progress.getAttribute('aria-valuetext')).toBe('Page 2–3 of 8')
  expect(progress.style.getPropertyValue('--progress')).toBe('37.5%')
  fireEvent.keyDown(progress, { key: 'End' })
  await position('Page 8 of 8')
  expect(window.location.hash).toBe('#/galleries/1/page/8')
  expect(progress.style.getPropertyValue('--progress')).toBe('100%')
  fireEvent.keyDown(progress, { key: 'ArrowRight' })
  await position('Page 7–8 of 8')
  fireEvent.keyDown(progress, { key: 'Home' })
  await position('Page 1 of 8')
})

test('selecting progress steps runs right-to-left and starts a fresh pairing', async () => {
  render(<Reader id={1} initialPage={3} />)
  await position('Page 3–4 of 8')
  const progress = screen.getByRole('slider', { name: 'Reading progress' })
  progress.setPointerCapture = vi.fn()
  vi.spyOn(progress, 'getBoundingClientRect').mockReturnValue({ left: 100, right: 900, width: 800 } as DOMRect)
  fireEvent.pointerDown(progress, { button: 0, clientX: 550, pointerId: 1 })
  await position('Page 4–5 of 8')
  expect(window.location.hash).toBe('#/galleries/1/page/4-5')
  fireEvent.keyDown(window, { key: 'ArrowRight' })
  await position('Page 2–3 of 8')
  fireEvent.pointerDown(progress, { button: 0, clientX: 100, pointerId: 2 })
  await position('Page 8 of 8')
  fireEvent.pointerDown(progress, { button: 0, clientX: 900, pointerId: 3 })
  await position('Page 1 of 8')
})

test('toggling touch controls keeps the current spread and side navigation available', async () => {
  render(<Reader id={1} initialPage={1} />)
  await position('Page 1 of 8')
  const toggle = screen.getByRole('button', { name: 'Toggle reader controls' })
  expect(toggle.getAttribute('aria-expanded')).toBe('false')
  fireEvent.click(toggle)
  expect(toggle.getAttribute('aria-expanded')).toBe('true')
  expect(screen.getByText('Page 1 of 8')).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Next spread' }))
  await position('Page 2–3 of 8')
  fireEvent.click(toggle)
  expect(toggle.getAttribute('aria-expanded')).toBe('false')
  expect(screen.getByText('Page 2–3 of 8')).toBeTruthy()
})

test.each([
  { entryPage: 1, key: 'ArrowLeft', positionText: 'Page 2–3 of 8', path: '2-3' },
  { entryPage: 3, key: 'ArrowRight', positionText: 'Page 2 of 8', path: '2' },
])('replaces the reader history entry and restores spread $path with Back/Forward', async ({ entryPage, key, positionText, path }) => {
  window.history.replaceState(null, '', galleryLink(1))
  window.history.pushState({ retained: true }, '', readerLink(1, entryPage))
  const historyLength = window.history.length
  render(<App />)
  await position(entryPage === 1 ? 'Page 1 of 8' : 'Page 3–4 of 8')
  await waitFor(() => expect((screen.getByRole('button', { name: 'Previous →' }) as HTMLButtonElement).disabled).toBe(entryPage === 1))
  fireEvent.keyDown(window, { key })
  await position(positionText)
  expect(window.location.hash).toBe(`#/galleries/1/page/${path}`)
  expect(window.history.length).toBe(historyLength)
  expect(window.history.state).toEqual({ retained: true })

  window.history.back()
  await screen.findByRole('link', { name: 'Read gallery' })
  expect(window.location.hash).toBe(galleryLink(1))
  window.history.forward()
  await position(positionText)
  expect(window.location.hash).toBe(`#/galleries/1/page/${path}`)

  fireEvent.keyDown(window, { key: 'Escape' })
  await screen.findByRole('link', { name: 'Read gallery' })
  window.history.back()
  await position(positionText)
  expect(window.location.hash).toBe(`#/galleries/1/page/${path}`)
})

test.each([
  { path: '2', positionText: 'Page 2 of 8', pages: ['Page 2'] },
  { path: '3-4', positionText: 'Page 3–4 of 8', pages: ['Page 3', 'Page 4'] },
])('opens saved spread $path directly', async ({ path, positionText, pages }) => {
  window.history.replaceState(null, '', `#/galleries/1/page/${path}`)
  render(<App />)
  await position(positionText)
  expect(screen.getAllByRole('img').map((image) => image.getAttribute('alt'))).toEqual(pages)
  expect(window.location.hash).toBe(`#/galleries/1/page/${path}`)
})
