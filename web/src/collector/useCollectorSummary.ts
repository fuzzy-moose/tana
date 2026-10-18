import { getCollectorSummary } from './api'
import { useCollectorResource } from './useCollectorResource'

export function useCollectorSummary(available: boolean, refreshKey: number, interval: number) {
  return useCollectorResource(getCollectorSummary, available, refreshKey, interval)
}
