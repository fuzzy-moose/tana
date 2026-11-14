import { useCallback, useEffect, useState } from 'react'
import type { CSSProperties, MouseEvent } from 'react'
import { AlertDialog, Button, Callout, DropdownMenu, Flex, Heading, IconButton, SegmentedControl, Spinner, Text, Theme } from '@radix-ui/themes'
import { Cross2Icon, DotsHorizontalIcon, ExclamationTriangleIcon, ReloadIcon } from '@radix-ui/react-icons'
import { useMutation } from '../collector/useMutation'
import GallerySearch from '../galleries/GallerySearch'
import { useGalleryGrid } from '../galleries/useGalleryLayout'
import { isVerdictView, pandaListingLink } from '../navigation'
import type { PandaListing, PandaView } from '../navigation'
import '../pagination/Pagination.css'
import { catchUpPanda, completePandaSearch, getPandaDefaultFilter, hasDefaultFilter, listPandaCatalog, recordPandaDisplays, savePandaVerdict, verdictLabels } from './api'
import type { PandaCatalogResult, PandaDefaultFilter as DefaultFilter, PandaGallery, PandaVerdict } from './api'
import PandaCard from './PandaCard'
import PandaCategoryFilter from './PandaCategoryFilter'
import PandaDefaultFilter from './PandaDefaultFilter'
import '../galleries/Galleries.css'
import './PandaCatalog.css'

const sizingGallery: PandaGallery = {
  gallery_id: 0, title: '', thumbnail_url: '', page_count: null, posted_at: null, metadata_source: 'collected', url: '', summary_tags: [],
}

