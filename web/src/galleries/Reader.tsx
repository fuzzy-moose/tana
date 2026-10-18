import { useEffect, useState } from 'react'
import { Badge, Button, Callout, Flex, Heading, Text } from '@radix-ui/themes'
import { galleryLink, readerSpreadLink } from '../navigation'
import type { Gallery } from './api'
import { useGallery } from './useGallery'
import { useReaderImages } from './useReaderImages'
import { previousSpread, spreadPages } from './spreads'
import ReaderProgress from './ReaderProgress'
import './Reader.css'

export default function Reader({ id, initialPage, initialLastPage }: { id: number, initialPage: number, initialLastPage?: number }) {
  const { gallery, error, retry } = useGallery(id)
  useEffect(() => {
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') window.location.hash = galleryLink(id)
    }
    window.addEventListener('keydown', escape)
    return () => window.removeEventListener('keydown', escape)
  }, [id])

  if (gallery && gallery.page_count > 0) return <ReadingGallery gallery={gallery} initialPage={initialPage} initialLastPage={initialLastPage} />
  return <Flex asChild direction="column" align="center" justify="center" gap="4">
    <main className="reader">
      <Button asChild variant="soft" color="gray"><a href={galleryLink(id)}>← Gallery detail</a></Button>
      {error ? <Callout.Root color="red" role="alert"><Callout.Text>{error}</Callout.Text><Button variant="soft" color="red" onClick={retry}>Retry</Button></Callout.Root>
        : <Text as="p" size="2" color="gray" role="status">{gallery ? 'This gallery has no pages to read.' : 'Loading gallery…'}</Text>}
    </main>
  </Flex>
}

