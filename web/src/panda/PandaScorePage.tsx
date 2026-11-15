import { useEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { AlertDialog, Button, Callout, DataList, Flex, Heading, IconButton, Link, SegmentedControl, Spinner, Switch, Table, Text, TextField } from '@radix-ui/themes'
import { ArrowLeftIcon, CheckIcon, Cross2Icon, DrawingPinIcon, ExclamationTriangleIcon, ValueNoneIcon } from '@radix-ui/react-icons'
import { APIError } from '../api'
import { formatDate } from '../collector/formatDate'
import TagCompletionField from '../galleries/TagCompletionField'
import { useMutation } from '../collector/useMutation'
import { completePandaSearch } from './api'
import { pandaCategories } from './categories'
import { applyPandaScore, currentHiddenShare, formatWeight, hiddenShare, parseNumber, previewPandaScore, recentHiddenShare, removePandaScoreOverride, removePandaScorePin, savePandaAuthorBonus, savePandaScoreOverride, savePandaScorePin, savePandaScoreThreshold, scoreRuleNames, turnOffPandaScore } from './score'
import type { NoScoreModel, ScoreCandidate, ScoreKind, ScoreOverride, ScorePin, ScorePreview, ScoreRule, ScoreWeight } from './score'
import './PandaCatalog.css'
import './PandaScorePage.css'

const pageSize = 50

const noModelMessages: Record<NoScoreModel, string> = {
  no_verdicts: 'No verdicts yet. Mark galleries as Interesting or Not for me with the two controls on each card of the Panda listing, then come back.',
  no_not_for_me: 'No "Not for me" verdicts yet. A model needs at least one: mark galleries you would skip as Not for me on their cards in the Panda listing.',
  no_acceptable: 'No acceptable galleries yet. A model needs at least one gallery marked Interesting, or one gallery that you passed in the Panda listing without a verdict.',
}

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
type PinChange = 'pin-row' | 'pin-list' | 'pin-form' | 'pin-author'
type Change = 'apply' | 'threshold' | 'off' | 'bonus' | OverrideChange | PinChange

/** Changes that bring a new calculation, not a new model: the threshold fields keep their values. */
const calculationChanges: Change[] = ['override-row', 'override-list', 'override-form', 'bonus']

const changeFailures: Record<Change, string> = {
  apply: 'Could not apply the model.',
  threshold: 'Could not save the threshold.',
  off: 'Could not turn off the model.',
  bonus: 'Could not save the author bonus.',
  'override-row': 'Could not change the override.',
  'override-list': 'Could not change the override.',
  'override-form': 'Could not change the override.',
  'pin-row': 'Could not change the pin.',
  'pin-list': 'Could not change the pin.',
  'pin-form': 'Could not change the pin.',
  'pin-author': 'Could not change the pin.',
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
  bonus: '',
  'override-row': '',
  'override-list': '',
  'override-form': '',
  'pin-row': '',
  'pin-list': '',
  'pin-form': '',
  'pin-author': '',
}

interface ChangeState {
  pending: boolean
  error: string
  notice: string
  action: Change | null
}

const rules: ScoreRule[] = ['always_hide', 'never_hide']

/** Saves or removes an override; the page calculates again after it. */
type OverrideRequest = (signal: AbortSignal) => Promise<void>

/** Saves or removes a pin and answers how the pin state changes. */
type PinRequest = (signal: AbortSignal) => Promise<(pins: ScorePin[]) => ScorePin[]>

const savePin = (pin: { kind: ScoreKind, value: string, weight: number }): PinRequest => async (signal) => {
  const saved = await savePandaScorePin(pin, signal)
  return (pins) => {
    const same = (item: ScorePin) => item.kind === saved.kind && item.label === saved.label
    return pins.some(same) ? pins.map((item) => same(item) ? saved : item) : [...pins, saved]
  }
}

const removePin = (id: number): PinRequest => async (signal) => {
  await removePandaScorePin(id, signal)
  return (pins) => pins.filter((item) => item.id !== id)
}

const findPin = (pins: ScorePin[], kind: ScoreKind, label: string) => pins.find((pin) => pin.kind === kind && pin.label === label)

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
  return <Flex gap="1" justify="end">{rules.map((rule) => {
    const isPressed = current?.rule === rule
    return <Button key={rule} size="1" variant={isPressed ? 'solid' : 'soft'} color={isPressed ? undefined : 'gray'} aria-pressed={isPressed} disabled={disabled}
      onClick={() => onOverride(current && isPressed
        ? (signal) => removePandaScoreOverride(current.id, signal)
        : (signal) => savePandaScoreOverride({ kind: weight.kind, value: weight.label, rule }, signal))}>{scoreRuleNames[rule]}</Button>
  })}</Flex>
}