export default function PandaCatalog({ search, cursor, includeExpunged, categories, bypassDefault, view }: PandaListing) {
  // Built during render: never an effect dependency, or every render would refetch.
  const listing: PandaListing = { search, cursor, includeExpunged, categories, bypassDefault, view }
  const [result, setResult] = useState<PandaCatalogResult | null>(null)
  const [requestState, setRequestState] = useState<{ key: string, error: string } | null>(null)
  const [attempt, setAttempt] = useState(0)
  const [defaultFilter, setDefaultFilter] = useState<DefaultFilter | null>(null)
  const [defaultError, setDefaultError] = useState('')
  const [recordError, setRecordError] = useState('')
  const [catchUpOpen, setCatchUpOpen] = useState(false)
  const [verdictError, setVerdictError] = useState<{ galleryID: number, title: string, message: string } | null>(null)
  const catchUp = useMutation()
  const pageHref = useCallback((query: string, position = '') => pandaListingLink({ search: query, cursor: position, includeExpunged, categories, bypassDefault, view }), [includeExpunged, categories, bypassDefault, view])
  const { viewportRef, cardRef, pageSize, cardHeight } = useGalleryGrid()
  const requestKey = JSON.stringify([search, cursor, pageSize, includeExpunged, categories, bypassDefault, view, attempt])
  const loading = requestState?.key !== requestKey
  // Keep the alert's height stable while retrying: it affects measured page capacity.
  const error = requestState?.error ?? ''
  const defaultConfigured = hasDefaultFilter(defaultFilter)
  const filtered = !!search || categories.length > 0 || (defaultConfigured && !bypassDefault)
  const clearHref = pandaListingLink({ bypassDefault: true, view })
  const allHref = pandaListingLink({ search, includeExpunged, categories, bypassDefault })
  const verdictView = isVerdictView(view)
  const catchUpDisabled = view !== 'unseen' || loading || !!error || !result?.items.length
  const newest = result?.items[0]
  const newestDate = newest?.posted_at ?? newest?.first_seen_at
  const confirmCatchUp = () => {
    if (catchUpDisabled || !newest) return
    // Captured now: the hash may change (browser Back) while the request is pending.
    const newestUnseen = pageHref(search)
    void catchUp.run((signal) => catchUpPanda(newest.gallery_id, signal), {
      onSuccess: () => {
        setCatchUpOpen(false)
        if (window.location.hash !== newestUnseen) window.location.hash = newestUnseen
        else setAttempt((value) => value + 1)
      },
    })
  }
  const goNext = useCallback(() => {
    if (!result?.next_cursor) return
    // Records settle in any order; each one sets or clears the alert as it lands.
    const ids = result.items.map((gallery) => gallery.gallery_id)
    if (ids.length) void recordPandaDisplays(ids).then(() => setRecordError(''), (error: Error) => setRecordError(error.message))
    window.location.hash = pageHref(search, result.next_cursor)
  }, [result, pageHref, search])
  const clickNext = (event: MouseEvent<HTMLAnchorElement>) => {
    if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return
    event.preventDefault()
    goNext()
  }

  // A save patches the result that is current when it settles and never loads the listing again:
  // a new load would drop a judged card from the unseen view, and the request key must stay put.
  const saveVerdict = useCallback(async ({ gallery_id: galleryID, title }: PandaGallery, verdict: PandaVerdict | null) => {
    try {
      await savePandaVerdict(galleryID, verdict)
    } catch (error) {
      setVerdictError({ galleryID, title, message: (error as Error).message })
      return
    }
    setResult((current) => current && { ...current, items: current.items.map((item) => item.gallery_id !== galleryID ? item
      : { ...item, verdict: verdict ?? undefined, displayed: verdict ? true : item.displayed }) })
    setVerdictError((current) => current?.galleryID === galleryID ? null : current)
  }, [])

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
    void listPandaCatalog({ search, cursor, includeExpunged, categories, bypassDefault, view }, pageSize, controller.signal).then((next) => {
      if (controller.signal.aborted) return
      setResult(next)
      setRequestState({ key: requestKey, error: '' })
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setRequestState({ key: requestKey, error: error.message })
    })
    return () => controller.abort()
  }, [search, cursor, pageSize, includeExpunged, categories, bypassDefault, view, requestKey, pageHref])

  useEffect(() => {
    if (!result || loading || error) return
    const navigate = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.isComposing || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return
      if (event.target instanceof HTMLElement && (event.target.isContentEditable || event.target.closest('input, textarea, select, [role="menu"], [role="dialog"], [role="alertdialog"], [contenteditable]:not([contenteditable="false"])'))) return
      const direction = event.key === 'ArrowLeft' || event.key === 'a' ? -1 : event.key === 'ArrowRight' || event.key === 'd' ? 1 : 0
      if (!direction) return
      event.preventDefault()
      if (direction > 0) goNext()
      else if (result.previous_cursor) window.location.hash = pageHref(search, result.previous_cursor)
    }
    window.addEventListener('keydown', navigate)
    return () => window.removeEventListener('keydown', navigate)
  }, [result, loading, error, pageHref, search, goNext])

  return <section className="gallery-listing gallery-boxes panda-catalog" aria-labelledby="panda-title" style={{ '--gallery-card-height': `${cardHeight}px` } as CSSProperties}>
    <div className="gallery-toolbar">
      <Flex className="panda-catalog-heading" align="center" gap="3" wrap="wrap">
        <Heading as="h1" size="5" id="panda-title">Panda</Heading>
        <SegmentedControl.Root size="1" aria-label="View" value={view} onValueChange={(value) => { window.location.hash = pandaListingLink({ ...listing, cursor: '', view: value as PandaView }) }}>
          <SegmentedControl.Item value="all">All</SegmentedControl.Item>
          <SegmentedControl.Item value="unseen">Unseen</SegmentedControl.Item>
          <SegmentedControl.Item value="interesting">Interesting</SegmentedControl.Item>
          <SegmentedControl.Item value="not_for_me">Not for me</SegmentedControl.Item>
        </SegmentedControl.Root>
      </Flex>
      <GallerySearch key={search} search={search} completeSearch={completePandaSearch} searchHref={pageHref} />
      <PandaCategoryFilter categories={categories} onChange={(next) => { window.location.hash = pandaListingLink({ ...listing, cursor: '', categories: next }) }} />
      <PandaDefaultFilter filter={defaultFilter} error={defaultError} bypassed={bypassDefault} onRetry={() => setAttempt((value) => value + 1)} onSave={(filter) => {
        setDefaultFilter(filter)
        if (cursor || bypassDefault) window.location.hash = pandaListingLink({ ...listing, cursor: '', bypassDefault: false })
        else setAttempt((value) => value + 1)
      }} />
      {(defaultConfigured || bypassDefault) && <Button asChild variant="ghost" color="gray"><a href={pandaListingLink({ ...listing, cursor: '', bypassDefault: !bypassDefault })}>{bypassDefault ? 'Apply default' : 'Bypass default'}</a></Button>}
      {(filtered || includeExpunged) && <Button asChild variant="ghost" color="gray"><a href={clearHref}>Clear all filters</a></Button>}
      {view === 'unseen' && <Button variant="soft" disabled={catchUpDisabled} onClick={() => { catchUp.clear(); setCatchUpOpen(true) }}>Catch up…</Button>}
      <Flex align="center" gap="2">
        {loading && result && <Spinner aria-label="Loading results" />}
        <DropdownMenu.Root>
          <DropdownMenu.Trigger><Button variant="soft" color="gray">{includeExpunged ? 'Options · 1' : 'Options'}<DotsHorizontalIcon /></Button></DropdownMenu.Trigger>
          <DropdownMenu.Content align="end">
            <DropdownMenu.CheckboxItem checked={includeExpunged} onCheckedChange={(checked) => { window.location.hash = pandaListingLink({ ...listing, cursor: '', includeExpunged: checked }) }}>Include expunged</DropdownMenu.CheckboxItem>
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

    {recordError && <Callout.Root size="1" color="red" role="alert">
      <Callout.Icon><ExclamationTriangleIcon /></Callout.Icon>
      <Callout.Text>Could not mark a page as seen. Its galleries stay unseen. {recordError}</Callout.Text>
    </Callout.Root>}

    <div className="gallery-viewport" ref={viewportRef} aria-busy={loading}>
      <Theme asChild radius="none"><div className="gallery-grid gallery-sizing" aria-hidden="true">
        <div ref={cardRef} inert><PandaCard gallery={sizingGallery} /></div>
      </div></Theme>
      {!result && loading && !error && <Text as="p" size="2" color="gray" role="status">Loading Panda galleries…</Text>}
      {result && (result.items.length === 0 ? <Flex className="gallery-empty" direction="column" align="center" justify="center" gap="3">
        {view === 'unseen' ? <>
          <Heading as="h2" size="4">{cursor ? (filtered ? 'No more unseen Panda galleries match.' : 'No more unseen Panda galleries.') : filtered ? 'No unseen Panda galleries match.' : 'No unseen Panda galleries.'}</Heading>
          <Text as="p" size="2" color="gray">{cursor ? 'Galleries that arrived during this visit wait with the newest unseen galleries.' : filtered ? 'Try different titles, tags, or categories, or show all galleries.' : 'You have seen every Panda gallery. New galleries appear here once feeds are collected.'}</Text>
          <Flex gap="3" wrap="wrap" justify="center">
            {cursor && <Button asChild variant="soft"><a href={pageHref(search)}>Newest unseen galleries</a></Button>}
            <Button asChild variant="soft" color={cursor ? 'gray' : undefined}><a href={allHref}>Show all galleries</a></Button>
          </Flex>
        </> : verdictView && !cursor ? <>
          <Heading as="h2" size="4">{filtered ? 'No Panda gallery with this verdict matches.' : 'No Panda gallery has this verdict.'}</Heading>
          <Text as="p" size="2" color="gray">{filtered ? 'Try different titles, tags, or categories, or clear all filters.' : `Mark a gallery as ${verdictLabels[view]} on its card, and it appears here.`}</Text>
          <Button asChild variant="soft"><a href={filtered ? clearHref : allHref}>{filtered ? 'Clear all filters' : 'Show all galleries'}</a></Button>
        </> : <>
          <Heading as="h2" size="4">{cursor ? 'No more matching Panda galleries.' : filtered ? 'No matching Panda galleries.' : 'No Panda galleries yet.'}</Heading>
          <Text as="p" size="2" color="gray">{cursor ? 'Browse back or return to the newest galleries.' : filtered ? 'Try different titles, tags, or categories, or bypass the default filter.' : 'Collect a feed to discover galleries and browse their feed metadata.'}</Text>
          <Button asChild variant="soft"><a href={cursor ? pageHref(search) : filtered ? clearHref : '#/collector/feeds'}>{cursor ? 'Newest galleries' : filtered ? 'Show all Panda galleries' : 'Collect feeds'}</a></Button>
        </>}
      </Flex> : <Theme asChild radius="none"><ul className="gallery-grid" aria-label="Panda galleries">
        {result.items.map((gallery) => <li key={gallery.gallery_id}><PandaCard gallery={gallery} onVerdict={saveVerdict} subdued={verdictView ? false : undefined} /></li>)}
      </ul></Theme>)}
    </div>
    <div className="gallery-pagination">
      <Text size="1" color="gray" className="gallery-sort">Upload date (first seen if unknown) · Newest first</Text>
      <nav className="pagination" aria-label="Panda gallery pages">
        {result?.previous_cursor && !loading && !error
          ? <Button asChild variant="soft" color="gray"><a href={pageHref(search, result.previous_cursor)} rel="prev">Previous</a></Button>
          : <Button variant="soft" color="gray" disabled>Previous</Button>}
        {result?.next_cursor && !loading && !error
          ? <Button asChild variant="soft" color="gray"><a href={pageHref(search, result.next_cursor)} rel="next" onClick={clickNext}>Next</a></Button>
          : <Button variant="soft" color="gray" disabled>Next</Button>}
      </nav>
      {/* Out of the page flow: an in-flow alert would change the page capacity and load the page again. */}
      {verdictError && <div className="panda-verdict-error">
        <Callout.Root size="1" color="red" role="alert">
          <Callout.Icon><ExclamationTriangleIcon /></Callout.Icon>
          <Flex align="start" gap="2" minWidth="0">
            <Callout.Text className="panda-verdict-error-text">
              Could not save the verdict for <span className="panda-verdict-error-title">{verdictError.title}.</span> {verdictError.message}
            </Callout.Text>
            <IconButton size="1" variant="ghost" color="red" aria-label="Dismiss" title="Dismiss" onClick={() => setVerdictError(null)}><Cross2Icon /></IconButton>
          </Flex>
        </Callout.Root>
      </div>}
    </div>

    <AlertDialog.Root open={catchUpOpen} onOpenChange={(open) => { if (!open && !catchUp.pending) setCatchUpOpen(false) }}>
      <AlertDialog.Content maxWidth="460px">
        <AlertDialog.Title>Catch up?</AlertDialog.Title>
        <AlertDialog.Description size="2">This page and every older Panda gallery, back from {newestDate ? new Date(newestDate).toLocaleString() : 'the newest gallery on this page'}, become seen, including galleries that the current search and filters hide. Newer galleries keep their state. This can't be undone.</AlertDialog.Description>
        {catchUp.error && <Text as="p" size="2" color="red" mt="3" role="alert">Catch-up failed. Nothing was marked as seen. {catchUp.error}</Text>}
        <Flex gap="3" mt="4" justify="end">
          <AlertDialog.Cancel><Button size="2" variant="soft" color="gray" disabled={catchUp.pending}>Cancel</Button></AlertDialog.Cancel>
          <Button size="2" variant="solid" disabled={catchUp.pending || catchUpDisabled} onClick={confirmCatchUp}>Catch up</Button>
        </Flex>
      </AlertDialog.Content>
    </AlertDialog.Root>
  </section>
}
