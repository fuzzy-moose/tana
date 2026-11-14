import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { AlertDialog, Button, Callout, DataList, Flex, Heading, Link, SegmentedControl, Spinner, Switch, Table, Text, TextField } from '@radix-ui/themes'
import { ArrowLeftIcon, ExclamationTriangleIcon } from '@radix-ui/react-icons'
import { APIError } from '../api'
import { formatDate } from '../collector/formatDate'
import { useMutation } from '../collector/useMutation'
import { pandaCategories } from './categories'
import { applyPandaScore, currentHiddenShare, hiddenShare, previewPandaScore, recentHiddenShare, removePandaScoreOverride, savePandaScoreOverride, savePandaScoreThreshold, turnOffPandaScore } from './score'
import type { NoScoreModel, ScoreCandidate, ScoreKind, ScoreOverride, ScorePreview, ScoreRule, ScoreWeight } from './score'
import './PandaCatalog.css'

const pageSize = 50

const noModelMessages: Record<NoScoreModel, string> = {
  no_verdicts: 'No verdicts yet. Mark galleries as Interesting or Not for me with the two controls on each card of the Panda listing, then come back.',
  no_not_for_me: 'No "Not for me" verdicts yet. A model needs at least one: mark galleries you would skip as Not for me on their cards in the Panda listing.',
  no_acceptable: 'No acceptable galleries yet. A model needs at least one gallery marked Interesting, or one gallery that you passed in the Panda listing without a verdict.',
}

const formatWeight = (value: number) => `${value >= 0 ? '+' : ''}${value.toFixed(2)}`
const formatThreshold = (tenths: number) => (tenths / 10).toFixed(1)
const formatShare = (share: number) => Math.round(share * 100)

/** The field's text in tenths, or null when it holds no number. */
function parseTenths(text: string | null) {
  if (text === null || text.trim() === '') return null
  const value = Number(text)
  return Number.isFinite(value) ? Math.round(value * 10) : null
}

/** The override changes are named after the control that made them, so the message shows beside it. */
type OverrideChange = 'override-row' | 'override-list' | 'override-form'
type Change = 'apply' | 'threshold' | 'off' | OverrideChange

const overrideChanges: OverrideChange[] = ['override-row', 'override-list', 'override-form']

const changeFailures: Record<Change, string> = {
  apply: 'Could not apply the model.',
  threshold: 'Could not save the threshold.',
  off: 'Could not turn off the model.',
  'override-row': 'Could not change the override.',
  'override-list': 'Could not change the override.',
  'override-form': 'Could not change the override.',
}

/** The texts of the two threshold fields; null until a preview supplies a start value. */
interface Thresholds { candidate: string | null, active: string | null }

/** Fills empty fields from the preview, and every field after a change of the model (`restart`). */
function startThresholds(current: Thresholds, preview: ScorePreview, restart: boolean): Thresholds {
  const pick = (value: string | null, tenths: number | undefined) => {
    if (!restart && value !== null) return value
    return tenths === undefined ? null : formatThreshold(tenths)
  }
  return {
    candidate: pick(current.candidate, preview.active?.threshold_tenths ?? preview.candidate?.suggested_threshold_tenths),
    active: pick(current.active, preview.active?.threshold_tenths),
  }
}

const changeNotices: Record<Change, string> = {
  apply: 'Model applied.',
  threshold: 'Threshold saved.',
  off: 'Model turned off.',
  'override-row': '',
  'override-list': '',
  'override-form': '',
}

interface ChangeState {
  pending: boolean
  error: string
  notice: string
  action: Change | null
}

const ruleNames: Record<ScoreRule, string> = { always_hide: 'Always hide', never_hide: 'Never hide' }
const rules: ScoreRule[] = ['always_hide', 'never_hide']

/** Saves or removes an override; the page calculates again after it. */
type OverrideRequest = (signal: AbortSignal) => Promise<void>

