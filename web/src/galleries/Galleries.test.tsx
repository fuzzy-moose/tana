// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import App from '../App'

let viewport: { width: number, height: number }
let total: number
const observers = new Set<() => void>()
const fetchMock = vi.fn<typeof fetch>()

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  viewport = { width: 716, height: 646 }
  total = 40
  const getStyle = window.getComputedStyle
  vi.spyOn(window, 'getComputedStyle').mockImplementation((element) => {
    const style = getStyle(element)
    if (element.classList.contains('gallery-sizing')) { style.columnGap = '12px'; style.rowGap = '12px' }
    return style
  })
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    return (this.classList.contains('gallery-viewport') ? viewport : { width: 170, height: 317 }) as DOMRect
  })
  vi.stubGlobal('ResizeObserver', class {
    callback: () => void
    constructor(callback: () => void) { this.callback = callback }
    observe() { observers.add(this.callback) }
    disconnect() { observers.delete(this.callback) }
  })
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input) => {
    const url = new URL(String(input), 'http://localhost')
    const pageSize = Number(url.searchParams.get('page_size'))
    const page = Math.min(Number(url.searchParams.get('page')), Math.max(1, Math.ceil(total / pageSize)))
    const first = (page - 1) * pageSize
    return Response.json({
      items: Array.from({ length: Math.min(pageSize, total - first) }, (_, index) => ({ id: String(first + index), title: `Gallery ${first + index}`, page_count: 0 })),
      total, page, page_size: pageSize,
    })
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); observers.clear() })

async function listing(first: number, count: number) {
  await screen.findByRole('heading', { name: `Gallery ${first}` })
  expect(within(screen.getByRole('list', { name: 'Galleries' })).getAllByRole('listitem')).toHaveLength(count)
}

function resize(width: number, height: number) {
  viewport = { width, height }
  act(() => { for (const callback of [...observers]) callback() })
}

test('fits complete rows, caps at 100, and retains one row in a short viewport', async () => {
  total = 500
  render(<App />)
  await listing(0, 8)
  expect(fetchMock).toHaveBeenLastCalledWith('/api/galleries?q=&page=1&page_size=8', expect.anything())
  resize(716, 645)
  await listing(0, 4)
  resize(716, 50)
  await listing(0, 4)
  resize(4000, 4000)
  await listing(0, 100)
})

test('keeps the anchor gallery across consecutive resizes without adding history entries', async () => {
  window.history.replaceState({ retained: true }, '', '#/galleries?q=manga&page=3')
  const historyLength = window.history.length
  render(<App />)
  await listing(16, 8)
  resize(716, 975)
  await listing(12, 12)
  expect(window.location.hash).toBe('#/galleries?q=manga&page=2')
  resize(716, 1304)
  await listing(16, 16)
  resize(716, 646)
  await listing(16, 8)
  expect(window.location.hash).toBe('#/galleries?q=manga&page=3')
  expect(window.history.length).toBe(historyLength)
  expect(window.history.state).toEqual({ retained: true })

  fireEvent.keyDown(window, { key: 'd' })
  await listing(24, 8)
  resize(716, 975)
  await listing(24, 12)
})

test('paginates with arrows and A/D, respects boundaries, and ignores editing and modifiers', async () => {
  total = 16
  render(<App />)
  await listing(0, 8)
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  expect(window.location.hash).toBe('')
  fireEvent.keyDown(window, { key: 'ArrowRight' })
  await listing(8, 8)
  fireEvent.keyDown(window, { key: 'd' })
  expect(window.location.hash).toBe('#/galleries?page=2')
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  await listing(0, 8)
  fireEvent.keyDown(window, { key: 'd' })
  await listing(8, 8)
  fireEvent.keyDown(window, { key: 'a' })
  await listing(0, 8)

  fireEvent.keyDown(screen.getByLabelText('Search titles and tags'), { key: 'd' })
  for (const modifier of ['altKey', 'ctrlKey', 'metaKey', 'shiftKey', 'isComposing']) {
    fireEvent.keyDown(window, { key: 'ArrowRight', [modifier]: true })
  }
  const editor = document.createElement('div')
  editor.setAttribute('contenteditable', 'true')
  const child = document.createElement('span')
  editor.append(child)
  document.body.append(editor)
  fireEvent.keyDown(child, { key: 'ArrowRight' })
  editor.remove()
  expect(window.location.hash).toBe('#/galleries')
})

test('clamps a saved page after galleries disappear and supports an empty search', async () => {
  window.history.replaceState(null, '', '#/galleries?page=20')
  render(<App />)
  await listing(32, 8)
  expect(window.location.hash).toBe('#/galleries?page=5')
  total = 0
  fireEvent.change(screen.getByLabelText('Search titles and tags'), { target: { value: 'missing' } })
  fireEvent.submit(screen.getByRole('search'))
  await screen.findByText('No matching galleries.')
  expect(screen.queryByRole('navigation', { name: 'Gallery pages' })).toBeNull()
})

test('shows the full title on hover and focus, and dismisses it with Escape', async () => {
  render(<App />)
  await listing(0, 8)
  const card = within(screen.getByRole('list', { name: 'Galleries' })).getAllByRole('link')[0]
  const tooltip = () => card.querySelector('.gallery-title-tooltip')
  expect(tooltip()).toBeNull()
  fireEvent.mouseEnter(card)
  expect(tooltip()?.textContent).toBe('Gallery 0')
  fireEvent.mouseLeave(card)
  expect(tooltip()).toBeNull()
  fireEvent.focus(card)
  expect(tooltip()?.textContent).toBe('Gallery 0')
  fireEvent.keyDown(card, { key: 'Escape' })
  expect(tooltip()).toBeNull()
})
