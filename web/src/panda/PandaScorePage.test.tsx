// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import App from '../App'
import type { ScoreFavoriteAuthor, ScorePin, ScorePreview, ScoreWeight } from './score'

vi.mock('../collector/FavoriteSyncAction', () => ({ default: () => null }))

/** Answers every request but the tag completions, from a queue in call order (`respond`). */
const fetchMock = vi.fn<typeof fetch>()
/** Answers the tag completions of the forms apart from the queue, with no suggestions unless a test sets some. */
const completionsMock = vi.fn<typeof fetch>()
const completionsPath = '/api/collector/catalog/completions'

function weight(index: number, fields: Partial<ScoreWeight> = {}): ScoreWeight {
  return { kind: 'tag', label: `tag:value ${index}`, weight: 0.1, favorites: 3, catalog: 40, active_weight: null, ...fields }
}

function preview(fields: Partial<ScorePreview> = {}): ScorePreview {
  const histogram = { first_bucket: 0, counts: [] }
  const heldOut = { total: 0, scores: histogram }
  return {
    evidence: { favorites: 1234, favorites_synced_at: null, new_favorites: 12, few_favorites: false },
    candidate: {
      id: 7, calculated_at: '2026-11-01T10:00:00Z', weight_count: 3100, suggested_threshold_tenths: -8,
      weights: [
        weight(0, { label: 'female:glasses', weight: 1.42, favorites: 31, catalog: 1200, active_weight: 1.1 }),
        weight(1, { kind: 'uploader', label: 'Some uploader', weight: 0.5, favorites: 9, catalog: 85 }),
        weight(2, { kind: 'category', label: 'Doujinshi', weight: 0.257, favorites: 640, catalog: 310000, active_weight: 0.3 }),
      ],
      held_out: heldOut,
    },
    active: { calculated_at: '2026-10-01T10:00:00Z', threshold_tenths: -5 },
    pins: [],
    author_bonus: null,
    favorite_author_count: 0,
    favorite_authors: [],
    recent: { total: 0, candidate: histogram, active: histogram },
    ...fields,
  }
}

function respond(...responses: Response[]) {
  for (const response of responses) fetchMock.mockResolvedValueOnce(response)
}

function previewBodies() {
  return fetchMock.mock.calls.filter(([input]) => input === '/api/collector/catalog/score/preview').map(([, init]) => init?.body)
}

/** The body of a preview that measures the candidate 7 on screen again. */
const measure7 = '{"candidate_id":7}'

/** Waits for the first preview, which shows the tabs. */
const loaded = () => screen.findByRole('tablist')

/** The text of a tab or a segmented control item in jsdom: Radix Themes repeats the label in a span that only its CSS hides. */
const tabName = (name: string) => `${name}${name}`

/** Opens a tab that is not open, as the press of a mouse button does, and answers its panel. The open tab keeps the focus where it is. */
function tab(name: string) {
  const trigger = screen.getByRole('tab', { name: tabName(name) })
  if (trigger.getAttribute('aria-selected') !== 'true') fireEvent.mouseDown(trigger)
  return within(screen.getByRole('tabpanel', { name: tabName(name) }))
}

function region(tabLabel: string, name: string) {
  return within(tab(tabLabel).getByRole('region', { name }))
}

const weightsPanel = () => tab('Weights')

function evidence() {
  return region('Model', 'Evidence')
}

function rows() {
  return within(weightsPanel().getByRole('table')).getAllByRole('row').slice(1).map((row) => within(row).getAllByRole('cell').slice(0, 5).map((cell) => cell.textContent))
}

beforeEach(() => {
  window.history.replaceState(null, '', '#/panda/score')
  fetchMock.mockReset()
  completionsMock.mockReset()
  completionsMock.mockImplementation(async () => Response.json({ start: 0, end: 0, items: [] }))
  vi.stubGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) =>
    String(input).startsWith(`${completionsPath}?`) ? completionsMock(input, init) : fetchMock(input, init))
})
afterEach(() => { cleanup(); vi.unstubAllGlobals() })

test('calculates on visit and shows the evidence, the active model and the weights', async () => {
  respond(Response.json(preview({ active: { ...preview().active!, threshold_tenths: 15 } })))
  render(<App />)
  expect(screen.getByRole('heading', { name: 'Score model' })).toBeTruthy()
  expect(screen.getByRole('link', { name: 'Panda' }).getAttribute('aria-current')).toBe('page')
  expect(screen.getByRole('link', { name: 'Panda catalog' }).getAttribute('href')).toBe('#/panda')
  expect(screen.getByText('Calculating…')).toBeTruthy()
  await loaded()
  expect(previewBodies()).toEqual(['{}'])
  expect(screen.getByText('Calculate a model that scores catalog galleries by their tags, uploader and category, learned from your Panda favorites inside the default filter.')).toBeTruthy()
  for (const [label, value] of [['Favorites in scope', '1,234'], ['Last Panda favorite sync', 'never'], ['New favorites since the active model was calculated', '12']]) {
    expect(evidence().getByText(label).nextElementSibling?.textContent).toBe(value)
  }
  const calculated = activeModel().getByText(/^Calculated/)
  expect(calculated.querySelector('time')?.dateTime).toBe('2026-10-01T10:00:00Z')
  expect(calculated.textContent).toBe(`Calculated ${calculated.querySelector('time')?.textContent}, threshold 1.5.`)
  expect(screen.getByText('3 of 3,100 weights, strongest first.')).toBeTruthy()
  expect(within(weightsPanel().getByRole('table')).getAllByRole('columnheader').map((header) => header.textContent)).toEqual(
    ['Tag, uploader or category', 'Weight', 'Favorites', 'Catalog', 'Active weight', 'Pin'])
  expect(rows()).toEqual([
    ['female:glasses', '+1.42', '31', '1,200', '+1.10'],
    ['Uploader Some uploader', '+0.50', '9', '85', '–'],
    ['Category Doujinshi', '+0.26', '640', '310,000', '+0.30'],
  ])
  expect(screen.queryByText(fewFavorites)).toBeNull()
})

test('shows the time of the last favorite sync', async () => {
  respond(Response.json(preview({ evidence: { ...preview().evidence, favorites_synced_at: '2026-11-20T08:30:00Z' } })))
  render(<App />)
  await loaded()
  const synced = evidence().getByText('Last Panda favorite sync').nextElementSibling!
  expect(synced.querySelector('time')?.dateTime).toBe('2026-11-20T08:30:00Z')
})

test('leaves out the new favorites while the collector does not count them', async () => {
  respond(Response.json(preview({ evidence: { ...preview().evidence, new_favorites: null } })))
  render(<App />)
  await loaded()
  expect(evidence().getByText('Favorites in scope')).toBeTruthy()
  expect(evidence().queryByText('New favorites since the active model was calculated')).toBeNull()
})

test('opens on the weights tab and lists every weight of the preview', async () => {
  const weights = Array.from({ length: 120 }, (_, index) => weight(index))
  respond(Response.json(preview({ candidate: { ...preview().candidate!, weight_count: 3100, weights } })))
  render(<App />)
  await loaded()
  expect(screen.getAllByRole('tab').map((item) => [item.textContent, item.getAttribute('aria-selected')])).toEqual(
    [['Weights', 'true'], ['Pins', 'false'], ['Favorite authors', 'false'], ['Model', 'false']].map(([name, selected]) => [tabName(name), selected]))
  expect(screen.getByText('120 of 3,100 weights, strongest first.')).toBeTruthy()
  expect(rows()).toHaveLength(120)
  expect(rows()[119][0]).toBe('tag:value 119')
  expect(screen.queryByRole('button', { name: 'Show more' })).toBeNull()
})