function weightLabel({ kind, label }: { kind: ScoreKind, label: string }) {
  if (kind === 'uploader') return `Uploader ${label}`
  if (kind === 'category') return `Category ${label}`
  return label
}

function Section({ id, title, children }: { id: string, title: string, children: ReactNode }) {
  return <Flex asChild direction="column" gap="3"><section aria-labelledby={id}>
    <Heading as="h2" size="4" id={id}>{title}</Heading>
    {children}
  </section></Flex>
}

function ThresholdField({ id, label, value, onChange }: { id: string, label: string, value: string, onChange: (value: string) => void }) {
  return <Flex direction="column" gap="1" maxWidth="160px">
    <Text as="label" htmlFor={id} size="2" weight="medium">{label}</Text>
    <TextField.Root id={id} type="number" step="0.1" value={value} onChange={(event) => onChange(event.target.value)} />
  </Flex>
}

function ChangeResult({ change, actions }: { change: ChangeState, actions: Change[] }) {
  if (!change.action || !actions.includes(change.action)) return null
  if (change.error) return <Text as="p" size="2" color="red" role="alert">{changeFailures[change.action]} {change.error}</Text>
  if (change.notice) return <Text as="p" size="2" color="green" role="status">{change.notice}</Text>
  return null
}

function CandidateThreshold({ preview, candidate, value, onChange, change, disabled, onApply }: {
  preview: ScorePreview, candidate: ScoreCandidate, value: string, onChange: (value: string) => void,
  change: ChangeState, disabled: boolean, onApply: (thresholdTenths: number) => void,
}) {
  const tenths = parseTenths(value)
  const { held_out: heldOut } = candidate
  const recent = preview.recent
  const notForMe = tenths === null ? null : formatShare(hiddenShare(heldOut.not_for_me, tenths))
  return <Flex direction="column" gap="3">
    <Flex align="end" gap="3" wrap="wrap">
      <ThresholdField id="panda-score-threshold" label="Threshold" value={value} onChange={onChange} />
      <Text as="p" size="2" color="gray">Suggested: {formatThreshold(candidate.suggested_threshold_tenths)}</Text>
    </Flex>
    {tenths !== null && <>
      <Text as="p" size="2">Hides {notForMe} % of Not for me, {formatShare(hiddenShare(heldOut.interesting, tenths))} % of Interesting and {formatShare(hiddenShare(heldOut.neutral, tenths))} % of neutral evidence galleries.</Text>
      <Text as="p" size="2">Hides {formatShare(recentHiddenShare(recent, recent.candidate, tenths))} % of the newest {recent.total.toLocaleString()} catalog galleries (now: {formatShare(currentHiddenShare(preview))} %)</Text>
      <Flex direction="column" gap="1">
        <Text as="p" size="2" weight="medium">Error rates, measured on galleries the model did not learn from</Text>
        <Text as="p" size="2">Interesting hidden: {formatShare(hiddenShare(heldOut.interesting, tenths))} %</Text>
        <Text as="p" size="2">Not for me shown: {100 - (notForMe ?? 0)} %</Text>
      </Flex>
    </>}
    <Flex><Button disabled={tenths === null || disabled} onClick={() => { if (tenths !== null) onApply(tenths) }}>Apply model and threshold</Button></Flex>
    <ChangeResult change={change} actions={['apply']} />
  </Flex>
}

