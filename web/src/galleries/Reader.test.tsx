// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import Reader from './Reader'

let wide: Set<number>
let failed: Set<number>
let imagePages: Map<string, number>
let sequence = 0
const fetchMock = vi.fn<typeof fetch>()
const revoke = vi.fn()

beforeEach(() => {
  wide = new Set()
  failed = new Set()
  imagePages = new Map()
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input) => {
    const match = String(input).match(/pages\/(\d+)\/image/)
    if (!match) return Response.json({ id: 'g', title: 'Test manga', page_count: 8 })
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
  render(<Reader id="g" initialPage={1} />)
  await position('Page 1 of 8')
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  await position('Page 2–3 of 8')
  expect(screen.getAllByRole('img').map((image) => image.getAttribute('alt'))).toEqual(['Page 2', 'Page 3'])
  fireEvent.keyDown(window, { key: 'ArrowLeft', shiftKey: true })
  await position('Page 3–4 of 8')
  fireEvent.click(screen.getByRole('button', { name: 'Next spread' }))
  await position('Page 5–6 of 8')
  fireEvent.click(screen.getByRole('button', { name: 'Previous spread' }))
  await position('Page 3–4 of 8')
  fireEvent.click(screen.getByRole('button', { name: '1 page →' }))
  await position('Page 2–3 of 8')
})

test('handles wide boundaries, a selected start page, and the end of a gallery', async () => {
  wide.add(3)
  render(<Reader id="g" initialPage={2} />)
  await position('Page 2 of 8')
  await screen.findByRole('img', { name: 'Page 2' })
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  await position('Page 3 of 8')
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
  expect(window.location.hash).toBe('#/galleries/g')
})

test('retries a missing image and does not skip its page', async () => {
  failed.add(2)
  render(<Reader id="g" initialPage={2} />)
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
  render(<Reader id="g" initialPage={3} />)
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