function filterWeights() {
  return weightsPanel().getByRole('textbox', { name: 'Filter weights' }) as HTMLInputElement
}

async function typeFilter(input: HTMLInputElement, text: string) {
  await userEvent.clear(input)
  await userEvent.type(input, text)
}

test('the weights filter searches every weight of the preview by its label, ignoring case and spaces around the text', async () => {
  const weights = Array.from({ length: 120 }, (_, index) => weight(index))
  respond(
    Response.json(preview({ candidate: { ...preview().candidate!, weights } })),
    Response.json({ id: 1, kind: 'tag', label: 'tag:value 0', weight: 0 }), Response.json(preview({ candidate: { ...preview().candidate!, weight_count: 3 } })),
  )
  render(<App />)
  await screen.findByText('120 of 3,100 weights, strongest first.')
  expect(filterWeights().placeholder).toBe('Filter, or type a tag as namespace:value')
  await typeFilter(filterWeights(), ' VALUE 11 ')
  expect(rows().map(([label]) => label)).toEqual(['tag:value 11', ...Array.from({ length: 10 }, (_, index) => `tag:value ${110 + index}`)])
  expect(screen.getByText('11 matching weights, strongest first. Only the 120 strongest of 3,100 weights are searched.')).toBeTruthy()
  await typeFilter(filterWeights(), 'zzz')
  expect(weightsPanel().queryByRole('table')).toBeNull()
  expect(screen.getByText('No weights match. Only the 120 strongest of 3,100 weights are searched.')).toBeTruthy()
  await typeFilter(filterWeights(), '   ')
  expect(screen.getByText('120 of 3,100 weights, strongest first.')).toBeTruthy()

  await userEvent.click(button(weightRow('tag:value 0'), 'Ignore'))
  await screen.findByText('3 of 3 weights, strongest first.')
  await typeFilter(filterWeights(), 'uploader')
  expect(rows().map(([label]) => label)).toEqual(['Uploader Some uploader'])
  expect(screen.getByText('1 matching weights, strongest first.')).toBeTruthy()
  await typeFilter(filterWeights(), 'zzz')
  expect(screen.getByText('No weights match.')).toBeTruthy()
})

test('the weights filter stays after a pin change and a change of tab', async () => {
  const body = preview()
  const pin: ScorePin = { id: 9, kind: 'tag', label: 'female:glasses', weight: 1 }
  respond(Response.json(body), Response.json(pin), Response.json({ ...body, pins: [pin] }))
  render(<App />)
  await loaded()
  await typeFilter(filterWeights(), 'glasses')
  await userEvent.type(pinField('female:glasses'), '1')
  await userEvent.click(button(weightRow('female:glasses'), 'Pin'))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  await waitFor(() => expect(listedPins()).toEqual(['female:glasses 1']))
  expect(filterWeights().value).toBe('glasses')
  expect(rows().map(([label]) => label)).toEqual(['female:glasses'])
})

test('a typed tag that no listed weight has gets a row that pins it in its saved spelling', async () => {
  const unlisted: ScorePin = { id: 9, kind: 'tag', label: 'artist:some artist', weight: 2 }
  respond(Response.json(preview()), Response.json(unlisted))
  fetchMock.mockReturnValueOnce(new Promise(() => {}))
  render(<App />)
  await loaded()
  await typeFilter(filterWeights(), ' A:Some_Artist ')
  expect(screen.getByText(/^No listed weight has this tag\./)).toBeTruthy()
  expect(rows()).toEqual([['artist:some artist', '–', '–', '–', '–']])
  expect(weightRow('artist:some artist').getByRole('button', { name: 'Ignore' })).toBeTruthy()
  expect(weightRow('artist:some artist').queryByRole('button', { name: 'Remove pin' })).toBeNull()
  await userEvent.type(pinField('artist:some artist'), '2{Enter}')
  await waitFor(() => expect(listedPins()).toEqual(['artist:some artist 2']))
  expect(weightRow('artist:some artist').getByRole('button', { name: 'Remove pin' })).toBeTruthy()

  await typeFilter(filterWeights(), 'female:glass')
  expect(rows().map(([label]) => label)).toEqual(['female:glass', 'female:glasses'])
  await typeFilter(filterWeights(), 'f:GLASSES')
  expect(rows().map(([label]) => label)).toEqual(['female:glasses'])
  await typeFilter(filterWeights(), 'glasses')
  expect(rows().map(([label]) => label)).toEqual(['female:glasses'])
  await typeFilter(filterWeights(), 'nonamespace:')
  expect(weightsPanel().queryByRole('table')).toBeNull()
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/pins', 'POST', '{"kind":"tag","value":"artist:some artist","weight":2}']])
})

const fewFavorites = 'Fewer than 50 favorites are in scope. Weights and the measurement from so little evidence are unreliable.'

test('warns about few favorites and still shows the table', async () => {
  respond(Response.json(preview({ evidence: { ...preview().evidence, few_favorites: true } })))
  render(<App />)
  await loaded()
  expect(newModel().getByText(fewFavorites)).toBeTruthy()
  expect(weightsPanel().getByRole('table')).toBeTruthy()
})

test('without a favorite in scope says that no model is possible and keeps the active model, pins and authors', async () => {
  respond(Response.json(preview({
    candidate: null, no_model: 'no_favorites', evidence: { ...preview().evidence, favorites: 0, few_favorites: true },
    pins: [glassesPin], favorite_author_count: 1, favorite_authors: [author(0)],
  })))
  render(<App />)
  await loaded()
  expect(newModel().getByText('No model is possible: no Panda favorite is in scope. Sync your Panda favorites, or widen the default filter.')).toBeTruthy()
  expect(weightsPanel().queryByRole('table')).toBeNull()
  expect(tab('Model').queryByRole('spinbutton', { name: 'Threshold' })).toBeNull()
  expect(tab('Model').queryByRole('button', { name: 'Apply model and threshold' })).toBeNull()
  expect(screen.queryByText(fewFavorites)).toBeNull()
  expect(activeModel().getByText(/threshold -0\.5/)).toBeTruthy()
  expect(field('Active threshold').value).toBe('-0.5')
  expect(listedPins()).toEqual(['female:glasses 2'])
  expect(listedAuthors()).toEqual([['artist:author 0', '2 favorites', '–']])
})

test('while the catalog in scope is counted, says how much is done instead of offering a model', async () => {
  respond(Response.json(preview({ candidate: null, no_model: 'counts_preparing', counts_progress: 42 })))
  render(<App />)
  await loaded()
  expect(newModel().getByText('The collector counts the catalog inside the default filter: 42 % done. The page calculates a model when the counts are complete.')).toBeTruthy()
  expect(weightsPanel().queryByRole('table')).toBeNull()
  expect(tab('Model').queryByRole('button', { name: 'Apply model and threshold' })).toBeNull()
})

test('a candidate without weights offers its threshold and an empty weights table', async () => {
  respond(Response.json(preview({ candidate: { ...preview().candidate!, weight_count: 0, weights: [] } })))
  render(<App />)
  await loaded()
  expect(field('Threshold').value).toBe('-0.5')
  expect(button(newModel(), 'Apply model and threshold').disabled).toBe(false)
  expect(weightsPanel().getByRole('table')).toBeTruthy()
  expect(rows()).toEqual([])
})

test('without an active model says so and leaves out the new favorites since it', async () => {
  respond(Response.json(preview({ active: null })))
  render(<App />)
  await loaded()
  expect(evidence().queryByText('New favorites since the active model was calculated')).toBeNull()
  expect(activeModel().getByText('No model is active.')).toBeTruthy()
})

