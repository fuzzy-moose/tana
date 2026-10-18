import { expect, test } from 'vitest'
import { previousSpread, spreadPages } from './spreads'
import type { PageShape } from './spreads'

test('cover alone, then pairs; an override shifts subsequent pairing', () => {
  const portrait = () => 'portrait' as const
  expect(spreadPages(1, 8, portrait)).toEqual([1])
  expect(spreadPages(2, 8, portrait)).toEqual([2, 3])
  expect(spreadPages(3, 8, portrait)).toEqual([3, 4])
  expect(spreadPages(5, 8, portrait)).toEqual([5, 6])
  expect(previousSpread(5, portrait)).toBe(3)
  expect(previousSpread(3, portrait)).toBe(2)
  expect(previousSpread(2, portrait)).toBe(1)
  expect(spreadPages(8, 8, portrait)).toEqual([8])
})

test('wide pages and portraits preceding them stand alone in both directions', () => {
  const shape = (page: number): PageShape => [3, 6, 7].includes(page) ? 'wide' : 'portrait'
  let start = 1
  const forward: number[][] = []
  while (start <= 9) {
    const spread = spreadPages(start, 9, shape)
    forward.push(spread)
    start += spread.length
  }
  expect(forward).toEqual([[1], [2], [3], [4, 5], [6], [7], [8, 9]])
  const backward = [8]
  while (backward.at(-1)! > 1) backward.push(previousSpread(backward.at(-1)!, shape))
  expect(backward).toEqual([8, 7, 6, 4, 3, 2, 1])
  expect(spreadPages(5, 9, shape)).toEqual([5])
})

test('unreadable pages retain their position in a spread', () => {
  expect(spreadPages(2, 4, () => 'error')).toEqual([2, 3])
  expect(spreadPages(1, 0, () => undefined)).toEqual([])
})