function ReadingGallery({ gallery, initialPage, initialLastPage }: { gallery: Gallery, initialPage: number, initialLastPage?: number }) {
  const total = gallery.page_count
  const [start, setStart] = useState(Math.max(1, Math.min(initialPage, total)))
  const [controlsVisible, setControlsVisible] = useState(false)
  // Keep the selected pairing when reversing through the cover boundary.
  // A one-page turn starts a fresh pairing; normal turns retain spread starts.
  const [boundaries, setBoundaries] = useState(() => new Set([1, start, total + 1, ...(initialLastPage === start ? [start + 1] : [])]))
  const { images, retry } = useReaderImages(gallery.id, start, total)
  const shape = (page: number) => images.get(page)?.shape
  const pages = boundaries.has(start + 1) ? [start] : spreadPages(start, total, shape)
  const ready = images.has(start) && (start === 1 || start === total || shape(start) === 'wide' || images.has(start + 1))
  const previousReady = start <= 2 || (images.has(start - 1) && (start === 3 || images.has(start - 2)))
  const lastPage = pages[pages.length - 1]
  const atEnd = lastPage === total

  useEffect(() => {
    if (!ready) return
    const hash = readerSpreadLink(gallery.id, start, lastPage)
    // Replacing the entry preserves Back/Forward without triggering a reader remount.
    if (window.location.hash !== hash) window.history.replaceState(window.history.state, '', hash)
  }, [gallery.id, start, lastPage, ready])

  function selectPage(page: number) {
    if (page === start) return
    setStart(page)
    setBoundaries(new Set([1, page, total + 1]))
  }

  function move(direction: 1 | -1, single = false) {
    if (single) {
      selectPage(Math.max(1, Math.min(total, start + direction)))
    } else if (direction === 1 && ready && !atEnd) {
      const next = start + pages.length
      setStart(next)
      setBoundaries((current) => new Set([...current, next]))
    } else if (direction === -1 && previousReady) {
      const next = boundaries.has(start - 1) ? start - 1 : previousSpread(start, shape)
      setStart(next)
      setBoundaries((current) => new Set([...current, next]))
    }
  }

  useEffect(() => {
    const navigate = (event: KeyboardEvent) => {
      if (event.altKey || event.ctrlKey || event.metaKey || (event.target instanceof HTMLElement && event.target.matches('input, textarea, select, [contenteditable="true"]'))) return
      const key = event.key.toLowerCase()
      if (key === 'arrowleft' || key === 'arrowright' || key === 'a' || key === 'd') {
        event.preventDefault()
        move(key === 'arrowleft' || key === 'a' ? 1 : -1, event.shiftKey)
      }
    }
    window.addEventListener('keydown', navigate)
    return () => window.removeEventListener('keydown', navigate)
  })

  return <main className="reader" aria-label={`Reading ${gallery.title}`} data-controls-visible={controlsVisible}>
    <Flex asChild align="center" gap={{ initial: '3', sm: '5' }} px={{ initial: '2', sm: '5' }} py="2">
      <header className="reader-header" id="reader-header">
        <Button asChild variant="soft" color="gray"><a href={galleryLink(gallery.id)}>← Gallery detail</a></Button>
        <Heading as="h1" size="2" truncate className="reader-title">{gallery.title}</Heading>
        <Badge color="gray" className="reader-direction">Right to left</Badge>
      </header>
    </Flex>
    <div className="reader-stage" aria-label="Reading spread" aria-busy={!ready}>
      {ready ? <div className={`reader-spread ${pages.length === 1 ? 'reader-single' : ''}`}>
        {pages.map((page) => <div className="reader-page" key={page}>
          {images.get(page)?.shape === 'error' ? <Flex direction="column" align="center" gap="3" p="5" className="reader-page-error" role="group" aria-label={`Page ${page} unavailable`}>
            <Text as="p" size="2" color="gray">Page {page} unavailable</Text>
            <Button variant="soft" color="gray" onClick={() => retry(page)}>Retry page {page}</Button>
          </Flex> : <img src={images.get(page)?.url} alt={`Page ${page}`} draggable={false} />}
        </div>)}
      </div> : <Text as="p" size="2" color="gray" className="reader-loading" role="status">Loading pages…</Text>}
      <button className="reader-side reader-side-left" aria-label="Next spread" tabIndex={-1} disabled={!ready || atEnd} onClick={(event) => move(1, event.shiftKey)} />
      <button className="reader-side reader-side-right" aria-label="Previous spread" tabIndex={-1} disabled={start === 1 || !previousReady} onClick={(event) => move(-1, event.shiftKey)} />
      <button className="reader-toggle" aria-label="Toggle reader controls" aria-expanded={controlsVisible} aria-controls="reader-header reader-footer" tabIndex={-1} onClick={() => setControlsVisible((visible) => !visible)} />
    </div>
    <div className="reader-bottom">
      <Flex asChild direction="column" gap="2" px="4" py="2">
        <footer className="reader-footer" id="reader-footer">
          <Flex align="center" justify="center" gap={{ initial: '2', sm: '3' }} className="reader-controls">
            <Button variant="soft" title="Left arrow" disabled={!ready || atEnd} onClick={() => move(1)}>← Next</Button>
            <Button variant="soft" color="gray" title="Shift + left arrow" disabled={start === total} onClick={() => move(1, true)}>← 1 page</Button>
            <Text as="p" size="2" role="status" align="center" className="reader-position">{ready ? `Page ${pages.join('–')} of ${total}` : `Page ${start} of ${total}`}{ready && atEnd && <Text as="span" size="1" color="green">End of gallery</Text>}</Text>
            <Button variant="soft" color="gray" title="Shift + right arrow" disabled={start === 1} onClick={() => move(-1, true)}>1 page →</Button>
            <Button variant="soft" title="Right arrow" disabled={start === 1 || !previousReady} onClick={() => move(-1)}>Previous →</Button>
          </Flex>
          <Text as="p" size="1" color="gray" align="center">
            <span className="reader-help-desktop">Click either side or use arrow keys / A / D · Hold Shift to turn one page · Esc returns to gallery</span>
            <span className="reader-help-touch">Tap sides to turn pages · Tap center for controls</span>
          </Text>
        </footer>
      </Flex>
      <ReaderProgress page={start} lastPage={ready ? lastPage : start} total={total} onSelect={selectPage} />
    </div>
  </main>
}
