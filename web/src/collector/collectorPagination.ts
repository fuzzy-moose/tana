export function collectorPagination({ total, pageSize, page }: { total: number, pageSize: number, page: number }) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  return { totalPages, page: Math.min(Math.max(page, 1), totalPages) }
}
