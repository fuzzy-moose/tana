import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Badge, Button, Flex, Text, TextField } from '@radix-ui/themes'
import { MagnifyingGlassIcon } from '@radix-ui/react-icons'
import { listingLink } from '../navigation'
import { completeGallerySearch } from './api'
import type { SearchCompletion, TagSuggestion } from './api'

interface GallerySearchProps {
  search: string
  completeSearch?: typeof completeGallerySearch
  searchHref?: (query: string) => string
}

export default function GallerySearch({ search, completeSearch = completeGallerySearch, searchHref = listingLink }: GallerySearchProps) {
  const [query, setQuery] = useState(search)
  const [cursor, setCursor] = useState(search.length)
  const [focused, setFocused] = useState(false)
  const [dismissed, setDismissed] = useState(false)
  const [composing, setComposing] = useState(false)
  const [response, setResponse] = useState<{ query: string, cursor: number, result: SearchCompletion } | null>(null)
  const [active, setActive] = useState(0)
  const input = useRef<HTMLInputElement>(null)
  const pendingCursor = useRef<number | null>(null)
  const completion = response?.query === query && response.cursor === cursor ? response.result : null
  const open = focused && !dismissed && !composing && !!completion?.items.length

  useEffect(() => {
    if (!focused || dismissed || composing || !query.trim() || cursor === 0) return
    const controller = new AbortController()
    const timer = setTimeout(async () => {
      try {
        const result = await completeSearch(query, cursor, controller.signal)
        if (!controller.signal.aborted) { setResponse({ query, cursor, result }); setActive(0) }
      } catch {
        // Completion is optional; a failed request must not interrupt typing.
      }
    }, 150)
    return () => { controller.abort(); clearTimeout(timer) }
  }, [query, cursor, focused, dismissed, composing, completeSearch])

  useLayoutEffect(() => {
    if (pendingCursor.current === null) return
    input.current?.focus()
    input.current?.setSelectionRange(pendingCursor.current, pendingCursor.current)
    pendingCursor.current = null
  }, [query])

  useEffect(() => {
    if (open) document.getElementById(`gallery-tag-${active}`)?.scrollIntoView?.({ block: 'nearest' })
  }, [active, open])

  function select(suggestion: TagSuggestion) {
    if (!completion) return
    const next = query.slice(0, completion.start) + suggestion.term + query.slice(completion.end)
    const position = completion.start + suggestion.term.length
    pendingCursor.current = position
    setQuery(next)
    setCursor(position)
    setDismissed(true)
    setResponse(null)
  }

  return (
    <form className="gallery-search" role="search" onSubmit={(event) => {
      event.preventDefault()
      if (composing) return
      setDismissed(true)
      window.location.hash = searchHref(query.trim())
    }}>
      <Flex gap="2" align="center">
        <div className="gallery-search-input">
          <TextField.Root ref={input} id="gallery-search" type="search" role="combobox" aria-label="Search titles and tags"
            aria-autocomplete="list" aria-expanded={open} aria-controls={open ? 'gallery-tags' : undefined}
            aria-activedescendant={open ? `gallery-tag-${active}` : undefined} autoComplete="off"
            value={query} placeholder="Title, tag, or artist:name…"
            onChange={(event) => {
              setQuery(event.target.value)
              setCursor(event.target.selectionStart ?? event.target.value.length)
              setDismissed(false)
              setResponse(null)
            }}
            onSelect={(event) => { setCursor(event.currentTarget.selectionStart ?? query.length) }}
            onFocus={() => setFocused(true)} onBlur={() => { setFocused(false); setResponse(null) }}
            onClick={() => setDismissed(false)}
            onCompositionStart={() => setComposing(true)} onCompositionEnd={() => setComposing(false)}
            onKeyDown={(event) => {
              if (event.nativeEvent.isComposing || composing) return
              if (event.key === 'Escape') {
                event.preventDefault()
                setDismissed(true)
              } else if (open && completion) {
                if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
                  event.preventDefault()
                  setActive((index) => (index + (event.key === 'ArrowDown' ? 1 : -1) + completion.items.length) % completion.items.length)
                } else if (event.key === 'Enter') {
                  event.preventDefault()
                  select(completion.items[active])
                }
              }
            }}>
            <TextField.Slot><MagnifyingGlassIcon /></TextField.Slot>
          </TextField.Root>
          {open && <ul id="gallery-tags" className="gallery-tag-suggestions" role="listbox" aria-label="Tag suggestions">
            {completion.items.map((suggestion, index) => <li key={`${suggestion.namespace}:${suggestion.value}`}
              id={`gallery-tag-${index}`} role="option" aria-selected={index === active}
              onMouseDown={(event) => event.preventDefault()} onMouseEnter={() => setActive(index)}
              onClick={() => select(suggestion)}>
              <Badge color="gray" size="1">{suggestion.namespace}</Badge><Text size="2">{suggestion.value}</Text>
            </li>)}
          </ul>}
        </div>
        <Button type="submit" variant="soft">Search</Button>
      </Flex>
    </form>
  )
}