/** The text of a pin's weight field: it follows the pin whenever the pin changes, and keeps the user's text otherwise. */
function usePinText(pin: ScorePin | undefined): [string, (text: string) => void] {
  const [text, setText] = useState(pin ? String(pin.weight) : '')
  const pinKey = pin ? `${pin.id}:${pin.weight}` : ''
  const [shownPin, setShownPin] = useState(pinKey)
  if (shownPin !== pinKey) {
    setShownPin(pinKey)
    setText(pin ? String(pin.weight) : '')
  }
  return [text, setText]
}

/** The pin field and buttons of one tag, uploader or category. */
function PinControls({ kind, label, pin, disabled, onPin }: {
  kind: ScoreKind, label: string, pin: ScorePin | undefined, disabled: boolean, onPin: (operation: PinRequest) => void,
}) {
  const [text, setText] = usePinText(pin)
  const weight = parseNumber(text)
  const pinWeight = () => { if (weight !== null && !disabled) onPin(savePin({ kind, value: label, weight })) }
  return <Flex gap="1" align="center">
    <TextField.Root size="1" type="number" step="any" aria-label={`Pinned weight of ${weightLabel({ kind, label })}`} style={{ width: '6rem' }}
      value={text} onChange={(event) => setText(event.target.value)}
      onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); pinWeight() } }} />
    <IconButton size="1" variant="soft" aria-label="Pin" title="Pin the entered weight" disabled={weight === null || disabled} onClick={pinWeight}><DrawingPinIcon /></IconButton>
    <IconButton size="1" variant="soft" color="gray" aria-label="Ignore" title="Ignore: pin the weight 0" disabled={disabled}
      onClick={() => onPin(savePin({ kind, value: label, weight: 0 }))}><ValueNoneIcon /></IconButton>
    {pin
      ? <IconButton size="1" variant="soft" color="gray" aria-label="Remove pin" title="Remove pin" disabled={disabled} onClick={() => onPin(removePin(pin.id))}><Cross2Icon /></IconButton>
      : <span className="score-pin-slot" />}
  </Flex>
}

/**
 * A filter text and the number of shown entries of a list, kept while the list stays mounted.
 * A new filter text shows the first page of its matches again.
 */
function useFilteredList<T>(entries: T[], label: (entry: T) => string) {
  const [filter, setFilterText] = useState('')
  const [shown, setShown] = useState(pageSize)
  const needle = filter.trim().toLowerCase()
  const matches = needle === '' ? entries : entries.filter((entry) => label(entry).toLowerCase().includes(needle))
  return {
    filter, filtering: needle !== '', matches, visible: matches.slice(0, shown), more: shown < matches.length,
    setFilter: (text: string) => { setFilterText(text); setShown(pageSize) },
    showMore: () => setShown(shown + pageSize),
  }
}

function FilterField({ label, placeholder, value, onChange }: { label: string, placeholder: string, value: string, onChange: (text: string) => void }) {
  return <TextField.Root size="2" aria-label={label} placeholder={placeholder} value={value} onChange={(event) => onChange(event.target.value)} />
}

/** The count line or the empty message of a list, with the scope of the filter when the preview carries fewer entries than exist. */
function CountLine({ list, carried, all, noun, order, empty, scope }: {
  list: { filtering: boolean, matches: unknown[], visible: unknown[] }, carried: number, all: number, noun: string, order: string, empty: string, scope: string,
}) {
  const text = !list.filtering
    ? `${list.visible.length.toLocaleString()} of ${all.toLocaleString()} ${noun}, ${order}.`
    : list.matches.length === 0 ? empty : `${list.visible.length.toLocaleString()} of ${list.matches.length.toLocaleString()} matching ${noun}, ${order}.`
  return <Text as="p" size="2" color="gray">{text}{list.filtering && carried < all && ` ${scope}`}</Text>
}

