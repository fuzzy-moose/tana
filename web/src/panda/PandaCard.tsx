import { useState } from 'react'
import { Badge, Flex, Text, VisuallyHidden } from '@radix-ui/themes'
import { ClockIcon, DrawingPinFilledIcon, ExclamationTriangleIcon, EyeNoneIcon, EyeOpenIcon, StarFilledIcon } from '@radix-ui/react-icons'
import GalleryBox from '../galleries/GalleryBox'
import type { GalleryBoxTag } from '../galleries/GalleryBox'
import type { PandaCatalogScoreFilter, PandaGallery, PandaScoreWeight, PandaSummaryTag } from './api'
import { scoreRuleNames } from './score'
import type { ScoreSource } from './score'

const weightPrefixes: Record<PandaScoreWeight['kind'], string> = { tag: '', uploader: 'Uploader ', category: 'Category ' }
const sourceSuffixes: Record<ScoreSource, string> = { pin: ' (pinned)', author_bonus: ' (author bonus)' }

/** The smallest absolute weight that colours a tag by its direction. */
const clearTagWeight = 0.2

/** The colour of a tag whose weight clearly likes or dislikes it. */
function weightColor(weight: number | undefined): GalleryBoxTag['color'] {
  if (weight !== undefined && weight >= clearTagWeight) return 'green'
  if (weight !== undefined && weight <= -clearTagWeight) return 'red'
  return undefined
}

/** Gives a tag at most one mark: its override rule, else a pin, else a favorite author's star. An override colours by its rule, any other tag by its weight; the order stays the catalog page's. */
function stripTag({ weight, override, source, favorite_author: favoriteAuthor, ...tag }: PandaSummaryTag): GalleryBoxTag {
  if (override) return { ...tag, color: override === 'never_hide' ? 'green' : 'red', mark: <>
    {override === 'never_hide' ? <EyeOpenIcon /> : <EyeNoneIcon />}<VisuallyHidden>{scoreRuleNames[override]}: </VisuallyHidden>
  </> }
  const color = weightColor(weight)
  const mark = source === 'pin' ? <><DrawingPinFilledIcon /><VisuallyHidden>Pinned: </VisuallyHidden></>
    : favoriteAuthor ? <><StarFilledIcon /><VisuallyHidden>Favorite author: </VisuallyHidden></>
      : undefined
  return { ...tag, color, mark }
}

/** The rule that decides whether the score filter hides the gallery, as the collector names it. */
function scoreRule({ score }: PandaGallery) {
  switch (score?.rule) {
    case 'never_hide': return `Never hidden: ${score.override}.`
    case 'always_hide': return `Hidden by the override for ${score.override}.`
    case 'below_threshold': return 'Hidden: the score is below the threshold.'
    default: return ''
  }
}

/** Hover details: everything the compact card leaves out. `scoreFilter` is the page's, present while the filter is in force. */
export function PandaDetails({ gallery, scoreFilter }: { gallery: PandaGallery, scoreFilter?: PandaCatalogScoreFilter }) {
  const date = gallery.posted_at || gallery.first_seen_at
  const score = gallery.score
  const rule = scoreFilter ? scoreRule(gallery) : ''
  return <Flex direction="column" gap="1">
    <Text weight="medium">{gallery.title}</Text>
    <Text>{gallery.page_count === null && 'Pages unknown · '}{date
      ? <>{gallery.posted_at ? 'Posted ' : 'First seen '}<time dateTime={date}>{new Date(date).toLocaleString()}</time></>
      : 'Publication unknown'}</Text>
    {gallery.metadata_source === 'feed' && <>
      <Text>Feed metadata · {gallery.metadata_state === 'failed' ? 'Collection failed' : 'Waiting for collection'}</Text>
      {gallery.fetch_eligible_at && <Text>Eligible from <time dateTime={gallery.fetch_eligible_at}>{new Date(gallery.fetch_eligible_at).toLocaleString()}</time>. Actual retrieval depends on collector availability.</Text>}
    </>}
    {score?.value !== undefined && <>
      <Text>Score {score.value.toFixed(2)}{scoreFilter?.threshold_tenths !== undefined && ` · Threshold ${(scoreFilter.threshold_tenths / 10).toFixed(1)}`}</Text>
      {score.weights?.map((weight) => <Text key={`${weight.kind}:${weight.label}`}>{weight.weight > 0 ? '+' : ''}{weight.weight.toFixed(2)} {weightPrefixes[weight.kind]}{weight.label}{weight.source && sourceSuffixes[weight.source]}</Text>)}
    </>}
    {rule && <Text>{rule}</Text>}
  </Flex>
}

export default function PandaCard({ gallery, scoreFilter }: { gallery: PandaGallery, scoreFilter?: PandaCatalogScoreFilter }) {
  const [failedImage, setFailedImage] = useState<string | null>(null)
  const failed = gallery.metadata_state === 'failed'
  return <GalleryBox href={gallery.url} target="_blank" rel="noopener noreferrer" title={gallery.title} pageCount={gallery.page_count} tags={gallery.summary_tags.map(stripTag)}
    subdued={!!gallery.displayed}
    tooltip={<PandaDetails gallery={gallery} scoreFilter={scoreFilter} />}
    status={gallery.metadata_source === 'feed' && <Badge variant="solid" color={failed ? 'red' : 'gray'}>
      {failed ? <ExclamationTriangleIcon /> : <ClockIcon />}<VisuallyHidden>{failed ? 'Collection failed' : 'Waiting for metadata'}</VisuallyHidden>
    </Badge>}
    cover={(image) => gallery.thumbnail_url && failedImage !== gallery.thumbnail_url
      ? <img src={gallery.thumbnail_url} alt="" loading="lazy" referrerPolicy="no-referrer" {...image} onError={() => setFailedImage(gallery.thumbnail_url)} />
      : <Text size="1" color="gray">No cover</Text>} />
}
