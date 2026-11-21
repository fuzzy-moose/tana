import type { ComponentProps } from 'react'
import { Badge, VisuallyHidden } from '@radix-ui/themes'
import { pandaCategories } from './categories'

type BadgeColor = NonNullable<ComponentProps<typeof Badge>['color']>

/** One fixed colour per Panda gallery category. Solid badges keep them apart from the soft tag badges. */
const categoryColors: Record<string, BadgeColor> = {
  Doujinshi: 'red', Manga: 'orange', 'Artist CG': 'yellow', 'Game CG': 'grass', Western: 'lime',
  'Non-H': 'blue', 'Image Set': 'indigo', Cosplay: 'purple', 'Asian Porn': 'pink', Misc: 'gray',
}

/** The Panda gallery category of a card; nothing for an empty or unknown category. Categories arrive normalized to lowercase. */
export default function PandaCategoryBadge({ category }: { category?: string }) {
  const name = pandaCategories.find((known) => known.toLowerCase() === category)
  if (!name) return null
  return <Badge variant="solid" color={categoryColors[name]}><VisuallyHidden>Category: </VisuallyHidden>{name}</Badge>
}
