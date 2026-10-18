import { useEffect } from 'react'
import { collectorLink } from '../navigation'
import type { CollectorPage } from '../navigation'
import { collectorPagination } from './collectorPagination'

/** Computes the page count and, once the total is `loaded`, replaces a URL past the last page with the last page. */
export function useCollectorPagination({ collectorPage, state = '', total, pageSize, page, loaded }: {
  collectorPage: CollectorPage, state?: string, total: number, pageSize: number, page: number, loaded: boolean,
}) {
  const { totalPages, page: currentPage } = collectorPagination({ total, pageSize, page })
  const pastTheEnd = loaded && page > totalPages

  useEffect(() => {
    if (pastTheEnd) window.location.replace(collectorLink(collectorPage, { page: totalPages, state }))
  }, [pastTheEnd, collectorPage, totalPages, state])

  return { totalPages, page: currentPage }
}
