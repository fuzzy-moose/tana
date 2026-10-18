import { resetBan } from './api'
import type { BanGroup } from './api'
import { useMutation } from './useMutation'

export function useBanReset(group: BanGroup, available: boolean, onReset: () => void) {
  const { pending, error, run } = useMutation()
  return { pending, error, reset: () => { if (available) void run((signal) => resetBan(group, signal), { onSuccess: onReset }) } }
}
