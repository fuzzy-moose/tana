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
        ? <a className="pagination-item" href={pageHref(page - 1)} aria-label="Previous" rel="prev">«</a>
        : <button className="pagination-item" aria-label="Previous" disabled>«</button>}
      {getPageNumbers(adjacentCount, page, totalPages).map((number, index) => number === -1
        ? <span className="pagination-ellipsis" key={`ellipsis-${index}`}>…</span>
        : number === page
          ? <span className="pagination-item" key={number} aria-label={`Page ${number}`} aria-current="page">{number}</span>
          : <a className="pagination-item" key={number} href={pageHref(number)} aria-label={`Page ${number}`}>{number}</a>)}
      {page < totalPages
        ? <a className="pagination-item" href={pageHref(page + 1)} aria-label="Next" rel="next">»</a>
        : <button className="pagination-item" aria-label="Next" disabled>»</button>}
    </nav>
  )
}
