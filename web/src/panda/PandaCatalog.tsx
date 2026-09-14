import { useCallback, useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { Button, Callout, DropdownMenu, Flex, Heading, Spinner, Text } from '@radix-ui/themes'
import { DotsHorizontalIcon, ExclamationTriangleIcon, ReloadIcon } from '@radix-ui/react-icons'
import GallerySearch from '../galleries/GallerySearch'
import { useGalleryGrid } from '../galleries/useGalleryLayout'
import { pandaListingLink } from '../navigation'
import '../pagination/Pagination.css'
import { completePandaSearch, getPandaDefaultFilter, hasDefaultFilter, listPandaCatalog } from './api'
import type { PandaCatalogResult, PandaDefaultFilter as DefaultFilter } from './api'
import PandaCard from './PandaCard'
import PandaCategoryFilter from './PandaCategoryFilter'
import PandaDefaultFilter from './PandaDefaultFilter'
import '../galleries/Galleries.css'
import './PandaCatalog.css'

interface PandaCatalogProps {
  search: string
  cursor: string
  includeExpunged: boolean
  categories: string[]
  bypassDefault: boolean
}

export default function PandaCatalog({ search, cursor, includeExpunged, categories, bypassDefault }: PandaCatalogProps) {
  const [result, setResult] = useState<PandaCatalogResult | null>(null)
  const [requestState, setRequestState] = useState<{ key: string, error: string } | null>(null)
  const [attempt, setAttempt] = useState(0)
  const [defaultFilter, setDefaultFilter] = useState<DefaultFilter | null>(null)
  const [defaultError, setDefaultError] = useState('')
  const pageHref = useCallback((query: string, position = '') => pandaListingLink(query, position, includeExpunged, categories, bypassDefault), [includeExpunged, categories, bypassDefault])
  const { viewportRef, cardRef, pageSize, cardHeight } = useGalleryGrid()
  const requestKey = JSON.stringify([search, cursor, pageSize, includeExpunged, categories, bypassDefault, attempt])
  const loading = requestState?.key !== requestKey
  // Keep the alert's height stable while retrying: it affects measured page capacity.
  const error = requestState?.error ?? ''
  const defaultConfigured = hasDefaultFilter(defaultFilter)
  const filtered = !!search || categories.length > 0 || (defaultConfigured && !bypassDefault)
  const clearHref = pandaListingLink('', '', false, [], true)

  useEffect(() => {
    if (!pageSize) return
    const controller = new AbortController()
    void getPandaDefaultFilter(controller.signal).then((filter) => {
      if (!controller.signal.aborted) { setDefaultFilter(filter); setDefaultError('') }
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setDefaultError(error.message)
    })
    return () => controller.abort()
  }, [requestKey, pageSize])

  useEffect(() => {
    if (!pageSize) return
    const controller = new AbortController()
    void listPandaCatalog(search, cursor, pageSize, includeExpunged, categories, bypassDefault, controller.signal).then((next) => {
      if (controller.signal.aborted) return
      setResult(next)
      setRequestState({ key: requestKey, error: '' })
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setRequestState({ key: requestKey, error: error.message })
    })
    return () => controller.abort()
  }, [search, cursor, pageSize, includeExpunged, categories, bypassDefault, requestKey, pageHref])

  useEffect(() => {
    if (!result || loading || error) return
    const navigate = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.isComposing || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return
      if (event.target instanceof HTMLElement && (event.target.isContentEditable || event.target.closest('input, textarea, select, [role="menu"], [role="dialog"], [contenteditable]:not([contenteditable="false"])'))) return
      const direction = event.key === 'ArrowLeft' || event.key === 'a' ? -1 : event.key === 'ArrowRight' || event.key === 'd' ? 1 : 0
      if (!direction) return
      event.preventDefault()
      const next = direction > 0 ? result.next_cursor : result.previous_cursor
      if (next) window.location.hash = pageHref(search, next)
    }
    window.addEventListener('keydown', navigate)
    return () => window.removeEventListener('keydown', navigate)
  }, [result, loading, error, pageHref, search])

  return <section className="gallery-listing panda-catalog" aria-labelledby="panda-title" style={{ '--gallery-card-height': `${cardHeight}px` } as CSSProperties}>
    <div className="gallery-toolbar">
      <Flex align="center" gap="3">
        <Heading as="h1" size="5" id="panda-title">Panda</Heading>
      </Flex>
      <GallerySearch key={search} search={search} completeSearch={completePandaSearch} searchHref={pageHref} />
      <PandaCategoryFilter categories={categories} onChange={(next) => { window.location.hash = pandaListingLink(search, '', includeExpunged, next, bypassDefault) }} />
      <PandaDefaultFilter filter={defaultFilter} error={defaultError} bypassed={bypassDefault} onRetry={() => setAttempt((value) => value + 1)} onSave={(filter) => {
        setDefaultFilter(filter)
        if (cursor || bypassDefault) window.location.hash = pandaListingLink(search, '', includeExpunged, categories)
        else setAttempt((value) => value + 1)
      }} />
      {(defaultConfigured || bypassDefault) && <Button asChild variant="ghost" color="gray"><a href={pandaListingLink(search, '', includeExpunged, categories, !bypassDefault)}>{bypassDefault ? 'Apply default' : 'Bypass default'}</a></Button>}
      {(filtered || includeExpunged) && <Button asChild variant="ghost" color="gray"><a href={clearHref}>Clear all filters</a></Button>}
      <Flex align="center" gap="2">
        {loading && result && <Spinner aria-label="Loading results" />}
        <DropdownMenu.Root>
          <DropdownMenu.Trigger><Button variant="soft" color="gray">{includeExpunged ? 'Options · 1' : 'Options'}<DotsHorizontalIcon /></Button></DropdownMenu.Trigger>
          <DropdownMenu.Content align="end">
            <DropdownMenu.CheckboxItem checked={includeExpunged} onCheckedChange={(checked) => { window.location.hash = pandaListingLink(search, '', checked, categories, bypassDefault) }}>Include expunged</DropdownMenu.CheckboxItem>
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
        {cursor && <Button asChild variant="soft" color="gray" size="1"><a href={pageHref(search)}>Newest galleries</a></Button>}
      </Flex>
    </Callout.Root>}

    <div className="gallery-viewport" ref={viewportRef} aria-busy={loading}>
      <div className="gallery-grid gallery-sizing" aria-hidden="true"><div className="gallery-card panda-card" ref={cardRef}><div className="gallery-cover" /><div className="gallery-title"><Text asChild size="1" weight="medium"><h2 /></Text></div><Text as="p" size="1">0 pages · 12/12/2026</Text></div></div>
      {!result && loading && !error && <Text as="p" size="2" color="gray" role="status">Loading Panda galleries…</Text>}
      {result && (result.items.length === 0 ? <Flex className="gallery-empty" direction="column" align="center" justify="center" gap="3">
        <Heading as="h2" size="4">{cursor ? 'No more matching Panda galleries.' : filtered ? 'No matching Panda galleries.' : 'No collected Panda galleries yet.'}</Heading>
        <Text as="p" size="2" color="gray">{cursor ? 'Browse back or return to the newest galleries.' : filtered ? 'Try different titles, tags, or categories, or bypass the default filter.' : 'Collect a feed to discover galleries, then return when metadata is available.'}</Text>
        <Button asChild variant="soft"><a href={cursor ? pageHref(search) : filtered ? clearHref : '#/collector/feeds'}>{cursor ? 'Newest galleries' : filtered ? 'Show all Panda galleries' : 'Collect feeds'}</a></Button>
      </Flex> : <ul className="gallery-grid" aria-label="Panda galleries">{result.items.map((gallery) => <li key={gallery.gallery_id}><PandaCard gallery={gallery} /></li>)}</ul>)}
    </div>
    <div className="gallery-pagination">
      <Text size="1" color="gray" className="gallery-sort">Upload date · Newest first</Text>
      <nav className="pagination" aria-label="Panda gallery pages">
        {result?.previous_cursor && !loading && !error
          ? <Button asChild variant="soft" color="gray"><a href={pageHref(search, result.previous_cursor)} rel="prev">Previous</a></Button>
          : <Button variant="soft" color="gray" disabled>Previous</Button>}
        {result?.next_cursor && !loading && !error
          ? <Button asChild variant="soft" color="gray"><a href={pageHref(search, result.next_cursor)} rel="next">Next</a></Button>
          : <Button variant="soft" color="gray" disabled>Next</Button>}
      </nav>
    </div>
  </section>
}