function Weights({ weights, count, overrides, disabled, onOverride, pins, pinDisabled, onPin }: {
  weights: ScoreWeight[], count: number, overrides: ScoreOverride[], disabled: boolean, onOverride: (operation: OverrideRequest) => void,
  pins: ScorePin[], pinDisabled: boolean, onPin: (operation: PinRequest) => void,
}) {
  const list = useFilteredList(weights, weightLabel)
  const { visible } = list
  return <Flex direction="column" gap="3">
    <FilterField label="Filter weights" placeholder="Filter by tag, uploader or category" value={list.filter} onChange={list.setFilter} />
    <CountLine list={list} carried={weights.length} all={count} noun="weights" order="strongest first" empty="No weights match."
      scope={`Only the ${weights.length.toLocaleString()} strongest of ${count.toLocaleString()} weights are searched.`} />
    {(!list.filtering || visible.length > 0) && <Table.Root size="1" variant="surface" className="score-weights">
      <Table.Header><Table.Row>
        <Table.ColumnHeaderCell>Tag, uploader or category</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell justify="end">Weight</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell justify="end">Interesting</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell justify="end">Not for me</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell justify="end">Neutral</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell justify="end">Active weight</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell justify="end">Override</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell>Pin</Table.ColumnHeaderCell>
      </Table.Row></Table.Header>
      <Table.Body>{visible.map((weight) => <Table.Row key={`${weight.kind}:${weight.label}`}>
        <Table.Cell className="score-label-cell"><span className="score-label" title={weightLabel(weight)}>{weightLabel(weight)}</span></Table.Cell>
        <Table.Cell justify="end">{formatWeight(weight.weight)}</Table.Cell>
        <Table.Cell justify="end">{weight.interesting.toLocaleString()}</Table.Cell>
        <Table.Cell justify="end">{weight.not_for_me.toLocaleString()}</Table.Cell>
        <Table.Cell justify="end">{weight.neutral.toLocaleString()}</Table.Cell>
        <Table.Cell justify="end">{weight.active_weight === null ? '–' : formatWeight(weight.active_weight)}</Table.Cell>
        <Table.Cell justify="end"><OverrideButtons weight={weight} overrides={overrides} disabled={disabled} onOverride={onOverride} /></Table.Cell>
        <Table.Cell><PinControls kind={weight.kind} label={weight.label} pin={findPin(pins, weight.kind, weight.label)} disabled={pinDisabled} onPin={onPin} /></Table.Cell>
      </Table.Row>)}</Table.Body>
    </Table.Root>}
    {list.more && <Button variant="soft" color="gray" onClick={list.showMore}>Show more</Button>}
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
        {rules.map((item) => <SegmentedControl.Item key={item} value={item}>{scoreRuleNames[item]}</SegmentedControl.Item>)}
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
          {kind === 'tag'
            ? <TagCompletionField id="panda-score-override-value" value={text} onChange={setText} complete={completePandaSearch} insert="tag" />
            : <TextField.Root id="panda-score-override-value" value={text} onChange={(event) => setText(event.target.value)} />}
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
        <Text size="2" data-override-label="">{scoreRuleNames[override.rule]} {weightLabel(override)}</Text>
        <Button size="1" variant="soft" color="gray" disabled={disabled}
          onClick={() => onOverride('override-list', (signal) => removePandaScoreOverride(override.id, signal))}>Remove</Button>
      </li></Flex>)}</ul></Flex>}
    <ChangeResult change={change} actions={['override-list']} />
    <OverrideForm disabled={disabled} change={change} onAdd={(operation, onSaved) => onOverride('override-form', operation, onSaved)} />
  </Section>
}

