import { Button, Text } from '@radix-ui/themes'
import { getPageNumbers } from './pageNumbers'
import './Pagination.css'

interface PaginationProps {
  page: number
  totalPages: number
  pageHref: (page: number) => string
  adjacentCount?: number
  label?: string
}

export default function Pagination({ page, totalPages, pageHref, adjacentCount = 3, label = 'Pagination' }: PaginationProps) {
  if (totalPages <= 1) return null

  return (
    <nav className="pagination" aria-label={label}>
      {page > 1
        ? <Button asChild variant="soft" color="gray" className="pagination-item"><a href={pageHref(page - 1)} aria-label="Previous" rel="prev">«</a></Button>
        : <Button variant="soft" color="gray" className="pagination-item" aria-label="Previous" disabled>«</Button>}
      {getPageNumbers(adjacentCount, page, totalPages).map((number, index) => number === -1
        ? <Text size="2" color="gray" align="center" className="pagination-ellipsis" key={`ellipsis-${index}`}>…</Text>
        : number === page
          ? <Button asChild className="pagination-item" key={number}><span aria-label={`Page ${number}`} aria-current="page">{number}</span></Button>
          : <Button asChild variant="soft" color="gray" className="pagination-item" key={number}><a href={pageHref(number)} aria-label={`Page ${number}`}>{number}</a></Button>)}
      {page < totalPages
        ? <Button asChild variant="soft" color="gray" className="pagination-item"><a href={pageHref(page + 1)} aria-label="Next" rel="next">»</a></Button>
        : <Button variant="soft" color="gray" className="pagination-item" aria-label="Next" disabled>»</Button>}
    </nav>
  )
}