test('reports an unavailable collector and calculates again on retry', async () => {
  respond(Response.json({ error: 'collector_unavailable' }, { status: 502 }), Response.json(preview()))
  render(<App />)
  const alert = await screen.findByRole('alert')
  expect(alert.textContent).toContain('The request could not be confirmed.')
  expect(screen.queryByRole('region', { name: 'Evidence' })).toBeNull()
  await userEvent.click(within(alert).getByRole('button', { name: 'Retry' }))
  await loaded()
  expect(previewBodies()).toEqual(['{}', '{}'])
  expect(screen.queryByRole('alert')).toBeNull()
})

function histogram(firstBucket: number, counts: Record<number, number>) {
  const last = Math.max(...Object.keys(counts).map(Number))
  return { first_bucket: firstBucket, counts: Array.from({ length: last - firstBucket + 1 }, (_, index) => counts[firstBucket + index] ?? 0) }
}

// At -0.8: held-out favorites 10 %, recent 8 %. At -0.5: 15, 18 %. At -0.2: 25, 38 %.
// The active histogram hides 18 % at -0.8, 33 % at -0.5 and 48 % at -0.2.
function measured(fields: Partial<ScorePreview> = {}): ScorePreview {
  return preview({
    candidate: {
      ...preview().candidate!,
      held_out: { total: 20, scores: histogram(-9, { [-9]: 2, [-6]: 1, [-3]: 2, 0: 15 }) },
    },
    recent: {
      total: 200,
      candidate: histogram(-9, { [-9]: 16, [-6]: 20, [-3]: 40, 0: 124 }),
      active: histogram(-9, { [-9]: 36, [-6]: 30, [-3]: 30, 0: 104 }),
    },
    ...fields,
  })
}

// A recalculated candidate that hides 10 % of the held-out favorites and 28 % of the recent sample at -0.2.
function recalculated(): ScorePreview {
  const base = measured()
  return {
    ...base,
    candidate: {
      ...base.candidate!, id: 9, suggested_threshold_tenths: -3,
      held_out: { total: 10, scores: histogram(-4, { [-4]: 1, 0: 9 }) },
    },
    recent: { ...base.recent, candidate: histogram(-4, { [-4]: 56, 0: 144 }) },
  }
}

const noContent = () => new Response(null, { status: 204 })

function newModel() {
  return region('Model', 'New model')
}

function activeModel() {
  return region('Model', 'Active model')
}

/** A threshold field of the Model tab. */
function field(name: string) {
  return tab('Model').getByRole('spinbutton', { name }) as HTMLInputElement
}

async function enter(name: string, value: string) {
  await userEvent.clear(field(name))
  await userEvent.type(field(name), value)
}

function changeCalls() {
  return fetchMock.mock.calls.filter(([input]) => input !== '/api/collector/catalog/score/preview').map(([input, init]) => [input, init?.method, init?.body])
}

function expectEffect(heldOut: number, recent: number, now: number) {
  const section = newModel()
  expect(section.getByText(/held-out favorites\. /).textContent).toBe(`Hides ${heldOut} % of the held-out favorites. A favorite is held out when its weights come from the other favorites only.`)
  expect(section.getByText(/catalog galleries \(now/).textContent).toBe(`Hides ${recent} % of the newest 200 catalog galleries (now: ${now} %)`)
}

const suggested = (threshold: string) => `Suggested: ${threshold} (hides at most 5 % of the held-out favorites)`

test('with an active model the threshold starts at the active threshold and follows the field without a request', async () => {
  respond(Response.json(measured()))
  render(<App />)
  await loaded()
  expect(field('Threshold').value).toBe('-0.5')
  expect(newModel().getByText(suggested('-0.8'))).toBeTruthy()
  expectEffect(15, 18, 33)
  await enter('Threshold', '-0.2')
  expectEffect(25, 38, 33)
  expect(field('Active threshold').value).toBe('-0.5')
  expect(activeModel().getByText(/^Hides/).textContent).toBe('Hides 33 % of the newest 200 catalog galleries')
  await enter('Active threshold', '-0.8')
  expect(activeModel().getByText(/^Hides/).textContent).toBe('Hides 18 % of the newest 200 catalog galleries')
  expect(fetchMock).toHaveBeenCalledTimes(1)
})

test('without an active model nothing is hidden now and the field starts at the suggested threshold', async () => {
  respond(Response.json(measured({ active: null, recent: { ...measured().recent, active: null } })))
  render(<App />)
  await loaded()
  expect(field('Threshold').value).toBe('-0.8')
  expect(newModel().getByText(suggested('-0.8'))).toBeTruthy()
  expectEffect(10, 8, 0)
  expect(tab('Model').queryByRole('spinbutton', { name: 'Active threshold' })).toBeNull()
  expect(tab('Model').queryByRole('button', { name: 'Save threshold' })).toBeNull()
  expect(tab('Model').queryByRole('button', { name: 'Turn off' })).toBeNull()
})

test('the entered threshold stays across a new calculation and applies the candidate on screen', async () => {
  respond(
    Response.json(measured()), Response.json({ ...glassesPin, weight: 0 }), Response.json(recalculated()),
    noContent(), Response.json(measured({ active: { ...measured().active!, threshold_tenths: -2 } })),
  )
  render(<App />)
  await loaded()
  await enter('Threshold', '-0.2')
  await userEvent.click(button(weightRow('female:glasses'), 'Ignore'))
  await newModel().findByText(suggested('-0.3'))
  expect(field('Threshold').value).toBe('-0.2')
  expectEffect(10, 28, 33)
  await userEvent.click(newModel().getByRole('button', { name: 'Apply model and threshold' }))
  expect(await newModel().findByText('Model applied.')).toBeTruthy()
  expect(changeCalls().at(-1)).toEqual(['/api/collector/catalog/score/apply', 'POST', '{"candidate_id":9,"threshold_tenths":-2}'])
  await waitFor(() => expect(previewBodies()).toEqual(['{}', measure7, '{}']))
})

test('after a successful change both fields start again from the next preview', async () => {
  respond(
    Response.json(measured()), noContent(), Response.json(measured({ active: { ...measured().active!, threshold_tenths: -3 } })),
    noContent(), Response.json(measured({ active: null })),
  )
  render(<App />)
  await loaded()
  await enter('Threshold', '-0.2')
  await enter('Active threshold', '-0.3')
  await userEvent.click(tab('Model').getByRole('button', { name: 'Save threshold' }))
  await activeModel().findByText('Threshold saved.')
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/threshold', 'PUT', '{"threshold_tenths":-3}']])
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  await waitFor(() => expect(field('Threshold').value).toBe('-0.3'))
  expect(field('Active threshold').value).toBe('-0.3')

  await userEvent.click(tab('Model').getByRole('button', { name: 'Turn off' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog.textContent).toContain("Turn off the score model?The model is deleted and can't be restored. The listing then hides nothing by score.")
  expect(changeCalls()).toHaveLength(1)
  await userEvent.click(within(dialog).getByRole('button', { name: 'Turn off' }))
  await activeModel().findByText('No model is active.')
  expect(changeCalls()[1]).toEqual(['/api/collector/catalog/score/model', 'DELETE', undefined])
  expect(previewBodies()).toHaveLength(3)
  expect(field('Threshold').value).toBe('-0.8')
  expect(tab('Model').queryByRole('spinbutton', { name: 'Active threshold' })).toBeNull()
})

test('a failed apply keeps the candidate and the field, and calculates again only for a replaced candidate', async () => {
  respond(
    Response.json(measured()),
    Response.json({ error: 'collector_unavailable' }, { status: 502 }),
    Response.json({ error: 'score_candidate_not_found' }, { status: 409 }),
    Response.json(recalculated()),
  )
  render(<App />)
  await loaded()
  await enter('Threshold', '-0.2')
  await userEvent.click(tab('Model').getByRole('button', { name: 'Apply model and threshold' }))
  const alert = await newModel().findByRole('alert')
  expect(alert.textContent).toMatch(/^Could not apply the model\. The request could not be confirmed\./)
  expect(previewBodies()).toHaveLength(1)
  expect(field('Threshold').value).toBe('-0.2')
  expect(weightsPanel().getByRole('table')).toBeTruthy()
  await userEvent.click(tab('Model').getByRole('button', { name: 'Apply model and threshold' }))
  await newModel().findByText('Could not apply the model. This calculation is no longer available. Check the new calculation and apply again.')
  await newModel().findByText(suggested('-0.3'))
  expect(previewBodies()).toEqual(['{}', '{}'])
  expect(field('Threshold').value).toBe('-0.2')
  expect(newModel().getByText(/Check the new calculation/)).toBeTruthy()
})

test('an empty threshold disables the apply button, few favorites do not', async () => {
  respond(Response.json(measured({ evidence: { ...measured().evidence, few_favorites: true } })))
  render(<App />)
  await loaded()
  const apply = newModel().getByRole('button', { name: 'Apply model and threshold' }) as HTMLButtonElement
  expect(apply.disabled).toBe(false)
  await userEvent.clear(field('Threshold'))
  expect(apply.disabled).toBe(true)
})

test('failures to save the threshold or turn off the model show a message and request nothing', async () => {
  respond(
    Response.json(measured()),
    Response.json({ error: 'invalid_threshold' }, { status: 400 }),
    Response.json({ error: 'score_model_not_active' }, { status: 409 }),
  )
  render(<App />)
  await loaded()
  await userEvent.click(tab('Model').getByRole('button', { name: 'Save threshold' }))
  await activeModel().findByText('Could not save the threshold. Enter a valid threshold.')
  await userEvent.click(tab('Model').getByRole('button', { name: 'Turn off' }))
  await userEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Cancel' }))
  expect(screen.queryByRole('alertdialog')).toBeNull()
  expect(changeCalls()).toHaveLength(1)
  await userEvent.click(tab('Model').getByRole('button', { name: 'Turn off' }))
  await userEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Turn off' }))
  await activeModel().findByText('Could not turn off the model. No score model is active any more.')
  expect(previewBodies()).toHaveLength(1)
})

test('a calculation in progress and a change in progress lock each other out', async () => {
  let finishPreview!: (response: Response) => void
  let finishChange!: (response: Response) => void
  respond(Response.json(measured()), Response.json({ ...glassesPin, weight: 0 }))
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishPreview = resolve }))
  render(<App />)
  await loaded()
  const ignore = () => button(weightRow('female:glasses'), 'Ignore')
  const save = () => tab('Model').getByRole('button', { name: 'Save threshold' }) as HTMLButtonElement
  const turnOff = () => tab('Model').getByRole('button', { name: 'Turn off' }) as HTMLButtonElement

  await userEvent.click(ignore())
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(save().disabled).toBe(true)
  expect(turnOff().disabled).toBe(true)
  finishPreview(Response.json(recalculated()))
  await newModel().findByText(suggested('-0.3'))
  expect(save().disabled).toBe(false)

  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishChange = resolve }))
  respond(Response.json(recalculated()))
  await userEvent.click(save())
  expect(ignore().disabled).toBe(true)
  finishChange(noContent())
  await activeModel().findByText('Threshold saved.')
  await waitFor(() => expect(previewBodies()).toEqual(['{}', measure7, '{}']))
})