function PinForm({ disabled, change, onAdd }: {
  disabled: boolean, change: ChangeState, onAdd: (operation: PinRequest, onSaved: () => void) => void,
}) {
  const [kind, setKind] = useState<ScoreKind>('tag')
  const [text, setText] = useState('')
  const [category, setCategory] = useState(pandaCategories[0])
  const [weightText, setWeightText] = useState('')
  const value = kind === 'category' ? category : text.trim()
  const weight = parseNumber(weightText)
  return <Flex asChild direction="column" gap="3"><form aria-label="Add pin" onSubmit={(event) => {
    event.preventDefault()
    if (value === '' || weight === null || disabled) return
    onAdd(savePin({ kind, value, weight }), () => { setText(''); setWeightText('') })
  }}>
    <Heading as="h3" size="3">Add pin</Heading>
    <Flex><SegmentedControl.Root size="1" aria-label="Kind" value={kind} onValueChange={(next) => setKind(next as ScoreKind)}>
      <SegmentedControl.Item value="tag">Tag</SegmentedControl.Item>
      <SegmentedControl.Item value="uploader">Uploader</SegmentedControl.Item>
      <SegmentedControl.Item value="category">Category</SegmentedControl.Item>
    </SegmentedControl.Root></Flex>
    <Flex direction="column" gap="1" maxWidth="360px">
      {kind === 'category'
        ? <>
          <Text as="label" htmlFor="panda-score-pin-category" size="2" weight="medium">Category</Text>
          <select id="panda-score-pin-category" value={category} onChange={(event) => setCategory(event.target.value)}>
            {pandaCategories.map((item) => <option key={item} value={item}>{item}</option>)}
          </select>
        </>
        : <>
          <Text as="label" htmlFor="panda-score-pin-value" size="2" weight="medium">{kind === 'tag' ? 'Tag (namespace:value)' : 'Uploader (exactly as Panda writes it)'}</Text>
          {kind === 'tag'
            ? <TagCompletionField id="panda-score-pin-value" value={text} onChange={setText} complete={completePandaSearch} insert="tag" />
            : <TextField.Root id="panda-score-pin-value" value={text} onChange={(event) => setText(event.target.value)} />}
        </>}
    </Flex>
    <Flex direction="column" gap="1" maxWidth="160px">
      <Text as="label" htmlFor="panda-score-pin-weight" size="2" weight="medium">Weight</Text>
      <TextField.Root id="panda-score-pin-weight" type="number" step="any" value={weightText} onChange={(event) => setWeightText(event.target.value)} />
    </Flex>
    <Flex><Button type="submit" disabled={value === '' || weight === null || disabled}>Add pin</Button></Flex>
    <ChangeResult change={change} actions={['pin-form']} />
  </form></Flex>
}

/** A row of the "Pinned weights" table: its field saves another weight for the pin, beside the weight of the value in the new model. */
function PinnedWeight({ pin, modelWeight, showModel, disabled, onSave, onRemove }: {
  pin: ScorePin, modelWeight: number | undefined, showModel: boolean, disabled: boolean, onSave: (weight: number) => void, onRemove: () => void,
}) {
  const [text, setText] = usePinText(pin)
  const weight = parseNumber(text)
  const changed = weight !== pin.weight
  const save = () => { if (weight !== null && changed && !disabled) onSave(weight) }
  const label = weightLabel(pin)
  return <Table.Row>
    <Table.Cell className="score-label-cell"><span className="score-label" title={label} data-pin-label="">{label}</span></Table.Cell>
    {showModel && <Table.Cell justify="end">{modelWeight === undefined ? '–' : formatWeight(modelWeight)}</Table.Cell>}
    <Table.Cell><Flex gap="1" align="center">
      <TextField.Root size="1" type="number" step="any" aria-label={`Weight of pin ${label}`} style={{ width: '6rem' }}
        value={text} onChange={(event) => setText(event.target.value)}
        onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); save() } }} />
      {changed
        ? <IconButton size="1" variant="soft" aria-label="Save" title="Save the weight" disabled={weight === null || disabled} onClick={save}><CheckIcon /></IconButton>
        : <span className="score-pin-slot" />}
      <Button size="1" variant="soft" color="gray" disabled={disabled} onClick={onRemove}>Remove</Button>
    </Flex></Table.Cell>
  </Table.Row>
}

