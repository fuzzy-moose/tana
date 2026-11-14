import { useState } from 'react'
import { Badge, Flex, IconButton, Spinner, Text, VisuallyHidden } from '@radix-ui/themes'
import { BookmarkFilledIcon, BookmarkIcon, ClockIcon, Cross2Icon, ExclamationTriangleIcon } from '@radix-ui/react-icons'
import GalleryBox from '../galleries/GalleryBox'
import { verdictLabels } from './api'
import type { PandaCatalogScoreFilter, PandaGallery, PandaScoreWeight, PandaVerdict } from './api'

const weightPrefixes: Record<PandaScoreWeight['kind'], string> = { tag: '', uploader: 'Uploader ', category: 'Category ' }

/** The rule that decides whether the score filter hides the gallery: the verdict first, then the collector's rule. */
function scoreRule({ verdict, score }: PandaGallery) {
  if (verdict === 'not_for_me') return 'Hidden by your "Not for me" verdict.'
  if (verdict === 'interesting') return ''
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
      {score.weights?.map((weight) => <Text key={`${weight.kind}:${weight.label}`}>{weight.weight > 0 ? '+' : ''}{weight.weight.toFixed(2)} {weightPrefixes[weight.kind]}{weight.label}</Text>)}
    </>}
    {rule && <Text>{rule}</Text>}
  </Flex>
}

/** Settles when the save settles and never rejects. */
export type VerdictHandler = (gallery: PandaGallery, verdict: PandaVerdict | null) => Promise<void>


/** The two verdict controls. Pressed state comes only from the stored verdict; a pending save only disables them. */
function VerdictControls({ gallery, onVerdict }: { gallery: PandaGallery, onVerdict: VerdictHandler }) {
  const [pending, setPending] = useState<PandaVerdict | null>(null)
  const click = (verdict: PandaVerdict) => {
    if (pending) return
    setPending(verdict)
    void onVerdict(gallery, gallery.verdict === verdict ? null : verdict).finally(() => setPending(null))
  }
  // aria-disabled, not disabled: a disabled button drops the keyboard focus.
  return (['not_for_me', 'interesting'] as const).map((verdict) => {
    const given = gallery.verdict === verdict
    const icon = verdict === 'not_for_me' ? <Cross2Icon /> : given ? <BookmarkFilledIcon /> : <BookmarkIcon />
    return <IconButton key={verdict} type="button" size="1" aria-label={verdictLabels[verdict]} title={verdictLabels[verdict]} aria-pressed={given}
      aria-disabled={pending ? true : undefined} variant={given ? 'solid' : 'soft'} color={given ? (verdict === 'not_for_me' ? 'red' : undefined) : 'gray'}
      onClick={() => click(verdict)}>
      {pending === verdict ? <Spinner /> : icon}
    </IconButton>
  })
}

export default function PandaCard({ gallery, onVerdict, subdued = !!gallery.displayed, scoreFilter }: { gallery: PandaGallery, onVerdict?: VerdictHandler, subdued?: boolean, scoreFilter?: PandaCatalogScoreFilter }) {
  const [failedImage, setFailedImage] = useState<string | null>(null)
  const failed = gallery.metadata_state === 'failed'
  return <GalleryBox actions={onVerdict && <VerdictControls gallery={gallery} onVerdict={onVerdict} />} href={gallery.url} target="_blank" rel="noopener noreferrer" title={gallery.title} pageCount={gallery.page_count} tags={gallery.summary_tags}
    subdued={subdued}
    tooltip={<PandaDetails gallery={gallery} scoreFilter={scoreFilter} />}
    status={gallery.metadata_source === 'feed' && <Badge variant="solid" color={failed ? 'red' : 'gray'}>
      {failed ? <ExclamationTriangleIcon /> : <ClockIcon />}<VisuallyHidden>{failed ? 'Collection failed' : 'Waiting for metadata'}</VisuallyHidden>
    </Badge>}
    cover={(image) => gallery.thumbnail_url && failedImage !== gallery.thumbnail_url
      ? <img src={gallery.thumbnail_url} alt="" loading="lazy" referrerPolicy="no-referrer" {...image} onError={() => setFailedImage(gallery.thumbnail_url)} />
      : <Text size="1" color="gray">No cover</Text>} />
}