test('Retry waits for a change in progress', async () => {
  let finishChange!: (response: Response) => void
  respond(Response.json(measured()), Response.json({ ...glassesPin, weight: 0 }), Response.json({ error: 'collector_unavailable' }, { status: 502 }))
  render(<App />)
  await loaded()
  await userEvent.click(button(weightRow('female:glasses'), 'Ignore'))
  const retry = within(await screen.findByRole('alert')).getByRole('button', { name: 'Retry' }) as HTMLButtonElement
  expect(retry.disabled).toBe(false)
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishChange = resolve }))
  await userEvent.click(tab('Model').getByRole('button', { name: 'Apply model and threshold' }))
  expect(retry.disabled).toBe(true)
  respond(Response.json(measured()))
  finishChange(noContent())
  await newModel().findByText('Model applied.')
  await waitFor(() => expect(previewBodies()).toEqual(['{}', measure7, '{}']))
})

function weightRow(label: string) {
  return within(within(weightsPanel().getByRole('table')).getByText(label).closest('tr')!)
}

function pinForm() {
  return within(screen.getByRole('form', { name: 'Add pin' }))
}

const formTag = () => pinForm().getByRole('combobox', { name: 'Tag' }) as HTMLInputElement
const formWeight = () => pinForm().getByRole('spinbutton', { name: 'Weight' }) as HTMLInputElement
const formAdd = () => button(pinForm(), 'Add')

test('the pin form pins or ignores a tag, an uploader or a category, also without a candidate or an active model', async () => {
  const otherPin: ScorePin = { id: 8, kind: 'tag', label: 'other:tag', weight: 1.5 }
  const someonePin: ScorePin = { id: 9, kind: 'uploader', label: 'Someone', weight: 0 }
  const cosplayPin: ScorePin = { id: 10, kind: 'category', label: 'Cosplay', weight: -2 }
  const empty = preview({ candidate: null, no_model: 'no_favorites', active: null })
  respond(
    Response.json(empty),
    Response.json(otherPin), Response.json({ ...empty, pins: [otherPin] }),
    Response.json(someonePin), Response.json({ ...empty, pins: [otherPin, someonePin] }),
    Response.json(cosplayPin), Response.json({ ...empty, pins: [otherPin, someonePin, cosplayPin] }),
  )
  render(<App />)
  await loaded()
  expect(within(pinForm().getByRole('radiogroup', { name: 'Action' })).getAllByRole('radio').map((item) => item.textContent)).toEqual(['Pin', 'Ignore'].map(tabName))
  expect(formWeight().step).toBe('any')
  expect(formAdd().disabled).toBe(true)
  await userEvent.type(formTag(), '   ')
  await userEvent.type(formWeight(), '1.5')
  expect(formAdd().disabled).toBe(true)
  await userEvent.type(formTag(), 'other:tag  ')
  await userEvent.clear(formWeight())
  expect(formAdd().disabled).toBe(true)
  await userEvent.type(formWeight(), '1.5')
  await userEvent.click(formAdd())
  await waitFor(() => expect(listedPins()).toEqual(['other:tag 1.5']))
  expect(formTag().value).toBe('')
  expect(formWeight().value).toBe('')

  await userEvent.click(pinForm().getByRole('radio', { name: 'Uploader' }))
  await userEvent.click(pinForm().getByRole('radio', { name: 'Ignore' }))
  expect(pinForm().queryByRole('spinbutton', { name: 'Weight' })).toBeNull()
  await userEvent.type(pinForm().getByRole('textbox', { name: 'Uploader' }), ' Someone ')
  await userEvent.click(formAdd())
  await waitFor(() => expect(listedPins()).toEqual(['other:tag 1.5', 'Uploader Someone 0']))

  await userEvent.click(pinForm().getByRole('radio', { name: 'Category' }))
  await userEvent.click(pinForm().getByRole('radio', { name: 'Pin' }))
  expect(pinForm().queryByRole('textbox')).toBeNull()
  await userEvent.selectOptions(pinForm().getByRole('combobox', { name: 'Category' }), 'Cosplay')
  await userEvent.type(formWeight(), '-2')
  await userEvent.click(formAdd())
  await waitFor(() => expect(listedPins()).toEqual(['other:tag 1.5', 'Uploader Someone 0', 'Category Cosplay -2']))
  await waitFor(() => expect(previewBodies()).toHaveLength(4))
  expect(changeCalls()).toEqual([
    ['/api/collector/catalog/score/pins', 'POST', '{"kind":"tag","value":"other:tag","weight":1.5}'],
    ['/api/collector/catalog/score/pins', 'POST', '{"kind":"uploader","value":"Someone","weight":0}'],
    ['/api/collector/catalog/score/pins', 'POST', '{"kind":"category","value":"Cosplay","weight":-2}'],
  ])
  expect(previewBodies()).toEqual(['{}', '{}', '{}', '{}'])
})

