import { expect, test } from 'vitest'
import { parseGalleryURL } from './downloads'

test('extracts a gallery reference without restricting the host, including copied query strings', () => {
  expect(parseGalleryURL(' https://example.test/g/12345/abc123/?p=1#top ')).toEqual({ gid: 12345, token: 'abc123' })
  expect(parseGalleryURL('http://panda.test/g/42/opaque-token')).toEqual({ gid: 42, token: 'opaque-token' })
})

test.each([
  'https://panda.test/s/token/42-1',
  'https://panda.test/g/0/token/',
  'https://panda.test/g/9007199254740993/token/',
  'https://panda.test/g/42/',
  'https://panda.test/g/42/%20/',
  'https://user:secret@panda.test/g/42/token/',
  'ftp://panda.test/g/42/token/',
])('rejects invalid gallery URL %s', (url) => {
  expect(() => parseGalleryURL(url)).toThrow('Enter a Panda gallery URL')
})
