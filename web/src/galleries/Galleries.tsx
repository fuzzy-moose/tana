import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { Badge, Button, Callout, Flex, Heading, Text } from '@radix-ui/themes'
import { ExclamationTriangleIcon } from '@radix-ui/react-icons'
import { listingLink } from '../navigation'
import Pagination from '../pagination/Pagination'
import { listGalleries } from './api'
import type { GalleryListing } from './api'
import GalleryCard from './GalleryCard'
import GallerySearch from './GallerySearch'
import { replaceListingPage, useGalleryLayout } from './useGalleryLayout'
import './Galleries.css'

export default function Galleries({ search, page }: { search: string, page: number }) {
  const [result, setResult] = useState<GalleryListing | null>(null)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  const { viewportRef, cardRef, pageSize, cardHeight } = useGalleryLayout(search, page)
  const current = result?.page_size === pageSize && result.page === page ? result : null

  useEffect(() => {
    if (!pageSize) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function refresh() {
      try {
        const next = await listGalleries(search, page, pageSize, controller.signal)
        if (!controller.signal.aborted) {
          setResult(next)
          setError('')
          if (next.page !== page) replaceListingPage(search, next.page)
        }
      } catch (error) {
        if (!controller.signal.aborted) setError((error as Error).message)
      } finally {
        // A scan can continue after leaving Libraries; new galleries appear here.
        if (!controller.signal.aborted) timer = setTimeout(refresh, 5000)
      }
    }
    void refresh()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [search, page, pageSize, attempt])

  useEffect(() => {
    if (!current) return
    const navigate = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.isComposing || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return
      if (event.target instanceof HTMLElement && (event.target.isContentEditable || event.target.closest('input, textarea, select, [role="menu"], [role="dialog"], [contenteditable]:not([contenteditable="false"])'))) return
      const direction = event.key === 'ArrowLeft' || event.key === 'a' ? -1
        : event.key === 'ArrowRight' || event.key === 'd' ? 1 : 0
      if (!direction) return
      event.preventDefault()
      const nextPage = current.page + direction
      if (nextPage >= 1 && nextPage <= Math.ceil(current.total / current.page_size)) window.location.hash = listingLink(search, nextPage)
    }
    window.addEventListener('keydown', navigate)
    return () => window.removeEventListener('keydown', navigate)
  }, [current, search])

  return (
    <section className="gallery-listing" aria-labelledby="galleries-title" style={{ '--gallery-card-height': `${cardHeight}px` } as CSSProperties}>
      <div className="gallery-toolbar">
        <Flex align="center" gap="3">
          <Heading as="h1" size="5" id="galleries-title">Galleries</Heading>
          {result && <Badge color="gray">{result.total.toLocaleString()} {result.total === 1 ? 'gallery' : 'galleries'}</Badge>}
        </Flex>
        <GallerySearch key={search} search={search} />
      </div>
      {error && <Callout.Root size="1" color="red" role="alert">
        <Callout.Icon><ExclamationTriangleIcon /></Callout.Icon>
        <Flex align="center" justify="between" wrap="wrap" gap="3">
          <Callout.Text>{error}</Callout.Text>
          <Button variant="soft" color="red" size="1" onClick={() => setAttempt((n) => n + 1)}>Retry</Button>
        </Flex>
      </Callout.Root>}
      <div className="gallery-viewport" ref={viewportRef} aria-busy={!current && !error}>
        <div className="gallery-grid gallery-sizing" aria-hidden="true">
          <div className="gallery-card" ref={cardRef}><div className="gallery-cover" /><div className="gallery-title"><Text asChild size="1" weight="medium"><h2 /></Text></div><Text as="p" size="1">0 pages</Text></div>
        </div>
        {!current && !error && <Text as="p" size="2" color="gray" role="status">Loading galleries…</Text>}
        {current && (current.items.length === 0 ? <Flex className="gallery-empty" direction="column" align="center" justify="center" gap="3">
          <Heading as="h2" size="4">{search ? 'No matching galleries.' : 'Your next read starts here.'}</Heading>
          <Text as="p" size="2" color="gray">{search ? 'Try different titles or tags.' : 'Add a library and scan it to discover your comics and manga.'}</Text>
          <Button asChild variant="soft"><a href={search ? '#/' : '#/libraries'}>{search ? 'Clear search' : 'Manage libraries'}</a></Button>
        </Flex> : <ul className="gallery-grid" aria-label="Galleries">
          {current.items.map((gallery) => <li key={gallery.id}><GalleryCard gallery={gallery} /></li>)}
        </ul>)}
      </div>
      <div className="gallery-pagination">
        <Text size="1" color="gray" className="gallery-sort">Title · A–Z</Text>
        {result && <Pagination page={current?.page ?? page} totalPages={Math.ceil(result.total / (pageSize || 1))} pageHref={(number) => listingLink(search, number)} adjacentCount={1} label="Gallery pages" />}
      </div>
    </section>
  )
}
