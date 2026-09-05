export type PageShape = 'portrait' | 'wide' | 'error'

export function spreadPages(start: number, total: number, shape: (page: number) => PageShape | undefined): number[] {
  if (total === 0) return []
  if (start === 1 || start >= total || shape(start) === 'wide' || shape(start + 1) === 'wide') return [start]
  return [start, start + 1]
}

export function previousSpread(start: number, shape: (page: number) => PageShape | undefined): number {
  if (start <= 2) return 1
  if (start === 3 || shape(start - 1) === 'wide' || shape(start - 2) === 'wide') return start - 1
  return start - 2
}
