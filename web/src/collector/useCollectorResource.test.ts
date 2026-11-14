// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { APIError } from '../api'
import { useCollectorResource } from './useCollectorResource'

afterEach(cleanup)

test('reports the API error code of the latest failed load until a load succeeds', async () => {
  const load = vi.fn<(signal: AbortSignal) => Promise<string>>()
    .mockRejectedValueOnce(new APIError('Collector is not configured', 'collector_not_configured'))
    .mockResolvedValue('ok')
  const { result } = renderHook(() => useCollectorResource(load, true, 0, 60_000))

  await waitFor(() => expect(result.current.errorCode).toBe('collector_not_configured'))
  act(() => result.current.refresh())
  await waitFor(() => expect(result.current.data).toBe('ok'))
  expect(result.current.errorCode).toBe('')
})

test('a new interval loads at once by default', async () => {
  const load = vi.fn<(signal: AbortSignal) => Promise<string>>().mockResolvedValue('ok')
  const { result, rerender } = renderHook(({ interval }) => useCollectorResource(load, true, 0, interval), { initialProps: { interval: 60_000 } })
  await waitFor(() => expect(result.current.data).toBe('ok'))
  rerender({ interval: 30_000 })
  await waitFor(() => expect(load).toHaveBeenCalledTimes(2))
})

test('with rescheduleOnIntervalChange a new interval reschedules the next load without loading at once', async () => {
  vi.useFakeTimers()
  try {
    const load = vi.fn<(signal: AbortSignal) => Promise<string>>().mockResolvedValue('ok')
    const { result, rerender } = renderHook(({ interval }) => useCollectorResource(load, true, 0, interval, { rescheduleOnIntervalChange: true }), { initialProps: { interval: 30_000 } })
    await act(async () => { await vi.advanceTimersByTimeAsync(0) })
    expect(result.current.data).toBe('ok')
    expect(load).toHaveBeenCalledTimes(1)
    rerender({ interval: 5_000 })
    await act(async () => { await vi.advanceTimersByTimeAsync(4_999) })
    expect(load).toHaveBeenCalledTimes(1)
    await act(async () => { await vi.advanceTimersByTimeAsync(1) })
    expect(load).toHaveBeenCalledTimes(2)
  } finally {
    vi.useRealTimers()
  }
})