function PinnedWeights({ pins, modelPins, active, disabled, change, onPin }: {
  pins: ScorePin[], modelPins: ScorePin[], active: boolean, disabled: boolean, change: ChangeState,
  onPin: (action: PinChange, operation: PinRequest, onSaved?: () => void) => void,
}) {
  const modelWeight = (pin: ScorePin) => findPin(modelPins, pin.kind, pin.label)?.model_weight
  const showModel = pins.some((pin) => modelWeight(pin) !== undefined)
  return <Section id="panda-score-pins" title="Pinned weights">
    {!active && <Text as="p" size="2" color="gray">No score model is active: pinned weights have no effect.</Text>}
    {pins.length === 0
      ? <Text as="p" size="2" color="gray">No pinned weights.</Text>
      : <Table.Root size="1" variant="surface" className="score-weights">
        <Table.Header><Table.Row>
          <Table.ColumnHeaderCell>Tag, uploader or category</Table.ColumnHeaderCell>
          {showModel && <Table.ColumnHeaderCell justify="end">Model weight</Table.ColumnHeaderCell>}
          <Table.ColumnHeaderCell>Pin</Table.ColumnHeaderCell>
        </Table.Row></Table.Header>
        <Table.Body>{pins.map((pin) => <PinnedWeight key={pin.id} pin={pin} modelWeight={modelWeight(pin)} showModel={showModel} disabled={disabled}
          onSave={(weight) => onPin('pin-list', savePin({ kind: pin.kind, value: pin.label, weight }))}
          onRemove={() => onPin('pin-list', removePin(pin.id))} />)}</Table.Body>
      </Table.Root>}
    <ChangeResult change={change} actions={['pin-list']} />
    <PinForm disabled={disabled} change={change} onAdd={(operation, onSaved) => onPin('pin-form', operation, onSaved)} />
  </Section>
}

/** The author bonus field and its buttons; remounted to start again from the preview. */
function AuthorBonus({ bonus, disabled, onSave }: { bonus: number | null, disabled: boolean, onSave: (bonus: number | null) => void }) {
  const [text, setText] = useState(bonus === null ? '' : String(bonus))
  const value = parseNumber(text)
  const valid = value !== null && value > 0
  return <Flex align="end" gap="3" wrap="wrap">
    <Flex direction="column" gap="1" maxWidth="160px">
      <Text as="label" htmlFor="panda-score-author-bonus" size="2" weight="medium">Author bonus</Text>
      <TextField.Root id="panda-score-author-bonus" type="number" step="0.1" value={text} onChange={(event) => setText(event.target.value)} />
    </Flex>
    <Button disabled={!valid || disabled} onClick={() => { if (valid) onSave(value) }}>Save bonus</Button>
    {bonus !== null && <Button color="red" variant="soft" disabled={disabled} onClick={() => onSave(null)}>Turn off bonus</Button>}
    <Text as="p" size="2" color="gray">{bonus === null ? 'The author bonus is off.' : `Each favorite author's tag weighs at least ${formatWeight(bonus)}.`}</Text>
  </Flex>
}

function FavoriteAuthors({ preview, bonusVersion, pins, disabled, pinDisabled, change, onBonus, onPin }: {
  preview: ScorePreview, bonusVersion: number, pins: ScorePin[], disabled: boolean, pinDisabled: boolean, change: ChangeState,
  onBonus: (bonus: number | null) => void, onPin: (operation: PinRequest) => void,
}) {
  const authors = preview.favorite_authors
  const list = useFilteredList(authors, (author) => author.label)
  const all = preview.favorite_author_count
  return <Section id="panda-score-authors" title="Favorite authors">
    <Text as="p" size="2">{preview.favorite_author_count.toLocaleString()} favorite authors: artist tags that at least 2 Panda favorites carry.</Text>
    {!preview.active && <Text as="p" size="2" color="gray">No score model is active: the author bonus has no effect.</Text>}
    <AuthorBonus key={bonusVersion} bonus={preview.author_bonus} disabled={disabled} onSave={onBonus} />
    <ChangeResult change={change} actions={['bonus', 'pin-author']} />
    {authors.length > 0 && <>
      <FilterField label="Filter favorite authors" placeholder="Filter by artist" value={list.filter} onChange={list.setFilter} />
      <CountLine list={list} carried={authors.length} all={all} noun="favorite authors" order="most favorites first" empty="No favorite authors match."
        scope={`Only the ${authors.length.toLocaleString()} authors with the most favorites of ${all.toLocaleString()} are searched.`} />
    </>}
    {list.visible.length > 0 && <ul className="rt-reset score-rows" style={{ '--score-columns': 3 } as CSSProperties}>{list.visible.map((author) => <li key={author.label}>
      <Text size="2" className="score-label" title={author.label} data-author-cell="">{author.label}</Text>
      <Text size="2" color="gray" data-author-cell="">{author.favorites.toLocaleString()} favorites</Text>
      <Text size="2" data-author-cell="">{author.weight === null ? '–' : formatWeight(author.weight)}</Text>
      <PinControls kind="tag" label={author.label} pin={findPin(pins, 'tag', author.label)} disabled={pinDisabled} onPin={onPin} />
    </li>)}</ul>}
    {list.more && <Flex><Button variant="soft" color="gray" onClick={list.showMore}>Show more</Button></Flex>}
  </Section>
}