test('a failed change from the pin form shows its message in the form, keeps the fields and calculates nothing', async () => {
  respond(
    Response.json(preview()),
    Response.json({ error: 'tag_not_found' }, { status: 404 }),
    Response.json({ error: 'collector_unavailable' }, { status: 502 }),
  )
  render(<App />)
  await loaded()
  await userEvent.type(formTag(), 'no:such tag')
  await userEvent.type(formWeight(), '1')
  await userEvent.click(formAdd())
  expect((await pinForm().findByRole('alert')).textContent).toBe('Could not change the pin. No gallery in the Panda catalog has this tag.')
  expect(formTag().value).toBe('no:such tag')
  expect(formWeight().value).toBe('1')
  expect(listedPins()).toEqual([])
  expect(previewBodies()).toHaveLength(1)

  await userEvent.click(button(weightRow('Uploader Some uploader'), 'Ignore'))
  expect((await weightsPanel().findByRole('alert')).textContent).toMatch(/^Could not change the pin\. The request could not be confirmed\./)
  expect(pinForm().queryByRole('alert')).toBeNull()
  expect(listedPins()).toEqual([])
  expect(previewBodies()).toHaveLength(1)
})

function pinsSection() {
  return region('Pins', 'Pinned weights')
}

function listedPins() {
  const table = pinsSection().queryByRole('table')
  return table ? within(table).getAllByRole('row').slice(1).map((item) => `${item.querySelector('[data-pin-label]')?.textContent} ${(within(item).getByRole('spinbutton') as HTMLInputElement).value}`) : []
}

function listField(label: string) {
  return pinsSection().getByRole('spinbutton', { name: `Weight of pin ${label}` }) as HTMLInputElement
}

function listRow(label: string) {
  return within(listField(label).closest('tr')!)
}

function pinField(label: string, scope = weightsPanel()) {
  return scope.getByRole('spinbutton', { name: `Pinned weight of ${label}` }) as HTMLInputElement
}

function button(scope: ReturnType<typeof within>, name: string) {
  return scope.getByRole('button', { name }) as HTMLButtonElement
}

const glassesPin: ScorePin = { id: 3, kind: 'tag', label: 'female:glasses', weight: 2 }
const uploaderPin: ScorePin = { id: 4, kind: 'uploader', label: 'Some uploader', weight: -1.5 }

test('a row shows the calculated weight and the pin, and the list shows every pin', async () => {
  const elsewhere: ScorePin = { id: 5, kind: 'category', label: 'Cosplay', weight: 0 }
  respond(Response.json(preview({ pins: [glassesPin, elsewhere] })))
  render(<App />)
  await loaded()
  expect(rows()[0]).toEqual(['female:glasses', '+1.42', '31', '1,200', '+1.10'])
  expect(pinField('female:glasses').value).toBe('2')
  expect(weightRow('female:glasses').getByRole('button', { name: 'Remove pin' })).toBeTruthy()
  expect(pinField('Uploader Some uploader').value).toBe('')
  expect(button(weightRow('Uploader Some uploader'), 'Pin').disabled).toBe(true)
  expect(weightRow('Uploader Some uploader').queryByRole('button', { name: 'Remove pin' })).toBeNull()
  expect(listedPins()).toEqual(['female:glasses 2', 'Category Cosplay 0'])
  expect(pinsSection().getAllByRole('button', { name: 'Remove' })).toHaveLength(2)
  expect(pinsSection().queryByText(/No score model is active/)).toBeNull()
})

test('the pinned weights show also without a candidate, and say without an active model that they have no effect', async () => {
  respond(Response.json(preview({ candidate: null, no_model: 'no_favorites', active: null })))
  render(<App />)
  await loaded()
  expect(pinsSection().getByText('No score model is active: pinned weights have no effect.')).toBeTruthy()
  expect(pinsSection().getByText('No pinned weights.')).toBeTruthy()
  expect(listedPins()).toEqual([])
})

test('Pin, Ignore and Remove pin change the pin at once, keep the threshold and measure the candidate again', async () => {
  respond(
    Response.json(measured()),
    Response.json(uploaderPin), Response.json(measured({ pins: [uploaderPin] })),
    Response.json({ ...glassesPin, weight: 0 }), Response.json(measured({ pins: [uploaderPin, { ...glassesPin, weight: 0 }] })),
  )
  let finishPreview!: (response: Response) => void
  render(<App />)
  await loaded()
  await enter('Threshold', '-0.2')
  await userEvent.type(pinField('Uploader Some uploader'), '-1.5')
  await userEvent.click(button(weightRow('Uploader Some uploader'), 'Pin'))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(listedPins()).toEqual(['Uploader Some uploader -1.5'])
  await userEvent.click(button(weightRow('female:glasses'), 'Ignore'))
  await waitFor(() => expect(previewBodies()).toHaveLength(3))
  await waitFor(() => expect(listedPins()).toEqual(['Uploader Some uploader -1.5', 'female:glasses 0']))
  expect(pinField('female:glasses').value).toBe('0')

  fetchMock.mockResolvedValueOnce(noContent())
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishPreview = resolve }))
  await userEvent.click(button(weightRow('Uploader Some uploader'), 'Remove pin'))
  await waitFor(() => expect(listedPins()).toEqual(['female:glasses 0']))
  expect(pinField('Uploader Some uploader').value).toBe('')
  expect(weightRow('Uploader Some uploader').queryByRole('button', { name: 'Remove pin' })).toBeNull()
  finishPreview(Response.json(measured({ pins: [{ ...glassesPin, weight: 0 }] })))
  await waitFor(() => expect(previewBodies()).toHaveLength(4))

  expect(changeCalls()).toEqual([
    ['/api/collector/catalog/score/pins', 'POST', '{"kind":"uploader","value":"Some uploader","weight":-1.5}'],
    ['/api/collector/catalog/score/pins', 'POST', '{"kind":"tag","value":"female:glasses","weight":0}'],
    ['/api/collector/catalog/score/pins/4', 'DELETE', undefined],
  ])
  expect(previewBodies()).toEqual(['{}', measure7, measure7, measure7])
  await waitFor(() => expect(button(newModel(), 'Apply model and threshold').disabled).toBe(false))
  expect(field('Threshold').value).toBe('-0.2')
})

