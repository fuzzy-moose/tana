import { useState } from 'react'
import { Button, Flex, TextField } from '@radix-ui/themes'
import { MagnifyingGlassIcon } from '@radix-ui/react-icons'
import { listingLink } from '../navigation'
import { completeGallerySearch } from './api'
import TagCompletionField from './TagCompletionField'

interface GallerySearchProps {
  search: string
  completeSearch?: typeof completeGallerySearch
  searchHref?: (query: string) => string
}

export default function GallerySearch({ search, completeSearch = completeGallerySearch, searchHref = listingLink }: GallerySearchProps) {
  const [query, setQuery] = useState(search)
  const [composing, setComposing] = useState(false)

  return (
    <form className="gallery-search" role="search"
      onCompositionStart={() => setComposing(true)} onCompositionEnd={() => setComposing(false)}
      onSubmit={(event) => {
        event.preventDefault()
        if (composing) return
        window.location.hash = searchHref(query.trim())
      }}>
      <Flex gap="2" align="center">
        <TagCompletionField id="gallery-search" type="search" aria-label="Search titles and tags"
          placeholder="Title, tag, or artist:name…" value={query} onChange={setQuery} complete={completeSearch}>
          <TextField.Slot><MagnifyingGlassIcon /></TextField.Slot>
        </TagCompletionField>
        <Button type="submit" variant="soft">Search</Button>
      </Flex>
    </form>
  )
}
