import Pagination from '../pagination/Pagination'
import { collectorLink } from '../navigation'
import type { CollectorPage } from '../navigation'

export default function CollectorPagination({ collectorPage, state = '', page, totalPages, label }: {
  collectorPage: CollectorPage, state?: string, page: number, totalPages: number, label: string,
}) {
  return <Pagination page={page} totalPages={totalPages} label={label}
    pageHref={(number) => collectorLink(collectorPage, { page: number, state })} />
}
