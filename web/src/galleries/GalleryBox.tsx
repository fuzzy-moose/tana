import { useLayoutEffect, useRef, useState } from 'react'
import type { ComponentProps, ReactEventHandler, ReactNode } from 'react'
import { Badge, Box, Flex, Text, Tooltip, VisuallyHidden } from '@radix-ui/themes'
import type { TagSuggestion } from './api'

function SummaryTags({ tags }: { tags: TagSuggestion[] }) {
  const listRef = useRef<HTMLUListElement>(null)
  const [shown, setShown] = useState(tags.length)

  // Drop trailing tags until the remaining ones fit the fixed strip.
  useLayoutEffect(() => {
    const bottom = listRef.current!.getBoundingClientRect().bottom
    const overflows = [...listRef.current!.children].some((item) => item.getBoundingClientRect().bottom > bottom)
    if (overflows && shown > 0) setShown(shown - 1)
  }, [shown])

  useLayoutEffect(() => {
    const observer = new ResizeObserver(() => setShown(tags.length))
    observer.observe(listRef.current!)
    return () => observer.disconnect()
  }, [tags])

  return <Flex asChild wrap="wrap" gap="1" m="0" p="0"><ul className="gallery-summary-tags" aria-label="Tags" ref={listRef}>
    {tags.slice(0, shown).map(({ term, value }) => <li key={term}><Badge color="gray"><Text truncate>{value}</Text></Badge></li>)}
  </ul></Flex>
}

export interface CoverImageProps {
  className?: string
  onLoad: ReactEventHandler<HTMLImageElement>
}

interface GalleryBoxProps extends Pick<ComponentProps<'a'>, 'href' | 'target' | 'rel'> {
  title: string
  cover: (image: CoverImageProps) => ReactNode
  pageCount: number | null
  tags: TagSuggestion[]
  children?: ReactNode
}

/** One cell of a gallery grid: title, cover with its page count, and the tags that fit. */
export default function GalleryBox({ title, cover, pageCount, tags, children, ...link }: GalleryBoxProps) {
  const [coverClass, setCoverClass] = useState<string>()
  // A loaded cover takes its own shape inside the 2:3 slot, so its border follows the image.
  const onCoverLoad: ReactEventHandler<HTMLImageElement> = ({ currentTarget: image }) => {
    setCoverClass(image.naturalWidth * 3 > image.naturalHeight * 2 ? 'gallery-cover-wide' : 'gallery-cover-tall')
  }
  return <Tooltip content={title}><Box asChild p="3"><a className="gallery-box" {...link}>
    <Flex direction="column" gap="2">
      <Text asChild size="1" weight="medium"><h2>{title}</h2></Text>
      <div className="gallery-cover">
        {cover({ className: coverClass, onLoad: onCoverLoad })}
        {pageCount !== null && pageCount > 0 && <Badge className="gallery-page-count" variant="solid" color="gray">
          {pageCount}<VisuallyHidden>{pageCount === 1 ? ' page' : ' pages'}</VisuallyHidden>
        </Badge>}
      </div>
      <SummaryTags key={tags.map((tag) => tag.term).join(' ')} tags={tags} />
      {children}
    </Flex>
  </a></Box></Tooltip>
}