function ActiveThreshold({ preview, value, onChange, disabled, onSave, onTurnOff }: {
  preview: ScorePreview, value: string, onChange: (value: string) => void,
  disabled: boolean, onSave: (thresholdTenths: number) => void, onTurnOff: () => void,
}) {
  const [confirming, setConfirming] = useState(false)
  const tenths = parseTenths(value)
  const recent = preview.recent
  return <Flex direction="column" gap="3">
    <ThresholdField id="panda-score-active-threshold" label="Active threshold" value={value} onChange={onChange} />
    {tenths !== null && <Text as="p" size="2">Hides {formatShare(recentHiddenShare(recent, recent.active, tenths))} % of the newest {recent.total.toLocaleString()} catalog galleries</Text>}
    <Flex gap="3">
      <Button disabled={tenths === null || disabled} onClick={() => { if (tenths !== null) onSave(tenths) }}>Save threshold</Button>
      <Button color="red" variant="soft" disabled={disabled} onClick={() => setConfirming(true)}>Turn off</Button>
    </Flex>
    <AlertDialog.Root open={confirming} onOpenChange={setConfirming}>
      <AlertDialog.Content maxWidth="460px">
        <AlertDialog.Title>Turn off the score model?</AlertDialog.Title>
        <AlertDialog.Description size="2">The model is deleted and can't be restored. Overrides keep working.</AlertDialog.Description>
        <Flex gap="3" mt="4" justify="end">
          <AlertDialog.Cancel><Button size="2" variant="soft" color="gray">Cancel</Button></AlertDialog.Cancel>
          <AlertDialog.Action><Button size="2" color="red" onClick={onTurnOff}>Turn off</Button></AlertDialog.Action>
        </Flex>
      </AlertDialog.Content>
    </AlertDialog.Root>
  </Flex>
}

function OverrideButtons({ weight, overrides, disabled, onOverride }: {
  weight: ScoreWeight, overrides: ScoreOverride[], disabled: boolean, onOverride: (operation: OverrideRequest) => void,
}) {
  const current = overrides.find((override) => override.kind === weight.kind && override.label === weight.label)
  return <Flex gap="1" justify="end" wrap="wrap">{rules.map((rule) => {
    const isPressed = current?.rule === rule
    return <Button key={rule} size="1" variant={isPressed ? 'solid' : 'soft'} color={isPressed ? undefined : 'gray'} aria-pressed={isPressed} disabled={disabled}
      onClick={() => onOverride(current && isPressed
        ? (signal) => removePandaScoreOverride(current.id, signal)
        : (signal) => savePandaScoreOverride({ kind: weight.kind, value: weight.label, rule }, signal))}>{ruleNames[rule]}</Button>
  })}</Flex>
}

function Weights({ weights, count, overrides, disabled, onOverride, shown, onShown }: {
  weights: ScoreWeight[], count: number, overrides: ScoreOverride[], disabled: boolean, onOverride: (operation: OverrideRequest) => void,
  shown: number, onShown: (shown: number) => void,
}) {
  const visible = weights.slice(0, shown)
  return <Flex direction="column" gap="3">
    <Text as="p" size="2" color="gray">{visible.length.toLocaleString()} of {count.toLocaleString()} weights, strongest first.</Text>
    <Table.Root size="1" variant="surface">
      <Table.Header><Table.Row>
        <Table.ColumnHeaderCell>Tag, uploader or category</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell justify="end">Weight</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell justify="end">Interesting</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell justify="end">Not for me</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell justify="end">Neutral</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell justify="end">Active weight</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell justify="end">Override</Table.ColumnHeaderCell>
      </Table.Row></Table.Header>
      <Table.Body>{visible.map((weight) => <Table.Row key={`${weight.kind}:${weight.label}`}>
        <Table.Cell>{weightLabel(weight)}</Table.Cell>
        <Table.Cell justify="end">{formatWeight(weight.weight)}</Table.Cell>
        <Table.Cell justify="end">{weight.interesting.toLocaleString()}</Table.Cell>
        <Table.Cell justify="end">{weight.not_for_me.toLocaleString()}</Table.Cell>
        <Table.Cell justify="end">{weight.neutral.toLocaleString()}</Table.Cell>
        <Table.Cell justify="end">{weight.active_weight === null ? '–' : formatWeight(weight.active_weight)}</Table.Cell>
        <Table.Cell justify="end"><OverrideButtons weight={weight} overrides={overrides} disabled={disabled} onOverride={onOverride} /></Table.Cell>
      </Table.Row>)}</Table.Body>
    </Table.Root>
    {shown < weights.length && <Button variant="soft" color="gray" onClick={() => onShown(shown + pageSize)}>Show more</Button>}
  </Flex>
}

