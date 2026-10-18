import { expect, test } from 'vitest'
import { collectorPagination } from './collectorPagination'

test('collectorPagination derives pages and clamps a past-the-end page', () => {
  expect(collectorPagination({ total: 60, pageSize: 25, page: 2 })).toEqual({ totalPages: 3, page: 2 })
  expect(collectorPagination({ total: 60, pageSize: 25, page: 9 })).toEqual({ totalPages: 3, page: 3 })
  expect(collectorPagination({ total: 0, pageSize: 25, page: 4 })).toEqual({ totalPages: 1, page: 1 })
})