interface PreviewProps {
  preview: ScorePreview
  loading: boolean
  onFavorites: (favorites: boolean) => void
  thresholds: Thresholds
  onThresholds: (thresholds: Thresholds) => void
  change: ChangeState
  onChange: (action: Change, operation: (signal: AbortSignal) => Promise<void>, onSaved?: () => void) => void
  pins: ScorePin[]
  onPin: (action: PinChange, operation: PinRequest, onSaved?: () => void) => void
  bonusVersion: number
}

function Preview({ preview, loading, onFavorites, thresholds, onThresholds, change, onChange, pins, onPin, bonusVersion }: PreviewProps) {
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
      <ChangeResult change={change} actions={['override-row', 'pin-row']} />
      {candidate
        ? <Weights weights={candidate.weights} count={candidate.weight_count} overrides={preview.overrides}
          disabled={change.pending || loading} onOverride={(operation) => onChange('override-row', operation)}
          pins={pins} pinDisabled={change.pending} onPin={(operation) => onPin('pin-row', operation)} />
        : preview.no_model && <Text as="p" size="2">{noModelMessages[preview.no_model]}</Text>}
    </Section>
    <Overrides overrides={preview.overrides} disabled={change.pending || loading} change={change} onOverride={onChange} />
    <PinnedWeights pins={pins} modelPins={preview.pins} active={active !== null} disabled={change.pending} change={change} onPin={onPin} />
    <FavoriteAuthors preview={preview} bonusVersion={bonusVersion} pins={pins} disabled={change.pending || loading} pinDisabled={change.pending} change={change}
      onBonus={(bonus) => onChange('bonus', (signal) => savePandaAuthorBonus(bonus, signal))} onPin={(operation) => onPin('pin-author', operation)} />
  </>
}

export default function PandaScorePage() {
  const [preview, setPreview] = useState<ScorePreview | null>(null)
  // Changed at once by a pin change, before the calculation that follows answers.
  const [pins, setPins] = useState<ScorePin[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  // A new request object calculates again, also with an unchanged favorites setting.
  const [request, setRequest] = useState<{ favorites: boolean | null }>({ favorites: null })
  const [thresholds, setThresholds] = useState<Thresholds>({ candidate: null, active: null })
  // Set by a successful change: the next preview restarts both threshold fields.
  const restart = useRef(false)
  // Set by a successful bonus change: the next preview starts the bonus field again.
  const restartBonus = useRef(false)
  const [bonusVersion, setBonusVersion] = useState(0)
  const change = useMutation()
  const [action, setAction] = useState<Change | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    previewPandaScore(request.favorites, controller.signal).then((next) => {
      if (controller.signal.aborted) return
      const restarting = restart.current
      restart.current = false
      if (restartBonus.current) { restartBonus.current = false; setBonusVersion((version) => version + 1) }
      setThresholds((current) => startThresholds(current, next, restarting))
      setPreview(next)
      setPins(next.pins)
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
        restart.current = !calculationChanges.includes(next)
        if (next === 'bonus') restartBonus.current = true
        onSaved?.()
        load(favorites)
        return notice
      },
    })
  }

  function runPinChange(next: PinChange, operation: PinRequest, onSaved?: () => void) {
    setAction(next)
    const favorites = request.favorites
    void change.run(operation, {
      // A pin change leaves the threshold fields and `restart` as they are; its calculation replaces one in progress.
      onSuccess: (update) => { setPins(update); onSaved?.(); load(favorites) },
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
      preview={preview} loading={loading} thresholds={thresholds} onThresholds={setThresholds}
      onFavorites={(favorites) => { change.clear(); load(favorites) }}
      change={{ pending: change.pending, error: change.error, notice: change.notice, action }}
      onChange={(next, operation, onSaved) => runChange(next, operation, preview.favorites, changeNotices[next], onSaved)}
      pins={pins} onPin={runPinChange} bonusVersion={bonusVersion} />}
  </section></Flex>
}