function OverrideForm({ disabled, change, onAdd }: {
  disabled: boolean, change: ChangeState, onAdd: (operation: OverrideRequest, onSaved: () => void) => void,
}) {
  const [kind, setKind] = useState<ScoreKind>('tag')
  const [text, setText] = useState('')
  const [category, setCategory] = useState(pandaCategories[0])
  const [rule, setRule] = useState<ScoreRule>('always_hide')
  const value = kind === 'category' ? category : text.trim()
  return <Flex asChild direction="column" gap="3"><form aria-label="Add override" onSubmit={(event) => {
    event.preventDefault()
    if (value === '' || disabled) return
    onAdd((signal) => savePandaScoreOverride({ kind, value, rule }, signal), () => setText(''))
  }}>
    <Heading as="h3" size="3">Add override</Heading>
    <Flex gap="3" wrap="wrap">
      <SegmentedControl.Root size="1" aria-label="Kind" value={kind} onValueChange={(next) => setKind(next as ScoreKind)}>
        <SegmentedControl.Item value="tag">Tag</SegmentedControl.Item>
        <SegmentedControl.Item value="uploader">Uploader</SegmentedControl.Item>
        <SegmentedControl.Item value="category">Category</SegmentedControl.Item>
      </SegmentedControl.Root>
      <SegmentedControl.Root size="1" aria-label="Rule" value={rule} onValueChange={(next) => setRule(next as ScoreRule)}>
        {rules.map((item) => <SegmentedControl.Item key={item} value={item}>{ruleNames[item]}</SegmentedControl.Item>)}
      </SegmentedControl.Root>
    </Flex>
    <Flex direction="column" gap="1" maxWidth="360px">
      {kind === 'category'
        ? <>
          <Text as="label" htmlFor="panda-score-override-category" size="2" weight="medium">Category</Text>
          <select id="panda-score-override-category" value={category} onChange={(event) => setCategory(event.target.value)}>
            {pandaCategories.map((item) => <option key={item} value={item}>{item}</option>)}
          </select>
        </>
        : <>
          <Text as="label" htmlFor="panda-score-override-value" size="2" weight="medium">{kind === 'tag' ? 'Tag (namespace:value)' : 'Uploader (exactly as Panda writes it)'}</Text>
          <TextField.Root id="panda-score-override-value" value={text} onChange={(event) => setText(event.target.value)} />
        </>}
    </Flex>
    <Flex><Button type="submit" disabled={value === '' || disabled}>Add override</Button></Flex>
    <ChangeResult change={change} actions={['override-form']} />
  </form></Flex>
}

function Overrides({ overrides, disabled, change, onOverride }: {
  overrides: ScoreOverride[], disabled: boolean, change: ChangeState,
  onOverride: (action: OverrideChange, operation: OverrideRequest, onSaved?: () => void) => void,
}) {
  return <Section id="panda-score-overrides" title="Overrides">
    {overrides.length === 0
      ? <Text as="p" size="2" color="gray">No overrides.</Text>
      : <Flex asChild direction="column" gap="2"><ul className="rt-reset">{overrides.map((override) => <Flex asChild key={override.id} align="center" gap="3" wrap="wrap"><li>
        <Text size="2" data-override-label="">{ruleNames[override.rule]} {weightLabel(override)}</Text>
        <Button size="1" variant="soft" color="gray" disabled={disabled}
          onClick={() => onOverride('override-list', (signal) => removePandaScoreOverride(override.id, signal))}>Remove</Button>
      </li></Flex>)}</ul></Flex>}
    <ChangeResult change={change} actions={['override-list']} />
    <OverrideForm disabled={disabled} change={change} onAdd={(operation, onSaved) => onOverride('override-form', operation, onSaved)} />
  </Section>
}