test('a saved pin replaces the pin of its value or is added before the calculation answers', async () => {
  let finishFirst!: (response: Response) => void
  let finishSecond!: (response: Response) => void
  respond(Response.json(preview({ pins: [glassesPin] })), Response.json({ ...glassesPin, weight: 3 }))
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishFirst = resolve }))
  render(<App />)
  await loaded()
  await userEvent.clear(pinField('female:glasses'))
  await userEvent.type(pinField('female:glasses'), '3')
  await userEvent.click(button(weightRow('female:glasses'), 'Pin'))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(listedPins()).toEqual(['female:glasses 3'])
  expect(pinField('female:glasses').value).toBe('3')

  respond(Response.json(uploaderPin))
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishSecond = resolve }))
  await userEvent.type(pinField('Uploader Some uploader'), '-1.5')
  await userEvent.click(button(weightRow('Uploader Some uploader'), 'Pin'))
  await waitFor(() => expect(previewBodies()).toHaveLength(3))
  expect(listedPins()).toEqual(['female:glasses 3', 'Uploader Some uploader -1.5'])
  expect(pinField('Uploader Some uploader').value).toBe('-1.5')
  expect(weightRow('Uploader Some uploader').getByRole('button', { name: 'Remove pin' })).toBeTruthy()

  const pins = [{ ...glassesPin, weight: 3 }, uploaderPin]
  finishFirst(Response.json(preview({ pins: [{ ...glassesPin, weight: 3 }] })))
  finishSecond(Response.json(preview({ pins })))
  await waitFor(() => expect(button(newModel(), 'Apply model and threshold').disabled).toBe(false))
  expect(listedPins()).toEqual(['female:glasses 3', 'Uploader Some uploader -1.5'])
})

test('each label of the table carries its full label as title, and the pin buttons are icons with titles', async () => {
  respond(Response.json(preview({ pins: [glassesPin] })))
  render(<App />)
  await loaded()
  for (const label of ['female:glasses', 'Uploader Some uploader', 'Category Doujinshi']) {
    expect(within(weightsPanel().getByRole('table')).getByText(label).getAttribute('title')).toBe(label)
  }
  for (const [name, title] of [['Pin', 'Pin the entered weight'], ['Ignore', 'Ignore: pin the weight 0'], ['Remove pin', 'Remove pin']]) {
    const icon = button(weightRow('female:glasses'), name)
    expect(icon.textContent).toBe('')
    expect(icon.title).toBe(title)
  }
  expect(weightRow('Category Doujinshi').queryByRole('button', { name: 'Remove pin' })).toBeNull()
})

test('Enter in a pin field pins its number, and does nothing without one', async () => {
  respond(Response.json(preview()), Response.json(uploaderPin))
  fetchMock.mockReturnValueOnce(new Promise(() => {}))
  render(<App />)
  await loaded()
  await userEvent.type(pinField('female:glasses'), '{Enter}')
  await userEvent.type(pinField('Uploader Some uploader'), '-1.5{Enter}')
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/pins', 'POST', '{"kind":"uploader","value":"Some uploader","weight":-1.5}']])
})

test('Remove in the list removes that pin', async () => {
  respond(Response.json(preview({ pins: [glassesPin, uploaderPin] })), noContent(), Response.json(preview({ pins: [glassesPin] })))
  render(<App />)
  await loaded()
  await userEvent.click(within(pinsSection().getByText('Uploader Some uploader').closest('tr')!).getByRole('button', { name: 'Remove' }))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/pins/4', 'DELETE', undefined]])
  expect(previewBodies()).toEqual(['{}', measure7])
  expect(listedPins()).toEqual(['female:glasses 2'])
})

test('each row of the list edits its pin and shows the weight of its value in the new model', async () => {
  const cosplayPin: ScorePin = { id: 5, kind: 'category', label: 'Cosplay', weight: 0 }
  respond(Response.json(preview({ pins: [{ ...glassesPin, model_weight: -0.5 }, { ...uploaderPin, model_weight: 0.5 }, cosplayPin] })))
  render(<App />)
  await loaded()
  expect(listedPins()).toEqual(['female:glasses 2', 'Uploader Some uploader -1.5', 'Category Cosplay 0'])
  for (const label of ['female:glasses', 'Uploader Some uploader', 'Category Cosplay']) {
    expect(pinsSection().getByText(label).getAttribute('title')).toBe(label)
    expect(listField(label).step).toBe('any')
    expect(listRow(label).getByRole('button', { name: 'Remove' })).toBeTruthy()
    expect(listRow(label).queryByRole('button', { name: 'Save' })).toBeNull()
  }
  expect(pinsSection().getByRole('columnheader', { name: 'Model weight' })).toBeTruthy()
  expect(listRow('female:glasses').getByText('-0.50')).toBeTruthy()
  expect(listRow('Uploader Some uploader').getByText('+0.50')).toBeTruthy()
  expect(listRow('Category Cosplay').getByText('–')).toBeTruthy()
  expect(pinsSection().queryByRole('button', { name: 'Ignore' })).toBeNull()
})

test('a pin new in this visit shows its model weight in the list once the calculation that follows answers', async () => {
  let finishPreview!: (response: Response) => void
  respond(Response.json(preview()), Response.json(uploaderPin))
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishPreview = resolve }))
  render(<App />)
  await loaded()
  await userEvent.type(pinField('Uploader Some uploader'), '-1.5{Enter}')
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(listedPins()).toEqual(['Uploader Some uploader -1.5'])
  expect(pinsSection().queryByRole('columnheader', { name: 'Model weight' })).toBeNull()
  finishPreview(Response.json(preview({ pins: [{ ...uploaderPin, model_weight: 0.5 }] })))
  expect(await listRow('Uploader Some uploader').findByText('+0.50')).toBeTruthy()
})

test('Enter or Save in a row of the list saves the typed weight, and the row keeps its model weight', async () => {
  let finishPreview!: (response: Response) => void
  respond(Response.json(preview({ pins: [{ ...glassesPin, model_weight: -0.5 }, uploaderPin] })), Response.json({ ...glassesPin, weight: 3 }))
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishPreview = resolve }))
  render(<App />)
  await loaded()
  await userEvent.clear(listField('female:glasses'))
  await userEvent.type(listField('female:glasses'), '3')
  expect(button(listRow('female:glasses'), 'Save').title).toBe('Save the weight')
  expect(button(listRow('female:glasses'), 'Save').disabled).toBe(false)
  await userEvent.keyboard('{Enter}')
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(listField('female:glasses').value).toBe('3')
  expect(listRow('female:glasses').queryByRole('button', { name: 'Save' })).toBeNull()
  expect(listRow('female:glasses').getByText('-0.50')).toBeTruthy()
  finishPreview(Response.json(preview({ pins: [{ ...glassesPin, weight: 3, model_weight: -0.5 }, uploaderPin] })))
  await waitFor(() => expect(button(newModel(), 'Apply model and threshold').disabled).toBe(false))

  respond(Response.json({ ...uploaderPin, weight: 0 }), Response.json(preview({ pins: [{ ...glassesPin, weight: 3 }, { ...uploaderPin, weight: 0 }] })))
  await userEvent.clear(listField('Uploader Some uploader'))
  await userEvent.type(listField('Uploader Some uploader'), '0')
  await userEvent.click(button(listRow('Uploader Some uploader'), 'Save'))
  await waitFor(() => expect(previewBodies()).toHaveLength(3))
  await waitFor(() => expect(listedPins()).toEqual(['female:glasses 3', 'Uploader Some uploader 0']))
  expect(changeCalls()).toEqual([
    ['/api/collector/catalog/score/pins', 'POST', '{"kind":"tag","value":"female:glasses","weight":3}'],
    ['/api/collector/catalog/score/pins', 'POST', '{"kind":"uploader","value":"Some uploader","weight":0}'],
  ])
})

test('a row of the list saves nothing on leaving the field, for the same number, or without a number', async () => {
  respond(Response.json(preview({ pins: [glassesPin] })))
  render(<App />)
  await loaded()
  await userEvent.clear(listField('female:glasses'))
  expect(button(listRow('female:glasses'), 'Save').disabled).toBe(true)
  await userEvent.keyboard('{Enter}')
  await userEvent.type(listField('female:glasses'), '2.0')
  expect(listRow('female:glasses').queryByRole('button', { name: 'Save' })).toBeNull()
  await userEvent.keyboard('{Enter}')
  await userEvent.clear(listField('female:glasses'))
  await userEvent.type(listField('female:glasses'), '5')
  await userEvent.tab()
  expect(listField('female:glasses').value).toBe('5')
  expect(button(listRow('female:glasses'), 'Save').disabled).toBe(false)
  expect(changeCalls()).toEqual([])
  expect(previewBodies()).toHaveLength(1)
})

