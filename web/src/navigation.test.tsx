// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import { collectorLink, useCollectorListParams, useRoute } from './navigation'

afterEach(() => { cleanup(); window.location.hash = '' })

test('collector links round trip through the route and omit defaults', async () => {
  expect(collectorLink('feeds')).toBe('#/collector/feeds')
  expect(collectorLink('overview')).toBe('#/collector')
  const link = collectorLink('feeds', { page: 3, state: 'failed' })
  expect(link).toBe('#/collector/feeds?page=3&state=failed')
  window.location.hash = link
  const { result } = renderHook(() => ({ route: useRoute(), params: useCollectorListParams() }))
  expect(result.current.route).toMatchObject({ kind: 'collector', page: 'feeds', listPage: 3, state: 'failed' })
  expect(result.current.params).toEqual({ page: 3, state: 'failed' })
  act(() => { window.location.hash = collectorLink('feeds') })
  await waitFor(() => expect(result.current.params).toEqual({ page: 1, state: '' }))
})