interface PreviewProps {
  preview: ScorePreview
  loading: boolean
  onFavorites: (favorites: boolean) => void
  thresholds: Thresholds
  onThresholds: (thresholds: Thresholds) => void
  shown: number
  onShown: (shown: number) => void
  change: ChangeState
  onChange: (action: Change, operation: (signal: AbortSignal) => Promise<void>, onSaved?: () => void) => void
}

function Preview({ preview, loading, onFavorites, thresholds, onThresholds, shown, onShown, change, onChange }: PreviewProps) {
  const { evidence, candidate, active } = preview
  const synced = evidence.favorites_synced_at
  const counts: { label: string, value: ReactNode }[] = [
    { label: 'Interesting verdicts', value: evidence.interesting.toLocaleString() },
    { label: 'Not for me verdicts', value: evidence.not_for_me.toLocaleString() },
    { label: 'Neutral galleries', value: evidence.neutral.toLocaleString() },
    { label: 'Counted favorites', value: evidence.counted_favorites.toLocaleString() },
    { label: 'Last Panda favorite sync', value: synced ? <time dateTime={synced}>{formatDate(synced)}</time> : 'never' },
  ]
  if (active) counts.push({ label: 'Verdicts since the active model was calculated', value: (evidence.new_verdicts ?? 0).toLocaleString() })
  return <>
    <Section id="panda-score-evidence" title="Evidence">
      <DataList.Root size="2" orientation={{ initial: 'vertical', sm: 'horizontal' }}>{counts.map(({ label, value }) => <DataList.Item key={label}>
        <DataList.Label minWidth="240px">{label}</DataList.Label>
        <DataList.Value>{value}</DataList.Value>
      </DataList.Item>)}</DataList.Root>
      <Flex align="center" gap="2">
        <Switch id="panda-score-favorites" checked={preview.favorites} disabled={loading || change.pending} onCheckedChange={onFavorites} />
        <Text as="label" htmlFor="panda-score-favorites" size="2" weight="medium">Count Panda favorites as Interesting</Text>
      </Flex>
    </Section>
    <Section id="panda-score-active" title="Active model">
      <Text as="p" size="2">{active
        ? <>Calculated <time dateTime={active.calculated_at}>{formatDate(active.calculated_at)}</time>, {active.favorites ? 'with favorites' : 'without favorites'}, threshold {formatThreshold(active.threshold_tenths)}.</>
        : 'No model is active.'}</Text>
      {active && thresholds.active !== null && <ActiveThreshold
        preview={preview} value={thresholds.active} onChange={(value) => onThresholds({ ...thresholds, active: value })} disabled={change.pending || loading}
        onSave={(tenths) => onChange('threshold', (signal) => savePandaScoreThreshold(tenths, signal))}
        onTurnOff={() => onChange('off', turnOffPandaScore)} />}
      <ChangeResult change={change} actions={['threshold', 'off']} />
    </Section>
    <Section id="panda-score-new" title="New model">
      {candidate && evidence.few_verdicts && <Callout.Root color="amber" size="1">
        <Callout.Icon><ExclamationTriangleIcon /></Callout.Icon>
        <Callout.Text>Few verdicts. Weights and error rates from so little evidence are unreliable.</Callout.Text>
      </Callout.Root>}
      {candidate && thresholds.candidate !== null && <CandidateThreshold
        preview={preview} candidate={candidate} value={thresholds.candidate} onChange={(value) => onThresholds({ ...thresholds, candidate: value })}
        change={change} disabled={change.pending || loading}
        onApply={(tenths) => onChange('apply', (signal) => applyPandaScore(candidate.id, tenths, signal))} />}
      <ChangeResult change={change} actions={['override-row']} />
      {candidate
        ? <Weights weights={candidate.weights} count={candidate.weight_count} overrides={preview.overrides} shown={shown} onShown={onShown}
          disabled={change.pending || loading} onOverride={(operation) => onChange('override-row', operation)} />
        : preview.no_model && <Text as="p" size="2">{noModelMessages[preview.no_model]}</Text>}
    </Section>
    <Overrides overrides={preview.overrides} disabled={change.pending || loading} change={change} onOverride={onChange} />
  </>
}

