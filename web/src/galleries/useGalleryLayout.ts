import { useLayoutEffect, useRef, useState } from 'react'
import { listingLink } from '../navigation'

export function replaceListingPage(search: string, page: number, pageHref = listingLink) {
  window.history.replaceState(window.history.state, '', pageHref(search, page))
  window.dispatchEvent(new HashChangeEvent('hashchange'))
}

export function useGalleryLayout(search: string, page: number, pageHref = listingLink) {
  const viewportRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  const previousSize = useRef(0)
  const previousScope = useRef({ search, pageHref })
  // Retain this anchor until navigation; consecutive resize events must not drift backward.
  const anchor = useRef({ page, first: 0 })
  const [layout, setLayout] = useState({ pageSize: 0, cardHeight: 0 })

  useLayoutEffect(() => {
    const viewport = viewportRef.current!
    const card = cardRef.current!
    if (previousScope.current.search !== search || previousScope.current.pageHref !== pageHref) {
      previousSize.current = 0
      anchor.current = { page, first: 0 }
      previousScope.current = { search, pageHref }
    }
    if (previousSize.current && anchor.current.page !== page) {
      anchor.current = { page, first: (page - 1) * previousSize.current }
    }
    function measure() {
      const available = viewport.getBoundingClientRect()
      const cardBounds = card.getBoundingClientRect()
      if (available.width <= 0 || cardBounds.width <= 0 || cardBounds.height <= 0) return
      const style = getComputedStyle(card.parentElement!)
      const columnGap = parseFloat(style.columnGap) || 0
      const rowGap = parseFloat(style.rowGap) || 0
      const columns = Math.max(1, Math.round((available.width + columnGap) / (cardBounds.width + columnGap)))
      const rows = Math.max(1, Math.floor((available.height + rowGap) / (cardBounds.height + rowGap)))
      const pageSize = Math.min(100, columns * rows)
      const oldSize = previousSize.current
      if (!oldSize) anchor.current = { page, first: (page - 1) * pageSize }
      previousSize.current = pageSize
      setLayout((current) => current.pageSize === pageSize && current.cardHeight === cardBounds.height
        ? current : { pageSize, cardHeight: cardBounds.height })
      if (oldSize > 0 && oldSize !== pageSize) {
        // Resizing replaces this history entry and keeps its first gallery in view.
        const nextPage = Math.floor(anchor.current.first / pageSize) + 1
        anchor.current.page = nextPage
        if (nextPage !== page) replaceListingPage(search, nextPage, pageHref)
      }
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(viewport)
    observer.observe(card)
    return () => observer.disconnect()
  }, [search, page, pageHref])

  return { viewportRef, cardRef, ...layout }
}
