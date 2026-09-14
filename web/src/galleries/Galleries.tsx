import { useCallback, useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { Badge, Button, Callout, DropdownMenu, Flex, Heading, Text } from '@radix-ui/themes'
import { ExclamationTriangleIcon } from '@radix-ui/react-icons'
import { listingLink } from '../navigation'
import type { GallerySort } from '../navigation'
import Pagination from '../pagination/Pagination'
import PandaCategoryFilter from '../panda/PandaCategoryFilter'
import { listGalleries } from './api'
import type { GalleryListing } from './api'
import GalleryCard from './GalleryCard'
import GallerySearch from './GallerySearch'
import { replaceListingPage, useGalleryLayout } from './useGalleryLayout'
import './Galleries.css'

const sortLabels: Record<GallerySort, string> = {
  title: 'Title · A–Z',
  favorited_desc: 'Panda favorite time · Newest first',
  favorited_asc: 'Panda favorite time · Oldest first',
}

export default function Galleries({ search, page, categories, sort }: { search: string, page: number, categories: string[], sort: GallerySort }) {
  const [result, setResult] = useState<GalleryListing | null>(null)
  const [loadedScope, setLoadedScope] = useState('')
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  const pageHref = useCallback((query: string, number = 1) => listingLink(query, number, categories, sort), [categories, sort])
  const { viewportRef, cardRef, pageSize, cardHeight } = useGalleryLayout(search, page, pageHref)
  const scope = JSON.stringify([search, categories, sort])
  const current = loadedScope === scope && result?.page_size === pageSize && result.page === page ? result : null

  useEffect(() => {
    if (!pageSize) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function refresh() {
      try {
        const next = await listGalleries(search, page, pageSize, categories, sort, controller.signal)
        if (!controller.signal.aborted) {
          setResult(next)
          setLoadedScope(scope)
          setError('')
          if (next.page !== page) replaceListingPage(search, next.page, pageHref)
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
  }, [search, page, pageSize, categories, sort, scope, pageHref, attempt])

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
      if (nextPage >= 1 && nextPage <= Math.ceil(current.total / current.page_size)) window.location.hash = pageHref(search, nextPage)
    }
    window.addEventListener('keydown', navigate)
    return () => window.removeEventListener('keydown', navigate)
  }, [current, search, pageHref])

  return (
    <section className="gallery-listing" aria-labelledby="galleries-title" style={{ '--gallery-card-height': `${cardHeight}px` } as CSSProperties}>
      <div className="gallery-toolbar">
        <Flex align="center" gap="3">
          <Heading as="h1" size="5" id="galleries-title">Galleries</Heading>
          {result && <Badge color="gray">{result.total.toLocaleString()} {result.total === 1 ? 'gallery' : 'galleries'}</Badge>}
        </Flex>
        <GallerySearch key={search} search={search} searchHref={pageHref} />
        <PandaCategoryFilter categories={categories} onChange={(next) => { window.location.hash = listingLink(search, 1, next, sort) }} />
        <DropdownMenu.Root>
          <DropdownMenu.Trigger><Button variant="soft" color="gray">Sort · {sortLabels[sort]}</Button></DropdownMenu.Trigger>
          <DropdownMenu.Content>
            <DropdownMenu.RadioGroup value={sort} onValueChange={(value) => { window.location.hash = listingLink(search, 1, categories, value as GallerySort) }}>
              {Object.entries(sortLabels).map(([value, label]) => <DropdownMenu.RadioItem key={value} value={value}>{label}</DropdownMenu.RadioItem>)}
            </DropdownMenu.RadioGroup>
          </DropdownMenu.Content>
        </DropdownMenu.Root>
        {(search || categories.length > 0) && <Button asChild variant="ghost" color="gray"><a href={listingLink('', 1, [], sort)}>Clear all filters</a></Button>}
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
          <Heading as="h2" size="4">{search || categories.length ? 'No matching galleries.' : 'Your next read starts here.'}</Heading>
          <Text as="p" size="2" color="gray">{search || categories.length ? 'Try different titles, tags, or categories.' : 'Add a library and scan it to discover your comics and manga.'}</Text>
          <Button asChild variant="soft"><a href={search || categories.length ? listingLink('', 1, [], sort) : '#/libraries'}>{search || categories.length ? 'Show all galleries' : 'Manage libraries'}</a></Button>
        </Flex> : <ul className="gallery-grid" aria-label="Galleries">
          {current.items.map((gallery) => <li key={gallery.id}><GalleryCard gallery={gallery} /></li>)}
        </ul>)}
      </div>
      <div className="gallery-pagination">
        {result && <Pagination page={current?.page ?? page} totalPages={Math.ceil(result.total / (pageSize || 1))} pageHref={(number) => pageHref(search, number)} adjacentCount={1} label="Gallery pages" />}
      </div>
    </section>
  )
}