export default function PandaScorePage() {
  const [preview, setPreview] = useState<ScorePreview | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  // A new request object calculates again, also with an unchanged favorites setting.
  const [request, setRequest] = useState<{ favorites: boolean | null }>({ favorites: null })
  const [thresholds, setThresholds] = useState<Thresholds>({ candidate: null, active: null })
  // Kept across calculations, so a row changed past the first page stays in view.
  const [shown, setShown] = useState(pageSize)
  // Set by a successful change: the next preview restarts both threshold fields.
  const restart = useRef(false)
  const change = useMutation()
  const [action, setAction] = useState<Change | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    previewPandaScore(request.favorites, controller.signal).then((next) => {
      if (controller.signal.aborted) return
      const restarting = restart.current
      restart.current = false
      setThresholds((current) => startThresholds(current, next, restarting))
      setPreview(next)
      setLoading(false)
    }).catch((error: Error) => {
      if (!controller.signal.aborted) { setError(error.message); setLoading(false) }
    })
    return () => controller.abort()
  }, [request])

  function load(favorites: boolean | null) {
    setLoading(true)
    setError('')
    setRequest({ favorites })
  }

  function runChange(next: Change, operation: (signal: AbortSignal) => Promise<void>, favorites: boolean, notice: string, onSaved?: () => void) {
    setAction(next)
    void change.run(async (signal) => {
      try {
        await operation(signal)
      } catch (error) {
        // A later calculation replaced the candidate: calculate again, keeping the threshold fields.
        if (error instanceof APIError && error.code === 'score_candidate_not_found') load(favorites)
        throw error
      }
    }, {
      onSuccess: () => {
        // An override change brings a new calculation, not a new model: the threshold fields keep their values.
        restart.current = !(overrideChanges as Change[]).includes(next)
        onSaved?.()
        load(favorites)
        return notice
      },
    })
  }

  return <Flex asChild direction="column" gap="5"><section aria-labelledby="panda-score-page-title">
    <Flex direction="column" gap="3">
      <Link href="#/panda" size="2"><ArrowLeftIcon /> Panda catalog</Link>
      <Flex align="center" gap="3">
        <Heading as="h1" size="6" id="panda-score-page-title">Score model</Heading>
        {loading && preview && <Spinner aria-label="Calculating" />}
      </Flex>
      <Text as="p" size="2" color="gray">Calculate a model that scores catalog galleries by their tags, uploader and category, learned from your verdicts, displayed galleries and Panda favorites.</Text>
    </Flex>
    {error && <Callout.Root color="red" role="alert">
      <Callout.Text>{error}</Callout.Text>
      <Flex><Button size="1" variant="soft" color="red" disabled={loading || change.pending} onClick={() => load(request.favorites)}>Retry</Button></Flex>
    </Callout.Root>}
    {loading && !preview && <Text as="p" size="2" color="gray" role="status">Calculating…</Text>}
    {preview && <Preview
      preview={preview} loading={loading} thresholds={thresholds} onThresholds={setThresholds} shown={shown} onShown={setShown}
      onFavorites={(favorites) => { change.clear(); load(favorites) }}
      change={{ pending: change.pending, error: change.error, notice: change.notice, action }}
      onChange={(next, operation, onSaved) => runChange(next, operation, preview.favorites, changeNotices[next], onSaved)} />}
  </section></Flex>
}