test('a recalculation keeps the text typed in a pin field until that pin gets another weight', async () => {
  const doujinshiPin: ScorePin = { id: 6, kind: 'category', label: 'Doujinshi', weight: 0 }
  respond(
    Response.json(preview({ pins: [glassesPin, uploaderPin] })),
    Response.json(doujinshiPin), Response.json(preview({ pins: [{ ...glassesPin }, { ...uploaderPin }, doujinshiPin] })),
    Response.json(doujinshiPin), Response.json(preview({ pins: [{ ...glassesPin, weight: 4 }, { ...uploaderPin, weight: 5 }, doujinshiPin] })),
  )
  render(<App />)
  await loaded()
  await userEvent.clear(listField('female:glasses'))
  await userEvent.type(listField('female:glasses'), '7')
  await userEvent.clear(pinField('Uploader Some uploader'))
  await userEvent.type(pinField('Uploader Some uploader'), '9')
  const ignore = () => button(weightRow('Category Doujinshi'), 'Ignore')
  await userEvent.click(ignore())
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  await waitFor(() => expect(button(newModel(), 'Apply model and threshold').disabled).toBe(false))
  expect(listField('female:glasses').value).toBe('7')
  expect(pinField('Uploader Some uploader').value).toBe('9')
  await userEvent.click(ignore())
  await waitFor(() => expect(listField('female:glasses').value).toBe('4'))
  expect(pinField('Uploader Some uploader').value).toBe('5')
})

test('a failed save in the list shows a message in the section, keeps the text and calculates nothing', async () => {
  respond(Response.json(preview({ pins: [glassesPin] })), Response.json({ error: 'collector_unavailable' }, { status: 502 }))
  render(<App />)
  await loaded()
  await userEvent.clear(listField('female:glasses'))
  await userEvent.type(listField('female:glasses'), '3{Enter}')
  expect((await pinsSection().findByRole('alert')).textContent).toMatch(/^Could not change the pin\. The request could not be confirmed\./)
  expect(listField('female:glasses').value).toBe('3')
  expect(button(listRow('female:glasses'), 'Save').disabled).toBe(false)
  expect(previewBodies()).toHaveLength(1)
})

test('pins stay usable during the calculation that follows, and the newest calculation wins', async () => {
  let finishFirst!: (response: Response) => void
  let finishSecond!: (response: Response) => void
  respond(Response.json(measured()), Response.json(glassesPin))
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishFirst = resolve }))
  render(<App />)
  await loaded()
  await userEvent.type(pinField('female:glasses'), '2')
  await userEvent.click(button(weightRow('female:glasses'), 'Pin'))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(button(newModel(), 'Apply model and threshold').disabled).toBe(true)
  expect(button(weightRow('Uploader Some uploader'), 'Ignore').disabled).toBe(false)

  respond(Response.json({ ...uploaderPin, weight: 0 }))
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishSecond = resolve }))
  await userEvent.click(button(weightRow('Uploader Some uploader'), 'Ignore'))
  await waitFor(() => expect(previewBodies()).toHaveLength(3))
  expect(changeCalls()).toHaveLength(2)
  finishSecond(Response.json(recalculated()))
  await newModel().findByText(suggested('-0.3'))
  finishFirst(Response.json(measured({ candidate: { ...measured().candidate!, suggested_threshold_tenths: -1 } })))
  await waitFor(() => expect(button(newModel(), 'Apply model and threshold').disabled).toBe(false))
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(newModel().getByText(suggested('-0.3'))).toBeTruthy()
  expect(newModel().queryByText('Suggested: -0.1')).toBeNull()
})

test('a failed pin change shows a message beside the control, keeps the pins and calculates nothing', async () => {
  respond(
    Response.json(preview({ pins: [glassesPin] })),
    Response.json({ error: 'collector_unavailable' }, { status: 502 }),
    Response.json({ error: 'collector_unavailable' }, { status: 502 }),
  )
  render(<App />)
  await loaded()
  await userEvent.click(button(weightRow('Uploader Some uploader'), 'Ignore'))
  expect((await weightsPanel().findByRole('alert')).textContent).toMatch(/^Could not change the pin\. The request could not be confirmed\./)
  expect(pinsSection().queryByRole('alert')).toBeNull()
  expect(listedPins()).toEqual(['female:glasses 2'])
  expect(pinField('Uploader Some uploader').value).toBe('')

  await userEvent.click(button(pinsSection(), 'Remove'))
  expect((await pinsSection().findByRole('alert')).textContent).toMatch(/^Could not change the pin\. The request could not be confirmed\./)
  expect(newModel().queryByRole('alert')).toBeNull()
  expect(listedPins()).toEqual(['female:glasses 2'])
  expect(pinField('female:glasses').value).toBe('2')
  expect(previewBodies()).toHaveLength(1)
})

const longTags = { start: 0, end: 6, items: [
  { namespace: 'female', value: 'long hair', term: 'female:"long hair$"' },
  { namespace: 'female', value: 'long gloves', term: 'female:"long gloves$"' },
] }

test('the tag field of the pin form completes Panda tags, Enter chooses the plain tag and the next Enter adds the pin', async () => {
  const longHairPin: ScorePin = { id: 8, kind: 'tag', label: 'female:long hair', weight: 1.5 }
  completionsMock.mockImplementation(async () => Response.json(longTags))
  respond(Response.json(preview()), Response.json(longHairPin), Response.json(preview({ pins: [longHairPin] })))
  const user = userEvent.setup()
  render(<App />)
  await loaded()
  await user.type(formWeight(), '1.5')
  await user.type(formTag(), 'f:long')
  const suggestions = await pinForm().findAllByRole('option')
  expect(suggestions.map((option) => option.textContent)).toEqual(['femalelong hair', 'femalelong gloves'])
  expect(completionsMock.mock.calls.map(([input]) => String(input))).toContain(`${completionsPath}?q=f%3Along&cursor=6`)

  await user.keyboard('{Enter}')
  expect(formTag().value).toBe('female:long hair')
  expect(pinForm().queryByRole('option')).toBeNull()
  expect(changeCalls()).toEqual([])

  await user.keyboard('{Enter}')
  await waitFor(() => expect(previewBodies()).toEqual(['{}', measure7]))
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/pins', 'POST', '{"kind":"tag","value":"female:long hair","weight":1.5}']])
})

function authorsPanel() {
  return tab('Favorite authors')
}

function listedAuthors() {
  const list = authorsPanel().queryByRole('list')
  return list ? within(list).getAllByRole('listitem').map((item) => [...item.querySelectorAll('[data-author-cell]')].map((cell) => cell.textContent)) : []
}

function author(index: number, fields: Partial<ScoreFavoriteAuthor> = {}): ScoreFavoriteAuthor {
  return { label: `artist:author ${index}`, favorites: 2, weight: null, ...fields }
}

test('lists the favorite authors in the preview order, also without a candidate', async () => {
  respond(Response.json(preview({
    candidate: null, no_model: 'no_favorites', favorite_author_count: 1234,
    favorite_authors: [author(0, { label: 'artist:most', favorites: 31, weight: 1.5 }), author(1, { label: 'artist:fewer', favorites: 2, weight: null })],
  })))
  render(<App />)
  await loaded()
  expect(authorsPanel().getByText('1,234 favorite authors: artist tags that at least 2 Panda favorites carry.')).toBeTruthy()
  expect(listedAuthors()).toEqual([['artist:most', '31 favorites', '+1.50'], ['artist:fewer', '2 favorites', '–']])
  expect(authorsPanel().getByText('artist:most').getAttribute('title')).toBe('artist:most')
  expect(authorsPanel().queryByText(/No score model is active/)).toBeNull()
})

