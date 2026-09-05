import type { CSSProperties, KeyboardEvent } from 'react'

export default function ReaderProgress({ page, lastPage, total, onSelect }: {
  page: number
  lastPage: number
  total: number
  onSelect: (page: number) => void
}) {
  function select(value: number) {
    onSelect(Math.max(1, Math.min(total, value)))
  }

  function selectAt(clientX: number, element: HTMLDivElement) {
    const { right, width } = element.getBoundingClientRect()
    if (width > 0) select(Math.floor((right - clientX) / width * total) + 1)
  }

  function navigate(event: KeyboardEvent<HTMLDivElement>) {
    let next: number
    switch (event.key) {
      case 'ArrowLeft':
      case 'ArrowUp': next = page + 1; break
      case 'ArrowRight':
      case 'ArrowDown': next = page - 1; break
      case 'Home': next = 1; break
      case 'End': next = total; break
      default: return
    }
    event.preventDefault()
    event.stopPropagation()
    select(next)
  }

  return <div
    className="reader-progress"
    role="slider"
    aria-label="Reading progress"
    aria-orientation="horizontal"
    aria-valuemin={1}
    aria-valuemax={total}
    aria-valuenow={page}
    aria-valuetext={`Page ${page === lastPage ? page : `${page}–${lastPage}`} of ${total}`}
    tabIndex={0}
    style={{ '--page-count': total, '--progress': `${lastPage / total * 100}%` } as CSSProperties}
    onKeyDown={navigate}
    onPointerDown={(event) => {
      if (event.button !== 0) return
      event.currentTarget.focus({ preventScroll: true })
      event.currentTarget.setPointerCapture(event.pointerId)
      selectAt(event.clientX, event.currentTarget)
    }}
    onPointerMove={(event) => {
      if (event.currentTarget.hasPointerCapture(event.pointerId)) selectAt(event.clientX, event.currentTarget)
    }}
    onPointerUp={(event) => {
      if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
    }}
  >
    <div className="reader-progress-track" aria-hidden="true" />
  </div>
}
