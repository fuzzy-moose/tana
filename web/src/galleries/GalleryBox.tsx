import { useLayoutEffect, useRef, useState } from 'react'
import type { ComponentProps, ReactEventHandler, ReactNode } from 'react'
import { Badge, Box, Flex, Text, Tooltip, VisuallyHidden } from '@radix-ui/themes'
import type { TagSuggestion } from './api'

type BadgeColor = NonNullable<ComponentProps<typeof Badge>['color']>

export interface GalleryBoxTag extends TagSuggestion {
  /** Badge colour; gray without one. */
  color?: BadgeColor
  /** Shown inside the badge, before the value. */
  mark?: ReactNode
}

function SummaryTags({ tags }: { tags: GalleryBoxTag[] }) {
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
  }, [tags.length])

  return <Flex asChild wrap="wrap" gap="1" m="0" p="0"><ul className="gallery-summary-tags" aria-label="Tags" ref={listRef}>
    {tags.slice(0, shown).map(({ term, value, color = 'gray', mark }) => <li key={term}><Badge color={color}>{mark}<Text truncate>{value}</Text></Badge></li>)}
  </ul></Flex>
}

export interface CoverImageProps {
  onLoad: ReactEventHandler<HTMLImageElement>
}

interface GalleryBoxProps extends Pick<ComponentProps<'a'>, 'href' | 'target' | 'rel'> {
  title: string
  cover: (image: CoverImageProps) => ReactNode
  pageCount: number | null
  tags: GalleryBoxTag[]
  /** Hover details; the title alone by default. */
  tooltip?: ReactNode
  /** Small badges pinned to the cover's bottom-left corner. */
  status?: ReactNode
  /** Dims the title and cover, for a gallery the user has already seen. */
  subdued?: boolean
}

/** One cell of a gallery grid: title, cover with its page count and status, and the tags that fit. */
export default function GalleryBox({ title, cover, pageCount, tags, tooltip = title, status, subdued, ...link }: GalleryBoxProps) {
  const [coverRatio, setCoverRatio] = useState<number>()
  // A loaded cover shapes its frame inside the 2:3 slot, so the border and page count follow the image.
  const onCoverLoad: ReactEventHandler<HTMLImageElement> = ({ currentTarget: image }) => {
    setCoverRatio(image.naturalWidth / image.naturalHeight)
  }
  const coverShape = coverRatio === undefined ? '' : coverRatio > 2 / 3 ? ' gallery-cover-wide' : ' gallery-cover-tall'
  return <Tooltip content={tooltip}><Box asChild p="3"><a className={subdued ? 'gallery-box gallery-box-subdued' : 'gallery-box'} {...link}>
    <Flex direction="column" gap="2">
      <Text asChild size="1" weight="medium"><h2>{title}</h2></Text>
      {subdued && <VisuallyHidden>Seen</VisuallyHidden>}
      <div className="gallery-cover"><div className={`gallery-cover-frame${coverShape}`} style={{ aspectRatio: coverRatio }}>
        {cover({ onLoad: onCoverLoad })}
        {status && <Flex className="gallery-cover-status" gap="1">{status}</Flex>}
        {pageCount !== null && pageCount > 0 && <Badge className="gallery-page-count" variant="solid" color="gray">
          {pageCount}<VisuallyHidden>{pageCount === 1 ? ' page' : ' pages'}</VisuallyHidden>
        </Badge>}
      </div></div>
      {/* A mark widens its badge, and a mounted strip fits again only on a resize. */}
      <SummaryTags key={tags.map((tag) => `${tag.mark ? '+' : '-'}${tag.term}`).join(' ')} tags={tags} />
    </Flex>
  </a></Box></Tooltip>
}