test('without an active model the section says that the author bonus has no effect', async () => {
  respond(Response.json(preview({ candidate: null, no_model: 'no_favorites', active: null })))
  render(<App />)
  await loaded()
  expect(authorsPanel().getByText('No score model is active: the author bonus has no effect.')).toBeTruthy()
})

test('Save bonus and Turn off bonus change the bonus, keep the threshold and measure the candidate again', async () => {
  respond(
    Response.json(measured()),
    noContent(), Response.json(measured({ author_bonus: 1.5 })),
    noContent(), Response.json(measured()),
  )
  render(<App />)
  await loaded()
  const bonus = () => authorsPanel().getByRole('spinbutton', { name: 'Author bonus' }) as HTMLInputElement
  expect(bonus().value).toBe('')
  expect(bonus().step).toBe('0.1')
  expect(authorsPanel().getByText('The author bonus is off.')).toBeTruthy()
  expect(authorsPanel().queryByRole('button', { name: 'Turn off bonus' })).toBeNull()
  expect(button(authorsPanel(), 'Save bonus').disabled).toBe(true)
  await userEvent.type(bonus(), '0')
  expect(button(authorsPanel(), 'Save bonus').disabled).toBe(true)
  await userEvent.clear(bonus())
  await userEvent.type(bonus(), '-1')
  expect(button(authorsPanel(), 'Save bonus').disabled).toBe(true)

  await enter('Threshold', '-0.2')
  await userEvent.clear(bonus())
  await userEvent.type(bonus(), '1.50')
  await userEvent.click(button(authorsPanel(), 'Save bonus'))
  await authorsPanel().findByText("Each favorite author's tag weighs at least +1.50.")
  expect(bonus().value).toBe('1.5')
  expect(field('Threshold').value).toBe('-0.2')

  await userEvent.click(button(authorsPanel(), 'Turn off bonus'))
  await authorsPanel().findByText('The author bonus is off.')
  expect(bonus().value).toBe('')
  expect(field('Threshold').value).toBe('-0.2')
  expect(changeCalls()).toEqual([
    ['/api/collector/catalog/score/author-bonus', 'PUT', '{"bonus":1.5}'],
    ['/api/collector/catalog/score/author-bonus', 'PUT', '{"bonus":null}'],
  ])
  expect(previewBodies()).toEqual(['{}', measure7, measure7])
})

test('a failed bonus change shows a message in the section and calculates nothing', async () => {
  respond(Response.json(preview()), Response.json({ error: 'invalid_author_bonus' }, { status: 400 }))
  render(<App />)
  await loaded()
  await userEvent.type(authorsPanel().getByRole('spinbutton', { name: 'Author bonus' }), '9999')
  await userEvent.click(button(authorsPanel(), 'Save bonus'))
  expect((await authorsPanel().findByRole('alert')).textContent).toBe('Could not save the author bonus. Enter a number above 0.')
  expect(authorsPanel().getByText('The author bonus is off.')).toBeTruthy()
  expect(previewBodies()).toHaveLength(1)
})

test('the pin controls of an author pin its tag before the calculation answers', async () => {
  const authorPin: ScorePin = { id: 11, kind: 'tag', label: 'artist:most', weight: 2.5 }
  respond(
    Response.json(preview({ favorite_author_count: 1, favorite_authors: [author(0, { label: 'artist:most', favorites: 31, weight: 1.5 })] })),
    Response.json(authorPin),
  )
  fetchMock.mockReturnValueOnce(new Promise(() => {}))
  render(<App />)
  await loaded()
  await userEvent.type(pinField('artist:most', authorsPanel()), '2.5')
  await userEvent.click(button(authorsPanel(), 'Pin'))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(listedPins()).toEqual(['artist:most 2.5'])
  expect(authorsPanel().getByRole('button', { name: 'Remove pin' })).toBeTruthy()
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/pins', 'POST', '{"kind":"tag","value":"artist:most","weight":2.5}']])
  expect(previewBodies()).toEqual(['{}', measure7])
})

test('a failed pin change from the author list shows its message in the section', async () => {
  respond(
    Response.json(preview({ favorite_author_count: 1, favorite_authors: [author(0)] })),
    Response.json({ error: 'collector_unavailable' }, { status: 502 }),
  )
  render(<App />)
  await loaded()
  await userEvent.click(button(authorsPanel(), 'Ignore'))
  expect((await authorsPanel().findByRole('alert')).textContent).toMatch(/^Could not change the pin\. The request could not be confirmed\./)
  expect(pinsSection().queryByRole('alert')).toBeNull()
  expect(newModel().queryByRole('alert')).toBeNull()
  expect(previewBodies()).toHaveLength(1)
})

function filterAuthors() {
  return authorsPanel().getByRole('textbox', { name: 'Filter favorite authors' }) as HTMLInputElement
}

test('the favorite authors filter searches every listed author by its tag', async () => {
  respond(Response.json(preview({ favorite_author_count: 300, favorite_authors: Array.from({ length: 120 }, (_, index) => author(index)) })))
  render(<App />)
  await loaded()
  expect(authorsPanel().getByText('120 of 300 favorite authors, most favorites first.')).toBeTruthy()
  expect(listedAuthors()).toHaveLength(120)
  expect(filterAuthors().placeholder).toBe('Filter by artist')
  await typeFilter(filterAuthors(), ' AUTHOR 11 ')
  expect(listedAuthors().map(([label]) => label)).toEqual(['artist:author 11', ...Array.from({ length: 10 }, (_, index) => `artist:author ${110 + index}`)])
  expect(authorsPanel().getByText('11 matching favorite authors, most favorites first. Only the 120 authors with the most favorites of 300 are searched.')).toBeTruthy()
  await typeFilter(filterAuthors(), 'zzz')
  expect(authorsPanel().queryByRole('list')).toBeNull()
  expect(authorsPanel().getByText('No favorite authors match. Only the 120 authors with the most favorites of 300 are searched.')).toBeTruthy()
})

test('the favorite authors filter says nothing about its scope when every author is listed, and stays after a pin change', async () => {
  const authors = Array.from({ length: 120 }, (_, index) => author(index))
  const body = preview({ favorite_author_count: 120, favorite_authors: authors })
  const pin: ScorePin = { id: 12, kind: 'tag', label: 'artist:author 70', weight: 1 }
  respond(Response.json(body), Response.json(pin), Response.json({ ...body, pins: [pin] }))
  render(<App />)
  await loaded()
  await typeFilter(filterAuthors(), 'artist')
  expect(authorsPanel().getByText('120 matching favorite authors, most favorites first.')).toBeTruthy()
  await userEvent.type(pinField('artist:author 70', authorsPanel()), '1')
  await userEvent.click(within(pinField('artist:author 70', authorsPanel()).closest('li')!).getByRole('button', { name: 'Pin' }))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  await waitFor(() => expect(listedPins()).toEqual(['artist:author 70 1']))
  expect(filterAuthors().value).toBe('artist')
  expect(listedAuthors()).toHaveLength(120)
  await typeFilter(filterAuthors(), 'zzz')
  expect(authorsPanel().getByText('No favorite authors match.')).toBeTruthy()
})

test('a preview without favorite authors shows no filter and no count line', async () => {
  respond(Response.json(preview()))
  render(<App />)
  await loaded()
  expect(authorsPanel().queryByRole('textbox', { name: 'Filter favorite authors' })).toBeNull()
  expect(authorsPanel().queryByText(/most favorites first/)).toBeNull()
})
