import { expect, test } from 'vitest'
import { formatBytes } from './formatBytes'

test('formats binary units with one decimal', () => {
  expect(formatBytes(0)).toBe('0 B')
  expect(formatBytes(1023)).toBe('1,023 B')
  expect(formatBytes(2150)).toBe('2.1 KiB')
  expect(formatBytes(1024 * 1024 * 1.4)).toBe('1.4 MiB')
  expect(formatBytes(1024 * 1024 - 1)).toBe('1.0 MiB')
})
