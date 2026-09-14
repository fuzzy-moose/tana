import { useCallback, useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { Badge, Button, Callout, DropdownMenu, Flex, Heading, Spinner, Text } from '@radix-ui/themes'
import { DotsHorizontalIcon, ExclamationTriangleIcon, ReloadIcon } from '@radix-ui/react-icons'
import GallerySearch from '../galleries/GallerySearch'
import { replaceListingPage, useGalleryLayout } from '../galleries/useGalleryLayout'
import { pandaListingLink } from '../navigation'
import Pagination from '../pagination/Pagination'
import { completePandaSearch, listPandaCatalog } from './api'
import type { PandaCatalogResult } from './api'
import PandaCard from './PandaCard'
import '../galleries/Galleries.css'
import './PandaCatalog.css'

interface PandaCatalogProps {
  search: string
  page: number
  includeExpunged: boolean
}

export default function PandaCatalog({ search, page, includeExpunged }: PandaCatalogProps) {
  const [result, setResult] = useState<PandaCatalogResult | null>(null)
  const [requestState, setRequestState] = useState<{ key: string, error: string } | null>(null)
  const [attempt, setAttempt] = useState(0)
  const pageHref = useCallback((query: string, number = 1) => pandaListingLink(query, number, includeExpunged), [includeExpunged])
  const { viewportRef, cardRef, pageSize, cardHeight } = useGalleryLayout(search, page, pageHref)
  const requestKey = JSON.stringify([search, page, pageSize, includeExpunged, attempt])
  const loading = requestState?.key !== requestKey
  // Keep the alert's height stable while retrying: it affects measured page capacity.
  const error = requestState?.error ?? ''

  useEffect(() => {
    if (!pageSize) return
    const controller = new AbortController()
    void listPandaCatalog(search, page, pageSize, includeExpunged, controller.signal).then((next) => {
      if (controller.signal.aborted) return
      setResult(next)
      setRequestState({ key: requestKey, error: '' })
      if (next.page !== page) replaceListingPage(search, next.page, pageHref)
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setRequestState({ key: requestKey, error: error.message })
    })
    return () => controller.abort()
  }, [search, page, pageSize, includeExpunged, requestKey, pageHref])

  useEffect(() => {
    if (!result || loading) return
    const navigate = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.isComposing || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return
      if (event.target instanceof HTMLElement && (event.target.isContentEditable || event.target.closest('input, textarea, select, [role="menu"], [role="dialog"], [contenteditable]:not([contenteditable="false"])'))) return
      const direction = event.key === 'ArrowLeft' || event.key === 'a' ? -1 : event.key === 'ArrowRight' || event.key === 'd' ? 1 : 0
      if (!direction) return
      event.preventDefault()
      const next = result.page + direction
      if (next >= 1 && next <= result.total_pages) window.location.hash = pageHref(search, next)
    }
    window.addEventListener('keydown', navigate)
    return () => window.removeEventListener('keydown', navigate)
  }, [result, loading, pageHref, search])

  return <section className="gallery-listing panda-catalog" aria-labelledby="panda-title" style={{ '--gallery-card-height': `${cardHeight}px` } as CSSProperties}>
    <div className="gallery-toolbar">
      <Flex align="center" gap="3">
        <Heading as="h1" size="5" id="panda-title">Panda</Heading>
        {result && <Badge color="gray">{result.total.toLocaleString()} {result.total === 1 ? 'gallery' : 'galleries'}</Badge>}
      </Flex>
      <GallerySearch key={search} search={search} completeSearch={completePandaSearch} searchHref={pageHref} />
      <Flex align="center" gap="2">
        {loading && result && <Spinner aria-label="Loading results" />}
        <DropdownMenu.Root>
          <DropdownMenu.Trigger><Button variant="soft" color="gray">{includeExpunged ? 'Options · 1' : 'Options'}<DotsHorizontalIcon /></Button></DropdownMenu.Trigger>
          <DropdownMenu.Content align="end">
            <DropdownMenu.CheckboxItem checked={includeExpunged} onCheckedChange={(checked) => { window.location.hash = pandaListingLink(search, 1, checked) }}>Include expunged</DropdownMenu.CheckboxItem>
            <DropdownMenu.Item disabled={loading} onSelect={() => setAttempt((value) => value + 1)}><ReloadIcon />Reload results</DropdownMenu.Item>
            <DropdownMenu.Separator />
            <DropdownMenu.Item asChild><a href="#/panda/lookup">Look up gallery</a></DropdownMenu.Item>
            <DropdownMenu.Item asChild><a href="#/collector/feeds">Collect feeds</a></DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Root>
      </Flex>
    </div>

    {error && <Callout.Root size="1" color="red" role="alert">
      <Callout.Icon><ExclamationTriangleIcon /></Callout.Icon>
      <Flex align="center" justify="between" wrap="wrap" gap="3">
        <Callout.Text>{result ? 'Results are stale. Showing previously loaded galleries.' : 'Panda catalog unavailable.'} {error}</Callout.Text>
        <Button variant="soft" color="red" size="1" onClick={() => setAttempt((value) => value + 1)}>Retry</Button>
      </Flex>
    </Callout.Root>}

    <div className="gallery-viewport" ref={viewportRef} aria-busy={loading}>
      <div className="gallery-grid gallery-sizing" aria-hidden="true"><div className="gallery-card panda-card" ref={cardRef}><div className="gallery-cover" /><div className="gallery-title"><Text asChild size="1" weight="medium"><h2 /></Text></div><Text as="p" size="1">0 pages · 12/12/2026</Text></div></div>
      {!result && loading && !error && <Text as="p" size="2" color="gray" role="status">Loading Panda galleries…</Text>}
      {result && (result.items.length === 0 ? <Flex className="gallery-empty" direction="column" align="center" justify="center" gap="3">
        <Heading as="h2" size="4">{search ? 'No matching Panda galleries.' : 'No collected Panda galleries yet.'}</Heading>
        <Text as="p" size="2" color="gray">{search ? 'Try different titles or tags.' : 'Collect a feed to discover galleries, then return when metadata is available.'}</Text>
        <Button asChild variant="soft"><a href={search ? pageHref('') : '#/collector/feeds'}>{search ? 'Clear search' : 'Collect feeds'}</a></Button>
      </Flex> : <ul className="gallery-grid" aria-label="Panda galleries">{result.items.map((gallery) => <li key={gallery.gallery_id}><PandaCard gallery={gallery} /></li>)}</ul>)}
    </div>
    <div className="gallery-pagination">
      <Text size="1" color="gray" className="gallery-sort">Upload date · Newest first</Text>
      {result && <Pagination page={result.page} totalPages={result.total_pages} pageHref={(number) => pageHref(search, number)} adjacentCount={1} label="Panda gallery pages" />}
    </div>
  </section>
}
