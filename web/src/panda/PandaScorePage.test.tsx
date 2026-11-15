// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
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

function weight(index: number, overrides: Partial<ScoreWeight> = {}): ScoreWeight {
  return { kind: 'tag', label: `tag:value ${index}`, weight: -1, interesting: 0, not_for_me: 3, neutral: 1, active_weight: null, ...overrides }
}

function preview(overrides: Partial<ScorePreview> = {}): ScorePreview {
  const histogram = { first_bucket: 0, counts: [] }
  const heldOut = { total: 0, always_hidden: 0, never_hidden: 0, scores: histogram }
  return {
    favorites: true,
    evidence: { interesting: 41, not_for_me: 122, neutral: 5003, counted_favorites: 7, favorites_synced_at: null, new_verdicts: 12, few_verdicts: false },
    candidate: {
      id: 7, calculated_at: '2026-11-01T10:00:00Z', favorites: true, weight_count: 3100, suggested_threshold_tenths: -8,
      weights: [
        weight(0, { label: 'female:glasses', weight: -1.42, interesting: 2, not_for_me: 31, neutral: 12, active_weight: -1.1 }),
        weight(1, { kind: 'uploader', label: 'Some uploader', weight: 0.5, interesting: 9, not_for_me: 0, neutral: 4 }),
        weight(2, { kind: 'category', label: 'Doujinshi', weight: 0.257, interesting: 5, not_for_me: 1, neutral: 8, active_weight: 0.3 }),
      ],
      held_out: { interesting: heldOut, not_for_me: heldOut, neutral: heldOut },
    },
    active: { calculated_at: '2026-10-01T10:00:00Z', favorites: true, threshold_tenths: -5 },
    overrides: [],
    pins: [],
    author_bonus: null,
    favorite_author_count: 0,
    favorite_authors: [],
    recent: { total: 0, interesting: 0, not_for_me: 0, never_hidden: 0, always_hidden: 0, candidate: histogram, active: histogram },
    ...overrides,
  }
}

function respond(...responses: Response[]) {
  for (const response of responses) fetchMock.mockResolvedValueOnce(response)
}

function previewBodies() {
  return fetchMock.mock.calls.filter(([input]) => input === '/api/collector/catalog/score/preview').map(([, init]) => init?.body)
}

function evidence() {
  return within(screen.getByRole('region', { name: 'Evidence' }))
}

function rows() {
  return within(newModel().getByRole('table')).getAllByRole('row').slice(1).map((row) => within(row).getAllByRole('cell').slice(0, 6).map((cell) => cell.textContent))
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
  respond(Response.json(preview()))
  render(<App />)
  expect(screen.getByRole('heading', { name: 'Score model' })).toBeTruthy()
  expect(screen.getByRole('link', { name: 'Panda' }).getAttribute('aria-current')).toBe('page')
  expect(screen.getByRole('link', { name: 'Panda catalog' }).getAttribute('href')).toBe('#/panda')
  expect(screen.getByText('Calculating…')).toBeTruthy()
  await screen.findByRole('region', { name: 'Evidence' })
  expect(previewBodies()).toEqual(['{"favorites":null}'])
  for (const [label, value] of [['Interesting verdicts', '41'], ['Not for me verdicts', '122'], ['Neutral galleries', '5,003'], ['Counted favorites', '7'], ['Last Panda favorite sync', 'never'], ['Verdicts since the active model was calculated', '12']]) {
    expect(evidence().getByText(label).nextElementSibling?.textContent).toBe(value)
  }
  expect(screen.getByRole('switch', { name: 'Count Panda favorites as Interesting' }).getAttribute('aria-checked')).toBe('true')
  expect(within(screen.getByRole('region', { name: 'Active model' })).getByText(/with favorites, threshold -0\.5/)).toBeTruthy()
  expect(screen.getByText('3 of 3,100 weights, strongest first.')).toBeTruthy()
  expect(rows()).toEqual([
    ['female:glasses', '-1.42', '2', '31', '12', '-1.10'],
    ['Uploader Some uploader', '+0.50', '9', '0', '4', '–'],
    ['Category Doujinshi', '+0.26', '5', '1', '8', '+0.30'],
  ])
  expect(screen.queryByText(/Few verdicts/)).toBeNull()
})

test('the favorites switch calculates again and replaces the result', async () => {
  respond(Response.json(preview()), Response.json(preview({
    favorites: false,
    evidence: { ...preview().evidence, interesting: 30, counted_favorites: 0, favorites_synced_at: '2026-11-01T10:00:00Z' },
    candidate: { ...preview().candidate!, favorites: false, weight_count: 1, weights: [weight(5, { label: 'other:tag', weight: 2 })] },
  })))
  render(<App />)
  await userEvent.click(await screen.findByRole('switch', { name: 'Count Panda favorites as Interesting' }))
  await screen.findByText('1 of 1 weights, strongest first.')
  expect(previewBodies()).toEqual(['{"favorites":null}', '{"favorites":false}'])
  expect(screen.getByRole('switch', { name: 'Count Panda favorites as Interesting' }).getAttribute('aria-checked')).toBe('false')
  expect(evidence().getByText('Interesting verdicts').nextElementSibling?.textContent).toBe('30')
  expect(evidence().getByText('Counted favorites').nextElementSibling?.textContent).toBe('0')
  expect(evidence().getByText('Last Panda favorite sync').nextElementSibling?.querySelector('time')?.dateTime).toBe('2026-11-01T10:00:00Z')
  expect(rows()).toEqual([['other:tag', '+2.00', '0', '3', '1', '–']])
})

test('shows 50 weights at first and 50 more per click', async () => {
  const weights = Array.from({ length: 120 }, (_, index) => weight(index))
  respond(Response.json(preview({ candidate: { ...preview().candidate!, weight_count: 3100, weights } })))
  render(<App />)
  await screen.findByText('50 of 3,100 weights, strongest first.')
  expect(rows()).toHaveLength(50)
  expect(rows()[49][0]).toBe('tag:value 49')
  await userEvent.click(screen.getByRole('button', { name: 'Show more' }))
  expect(rows()).toHaveLength(100)
  expect(screen.getByText('100 of 3,100 weights, strongest first.')).toBeTruthy()
  await userEvent.click(screen.getByRole('button', { name: 'Show more' }))
  expect(rows()).toHaveLength(120)
  expect(rows()[119][0]).toBe('tag:value 119')
  expect(screen.queryByRole('button', { name: 'Show more' })).toBeNull()
})

test('the shown weights stay shown after an override calculates again', async () => {
  const weights = Array.from({ length: 120 }, (_, index) => weight(index))
  const body = preview({ candidate: { ...preview().candidate!, weights } })
  respond(Response.json(body), new Response(null, { status: 204 }), Response.json({ ...body, candidate: { ...body.candidate!, id: 8 }, overrides: [{ id: 1, kind: 'tag', label: 'tag:value 60', rule: 'always_hide' }] }))
  render(<App />)
  await screen.findByText('50 of 3,100 weights, strongest first.')
  await userEvent.click(screen.getByRole('button', { name: 'Show more' }))
  await userEvent.click(within(within(newModel().getByRole('table')).getByText('tag:value 60').closest('tr')!).getByRole('button', { name: 'Always hide' }))
  await screen.findByText('Always hide tag:value 60')
  expect(previewBodies()).toHaveLength(2)
  expect(rows()).toHaveLength(100)
})

function filterWeights() {
  return screen.getByRole('textbox', { name: 'Filter weights' }) as HTMLInputElement
}

async function typeFilter(input: HTMLInputElement, text: string) {
  await userEvent.clear(input)
  await userEvent.type(input, text)
}

test('the weights filter searches every weight of the preview by its label, ignoring case and spaces around the text', async () => {
  const weights = Array.from({ length: 120 }, (_, index) => weight(index))
  respond(Response.json(preview({ candidate: { ...preview().candidate!, weights } })), Response.json(preview({ candidate: { ...preview().candidate!, weight_count: 3 } })))
  render(<App />)
  await screen.findByText('50 of 3,100 weights, strongest first.')
  expect(filterWeights().placeholder).toBe('Filter by tag, uploader or category')
  await typeFilter(filterWeights(), ' VALUE 11 ')
  expect(rows().map(([label]) => label)).toEqual(['tag:value 11', ...Array.from({ length: 10 }, (_, index) => `tag:value ${110 + index}`)])
  expect(screen.getByText('11 of 11 matching weights, strongest first. Only the 120 strongest of 3,100 weights are searched.')).toBeTruthy()
  await typeFilter(filterWeights(), 'zzz')
  expect(newModel().queryByRole('table')).toBeNull()
  expect(screen.getByText('No weights match. Only the 120 strongest of 3,100 weights are searched.')).toBeTruthy()
  await typeFilter(filterWeights(), '   ')
  expect(screen.getByText('50 of 3,100 weights, strongest first.')).toBeTruthy()

  await userEvent.click(screen.getByRole('switch', { name: 'Count Panda favorites as Interesting' }))
  await screen.findByText('3 of 3 weights, strongest first.')
  await typeFilter(filterWeights(), 'uploader')
  expect(rows().map(([label]) => label)).toEqual(['Uploader Some uploader'])
  expect(screen.getByText('1 of 1 matching weights, strongest first.')).toBeTruthy()
  await typeFilter(filterWeights(), 'zzz')
  expect(screen.getByText('No weights match.')).toBeTruthy()
})

test('the weights filter pages through more than 50 matches and starts again at 50 for a new text', async () => {
  const weights = [...Array.from({ length: 120 }, (_, index) => weight(index)), weight(120, { kind: 'uploader', label: 'Some uploader' })]
  respond(Response.json(preview({ candidate: { ...preview().candidate!, weight_count: 121, weights } })))
  render(<App />)
  await screen.findByText('50 of 121 weights, strongest first.')
  await typeFilter(filterWeights(), 'tag:')
  expect(screen.getByText('50 of 120 matching weights, strongest first.')).toBeTruthy()
  await userEvent.click(screen.getByRole('button', { name: 'Show more' }))
  expect(rows()).toHaveLength(100)
  await userEvent.click(screen.getByRole('button', { name: 'Show more' }))
  expect(rows()).toHaveLength(120)
  expect(screen.queryByRole('button', { name: 'Show more' })).toBeNull()
  await userEvent.type(filterWeights(), 'v')
  expect(rows()).toHaveLength(50)
  expect(screen.getByText('50 of 120 matching weights, strongest first.')).toBeTruthy()
})

test('the weights filter and the shown matches stay after a pin change or an override change calculates again', async () => {
  const weights = [...Array.from({ length: 120 }, (_, index) => weight(index)), weight(120, { kind: 'uploader', label: 'Some uploader' })]
  const body = preview({ candidate: { ...preview().candidate!, weights } })
  const pin: ScorePin = { id: 9, kind: 'tag', label: 'tag:value 70', weight: 1 }
  respond(
    Response.json(body),
    Response.json(pin), Response.json({ ...body, pins: [pin] }),
    new Response(null, { status: 204 }), Response.json({ ...body, pins: [pin], overrides: [{ id: 1, kind: 'tag', label: 'tag:value 80', rule: 'always_hide' }] }),
  )
  render(<App />)
  await screen.findByText('50 of 3,100 weights, strongest first.')
  await typeFilter(filterWeights(), 'tag')
  await userEvent.click(screen.getByRole('button', { name: 'Show more' }))
  await userEvent.type(pinField('tag:value 70'), '1')
  await userEvent.click(button(weightRow('tag:value 70'), 'Pin'))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  await waitFor(() => expect(listedPins()).toEqual(['tag:value 70 1']))
  expect(filterWeights().value).toBe('tag')
  expect(rows()).toHaveLength(100)
  await userEvent.click(button(weightRow('tag:value 80'), 'Always hide'))
  await screen.findByText('Always hide tag:value 80')
  expect(previewBodies()).toHaveLength(3)
  expect(filterWeights().value).toBe('tag')
  expect(rows()).toHaveLength(100)
  expect(screen.getByText('100 of 120 matching weights, strongest first. Only the 121 strongest of 3,100 weights are searched.')).toBeTruthy()
})

test('warns about few verdicts and still shows the table', async () => {
  respond(Response.json(preview({ evidence: { ...preview().evidence, few_verdicts: true } })))
  render(<App />)
  expect((await screen.findByText(/Few verdicts\./)).textContent).toBe('Few verdicts. Weights and error rates from so little evidence are unreliable.')
  expect(newModel().getByRole('table')).toBeTruthy()
})

test.each([
  ['no_verdicts', 'No verdicts yet.'],
  ['no_not_for_me', 'No "Not for me" verdicts yet.'],
  ['no_acceptable', 'No acceptable galleries yet.'],
] as const)('without a candidate for %s explains why and keeps the active model', async (noModel, message) => {
  respond(Response.json(preview({ candidate: null, no_model: noModel, evidence: { ...preview().evidence, few_verdicts: true } })))
  render(<App />)
  await screen.findByRole('region', { name: 'Evidence' })
  const section = within(screen.getByRole('region', { name: 'New model' }))
  const messages = ['No verdicts yet.', 'No "Not for me" verdicts yet.', 'No acceptable galleries yet.']
  for (const other of messages) expect(section.queryAllByText((text) => text.startsWith(other))).toHaveLength(other === message ? 1 : 0)
  expect(newModel().queryByRole('table')).toBeNull()
  expect(screen.queryByText(/Few verdicts/)).toBeNull()
  expect(within(screen.getByRole('region', { name: 'Active model' })).getByText(/threshold -0\.5/)).toBeTruthy()
})

test('without an active model says so and leaves out the verdicts since it', async () => {
  respond(Response.json(preview({ active: null, evidence: { ...preview().evidence, new_verdicts: null } })))
  render(<App />)
  await screen.findByRole('region', { name: 'Evidence' })
  expect(evidence().queryByText('Verdicts since the active model was calculated')).toBeNull()
  expect(within(screen.getByRole('region', { name: 'Active model' })).getByText('No model is active.')).toBeTruthy()
})

test('reports an unavailable collector and calculates again on retry', async () => {
  respond(Response.json({ error: 'collector_unavailable' }, { status: 502 }), Response.json(preview()))
  render(<App />)
  const alert = await screen.findByRole('alert')
  expect(alert.textContent).toContain('The request could not be confirmed.')
  expect(screen.queryByRole('region', { name: 'Evidence' })).toBeNull()
  await userEvent.click(within(alert).getByRole('button', { name: 'Retry' }))
  await screen.findByRole('region', { name: 'Evidence' })
  expect(previewBodies()).toEqual(['{"favorites":null}', '{"favorites":null}'])
  expect(screen.queryByRole('alert')).toBeNull()
})

function histogram(firstBucket: number, counts: Record<number, number>) {
  const last = Math.max(...Object.keys(counts).map(Number))
  return { first_bucket: firstBucket, counts: Array.from({ length: last - firstBucket + 1 }, (_, index) => counts[firstBucket + index] ?? 0) }
}

// At -0.8: Not for me 40 %, Interesting 5 %, neutral 4 %, recent 10 %. At -0.5: 60, 10, 8, 20 %. At -0.2: 80, 20, 18, 40 %.
// The active histogram hides 20 % at -0.8, 35 % at -0.5 and 50 % at -0.2.
function measured(overrides: Partial<ScorePreview> = {}): ScorePreview {
  return preview({
    candidate: {
      ...preview().candidate!,
      held_out: {
        not_for_me: { total: 10, always_hidden: 1, never_hidden: 0, scores: histogram(-9, { [-9]: 3, [-6]: 2, [-3]: 2, 0: 2 }) },
        interesting: { total: 20, always_hidden: 0, never_hidden: 0, scores: histogram(-9, { [-9]: 1, [-6]: 1, [-3]: 2, 0: 16 }) },
        neutral: { total: 100, always_hidden: 1, never_hidden: 0, scores: histogram(-9, { [-9]: 3, [-6]: 4, [-3]: 10, 0: 82 }) },
      },
    },
    recent: {
      total: 200, interesting: 2, not_for_me: 4, never_hidden: 1, always_hidden: 6,
      candidate: histogram(-9, { [-9]: 10, [-6]: 20, [-3]: 40, 0: 120 }),
      active: histogram(-9, { [-9]: 30, [-6]: 30, [-3]: 30, 0: 100 }),
    },
    ...overrides,
  })
}

// A second candidate that hides 50 % of Not for me, 10 % of Interesting, 20 % of neutral and 30 % of the recent sample at -0.2.
function switched(): ScorePreview {
  const base = measured()
  return {
    ...base,
    favorites: false,
    candidate: {
      ...base.candidate!, id: 9, favorites: false, suggested_threshold_tenths: -3,
      held_out: {
        not_for_me: { total: 10, always_hidden: 0, never_hidden: 0, scores: histogram(-4, { [-4]: 5, 0: 5 }) },
        interesting: { total: 10, always_hidden: 0, never_hidden: 0, scores: histogram(-4, { [-4]: 1, 0: 9 }) },
        neutral: { total: 10, always_hidden: 0, never_hidden: 0, scores: histogram(-4, { [-4]: 2, 0: 8 }) },
      },
    },
    recent: { ...base.recent, candidate: histogram(-4, { [-4]: 50, 0: 150 }) },
  }
}

const noContent = () => new Response(null, { status: 204 })

function newModel() {
  return within(screen.getByRole('region', { name: 'New model' }))
}

function activeModel() {
  return within(screen.getByRole('region', { name: 'Active model' }))
}

function field(name: string) {
  return screen.getByRole('spinbutton', { name }) as HTMLInputElement
}

async function enter(name: string, value: string) {
  await userEvent.clear(field(name))
  await userEvent.type(field(name), value)
}

function changeCalls() {
  return fetchMock.mock.calls.filter(([input]) => input !== '/api/collector/catalog/score/preview').map(([input, init]) => [input, init?.method, init?.body])
}

function expectEffect(notForMe: number, interesting: number, neutral: number, recent: number, now: number) {
  const section = newModel()
  expect(section.getByText(/^Hides \d+ % of Not for me/).textContent).toBe(`Hides ${notForMe} % of Not for me, ${interesting} % of Interesting and ${neutral} % of neutral evidence galleries.`)
  expect(section.getByText(/catalog galleries \(now/).textContent).toBe(`Hides ${recent} % of the newest 200 catalog galleries (now: ${now} %)`)
  expect(section.getByText(/^Interesting hidden:/).textContent).toBe(`Interesting hidden: ${interesting} %`)
  expect(section.getByText(/^Not for me shown:/).textContent).toBe(`Not for me shown: ${100 - notForMe} %`)
}

test('with an active model the threshold starts at the active threshold and follows the field without a request', async () => {
  respond(Response.json(measured()))
  render(<App />)
  await screen.findByRole('region', { name: 'Evidence' })
  expect(field('Threshold').value).toBe('-0.5')
  expect(newModel().getByText('Suggested: -0.8')).toBeTruthy()
  expect(newModel().getByText('Error rates, measured on galleries the model did not learn from')).toBeTruthy()
  expectEffect(60, 10, 8, 20, 35)
  await enter('Threshold', '-0.2')
  expectEffect(80, 20, 18, 40, 35)
  expect(field('Active threshold').value).toBe('-0.5')
  expect(activeModel().getByText(/^Hides/).textContent).toBe('Hides 35 % of the newest 200 catalog galleries')
  await enter('Active threshold', '-0.8')
  expect(activeModel().getByText(/^Hides/).textContent).toBe('Hides 20 % of the newest 200 catalog galleries')
  expect(fetchMock).toHaveBeenCalledTimes(1)
})

test('without an active model or overrides nothing is hidden now and the field starts at the suggested threshold', async () => {
  respond(Response.json(measured({ active: null, recent: { ...measured().recent, active: null } })))
  render(<App />)
  await screen.findByRole('region', { name: 'Evidence' })
  expect(field('Threshold').value).toBe('-0.8')
  expect(newModel().getByText('Suggested: -0.8')).toBeTruthy()
  expectEffect(40, 5, 4, 10, 0)
  expect(screen.queryByRole('spinbutton', { name: 'Active threshold' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Save threshold' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Turn off' })).toBeNull()
})

test('the entered threshold stays across the favorites switch and applies the candidate on screen', async () => {
  respond(Response.json(measured()), Response.json(switched()), noContent(), Response.json(measured({ active: { ...measured().active!, threshold_tenths: -2 } })))
  render(<App />)
  await screen.findByRole('region', { name: 'Evidence' })
  await enter('Threshold', '-0.2')
  await userEvent.click(screen.getByRole('switch', { name: 'Count Panda favorites as Interesting' }))
  await newModel().findByText('Suggested: -0.3')
  expect(field('Threshold').value).toBe('-0.2')
  expectEffect(50, 10, 20, 30, 35)
  await userEvent.click(newModel().getByRole('button', { name: 'Apply model and threshold' }))
  expect(await newModel().findByText('Model applied.')).toBeTruthy()
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/apply', 'POST', '{"candidate_id":9,"threshold_tenths":-2}']])
  await waitFor(() => expect(previewBodies()).toEqual(['{"favorites":null}', '{"favorites":false}', '{"favorites":false}']))
})

test('after a successful change both fields start again from the next preview', async () => {
  respond(
    Response.json(measured()), noContent(), Response.json(measured({ active: { ...measured().active!, threshold_tenths: -3 } })),
    noContent(), Response.json(measured({ active: null })),
  )
  render(<App />)
  await screen.findByRole('region', { name: 'Evidence' })
  await enter('Threshold', '-0.2')
  await enter('Active threshold', '-0.3')
  await userEvent.click(screen.getByRole('button', { name: 'Save threshold' }))
  await activeModel().findByText('Threshold saved.')
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/threshold', 'PUT', '{"threshold_tenths":-3}']])
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  await waitFor(() => expect(field('Threshold').value).toBe('-0.3'))
  expect(field('Active threshold').value).toBe('-0.3')

  await userEvent.click(screen.getByRole('button', { name: 'Turn off' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog.textContent).toContain("Turn off the score model?The model is deleted and can't be restored. Overrides keep working.")
  expect(changeCalls()).toHaveLength(1)
  await userEvent.click(within(dialog).getByRole('button', { name: 'Turn off' }))
  await activeModel().findByText('No model is active.')
  expect(changeCalls()[1]).toEqual(['/api/collector/catalog/score/model', 'DELETE', undefined])
  expect(previewBodies()).toHaveLength(3)
  expect(field('Threshold').value).toBe('-0.8')
  expect(screen.queryByRole('spinbutton', { name: 'Active threshold' })).toBeNull()
})

test('a failed apply keeps the candidate and the field, and calculates again only for a replaced candidate', async () => {
  respond(
    Response.json(measured()),
    Response.json({ error: 'collector_unavailable' }, { status: 502 }),
    Response.json({ error: 'score_candidate_not_found' }, { status: 409 }),
    Response.json(switched()),
  )
  render(<App />)
  await screen.findByRole('region', { name: 'Evidence' })
  await enter('Threshold', '-0.2')
  await userEvent.click(screen.getByRole('button', { name: 'Apply model and threshold' }))
  const alert = await newModel().findByRole('alert')
  expect(alert.textContent).toMatch(/^Could not apply the model\. The request could not be confirmed\./)
  expect(previewBodies()).toHaveLength(1)
  expect(field('Threshold').value).toBe('-0.2')
  expect(newModel().getByRole('table')).toBeTruthy()
  await userEvent.click(screen.getByRole('button', { name: 'Apply model and threshold' }))
  await newModel().findByText('Could not apply the model. This calculation is no longer available. Check the new calculation and apply again.')
  await newModel().findByText('Suggested: -0.3')
  expect(previewBodies()).toEqual(['{"favorites":null}', '{"favorites":true}'])
  expect(field('Threshold').value).toBe('-0.2')
  expect(newModel().getByText(/Check the new calculation/)).toBeTruthy()
})

test('an empty threshold disables the apply button, few verdicts do not', async () => {
  respond(Response.json(measured({ evidence: { ...measured().evidence, few_verdicts: true } })))
  render(<App />)
  await screen.findByRole('region', { name: 'Evidence' })
  const apply = newModel().getByRole('button', { name: 'Apply model and threshold' }) as HTMLButtonElement
  expect(apply.disabled).toBe(false)
  await userEvent.clear(field('Threshold'))
  expect(apply.disabled).toBe(true)
})

test('without a candidate there is no threshold for a new model', async () => {
  respond(Response.json(measured({ candidate: null, no_model: 'no_verdicts' })))
  render(<App />)
  await screen.findByRole('region', { name: 'Evidence' })
  expect(screen.queryByRole('spinbutton', { name: 'Threshold' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Apply model and threshold' })).toBeNull()
  expect(field('Active threshold').value).toBe('-0.5')
})

test('failures to save the threshold or turn off the model show a message and request nothing', async () => {
  respond(
    Response.json(measured()),
    Response.json({ error: 'invalid_threshold' }, { status: 400 }),
    Response.json({ error: 'score_model_not_active' }, { status: 409 }),
  )
  render(<App />)
  await screen.findByRole('region', { name: 'Evidence' })
  await userEvent.click(screen.getByRole('button', { name: 'Save threshold' }))
  await activeModel().findByText('Could not save the threshold. Enter a valid threshold.')
  await userEvent.click(screen.getByRole('button', { name: 'Turn off' }))
  await userEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Cancel' }))
  expect(screen.queryByRole('alertdialog')).toBeNull()
  expect(changeCalls()).toHaveLength(1)
  await userEvent.click(screen.getByRole('button', { name: 'Turn off' }))
  await userEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Turn off' }))
  await activeModel().findByText('Could not turn off the model. No score model is active any more.')
  expect(previewBodies()).toHaveLength(1)
})

test('a calculation in progress and a change in progress lock each other out, so the favorites setting is never lost', async () => {
  let finishPreview!: (response: Response) => void
  let finishChange!: (response: Response) => void
  respond(Response.json(measured()))
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishPreview = resolve }))
  render(<App />)
  await screen.findByRole('region', { name: 'Evidence' })
  const favorites = () => screen.getByRole('switch', { name: 'Count Panda favorites as Interesting' }) as HTMLButtonElement
  const save = () => screen.getByRole('button', { name: 'Save threshold' }) as HTMLButtonElement
  const turnOff = () => screen.getByRole('button', { name: 'Turn off' }) as HTMLButtonElement

  await userEvent.click(favorites())
  expect(save().disabled).toBe(true)
  expect(turnOff().disabled).toBe(true)
  finishPreview(Response.json(switched()))
  await newModel().findByText('Suggested: -0.3')
  expect(save().disabled).toBe(false)

  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishChange = resolve }))
  respond(Response.json(switched()))
  await userEvent.click(save())
  expect(favorites().disabled).toBe(true)
  finishChange(noContent())
  await activeModel().findByText('Threshold saved.')
  await waitFor(() => expect(previewBodies()).toEqual(['{"favorites":null}', '{"favorites":false}', '{"favorites":false}']))
})

test('Retry waits for a change in progress, so a retried favorites setting is not replaced', async () => {
  let finishChange!: (response: Response) => void
  respond(Response.json(measured()), Response.json({ error: 'collector_unavailable' }, { status: 502 }))
  render(<App />)
  await userEvent.click(await screen.findByRole('switch', { name: 'Count Panda favorites as Interesting' }))
  const retry = within(await screen.findByRole('alert')).getByRole('button', { name: 'Retry' }) as HTMLButtonElement
  expect(retry.disabled).toBe(false)
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishChange = resolve }))
  await userEvent.click(screen.getByRole('button', { name: 'Apply model and threshold' }))
  expect(retry.disabled).toBe(true)
  respond(Response.json(measured()))
  finishChange(noContent())
  await newModel().findByText('Model applied.')
  await waitFor(() => expect(previewBodies()).toEqual(['{"favorites":null}', '{"favorites":false}', '{"favorites":true}']))
})

function overridesSection() {
  return within(screen.getByRole('region', { name: 'Overrides' }))
}

function listedOverrides() {
  const list = overridesSection().queryByRole('list')
  return list ? within(list).getAllByRole('listitem').map((item) => item.querySelector('[data-override-label]')?.textContent) : []
}

function weightRow(label: string) {
  return within(within(newModel().getByRole('table')).getByText(label).closest('tr')!)
}

function pressed(label: string, rule: 'Always hide' | 'Never hide') {
  return weightRow(label).getByRole('button', { name: rule }).getAttribute('aria-pressed')
}

const glassesHidden = { id: 4, kind: 'tag', label: 'female:glasses', rule: 'always_hide' } as const

test('lists the overrides also without a candidate or an active model', async () => {
  respond(Response.json(preview({
    candidate: null, no_model: 'no_verdicts', active: null,
    overrides: [glassesHidden, { id: 5, kind: 'uploader', label: 'Some uploader', rule: 'never_hide' }, { id: 6, kind: 'category', label: 'Cosplay', rule: 'always_hide' }],
  })))
  render(<App />)
  await screen.findByRole('region', { name: 'Overrides' })
  expect(listedOverrides()).toEqual(['Always hide female:glasses', 'Never hide Uploader Some uploader', 'Always hide Category Cosplay'])
  expect(overridesSection().getAllByRole('button', { name: 'Remove' })).toHaveLength(3)
})

test('an empty list of overrides says so', async () => {
  respond(Response.json(preview()))
  render(<App />)
  await screen.findByRole('region', { name: 'Overrides' })
  expect(overridesSection().getByText('No overrides.')).toBeTruthy()
  expect(listedOverrides()).toEqual([])
})

test('a row button saves an override, calculates again and shows it pressed and listed', async () => {
  respond(Response.json(preview()), noContent(), Response.json(preview({ overrides: [glassesHidden] })))
  render(<App />)
  await screen.findByRole('region', { name: 'Overrides' })
  expect(pressed('female:glasses', 'Always hide')).toBe('false')
  await userEvent.click(weightRow('female:glasses').getByRole('button', { name: 'Always hide' }))
  await overridesSection().findByText('Always hide female:glasses')
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/overrides', 'POST', '{"kind":"tag","value":"female:glasses","rule":"always_hide"}']])
  expect(previewBodies()).toEqual(['{"favorites":null}', '{"favorites":true}'])
  expect(pressed('female:glasses', 'Always hide')).toBe('true')
  expect(pressed('female:glasses', 'Never hide')).toBe('false')
})

test('an override change keeps the entered threshold and follows the new calculation', async () => {
  respond(Response.json(measured()), noContent(), Response.json({ ...switched(), favorites: true, overrides: [glassesHidden] }))
  render(<App />)
  await screen.findByRole('region', { name: 'Evidence' })
  await enter('Threshold', '-0.2')
  await userEvent.click(weightRow('female:glasses').getByRole('button', { name: 'Always hide' }))
  await newModel().findByText('Suggested: -0.3')
  expect(field('Threshold').value).toBe('-0.2')
  expect(field('Active threshold').value).toBe('-0.5')
  expectEffect(50, 10, 20, 30, 35)
  expect(previewBodies()).toEqual(['{"favorites":null}', '{"favorites":true}'])
})

test('a click on a pressed row button removes the override', async () => {
  respond(Response.json(preview({ overrides: [glassesHidden] })), noContent(), Response.json(preview()))
  render(<App />)
  await screen.findByRole('region', { name: 'Overrides' })
  await userEvent.click(weightRow('female:glasses').getByRole('button', { name: 'Always hide' }))
  await overridesSection().findByText('No overrides.')
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/overrides/4', 'DELETE', undefined]])
  expect(previewBodies()).toHaveLength(2)
  expect(pressed('female:glasses', 'Always hide')).toBe('false')
})

test('the other rule on a row replaces the override without removing it', async () => {
  respond(Response.json(preview({ overrides: [glassesHidden] })), noContent(), Response.json(preview({ overrides: [{ ...glassesHidden, rule: 'never_hide' }] })))
  render(<App />)
  await screen.findByRole('region', { name: 'Overrides' })
  await userEvent.click(weightRow('female:glasses').getByRole('button', { name: 'Never hide' }))
  await overridesSection().findByText('Never hide female:glasses')
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/overrides', 'POST', '{"kind":"tag","value":"female:glasses","rule":"never_hide"}']])
  expect(pressed('female:glasses', 'Never hide')).toBe('true')
  expect(pressed('female:glasses', 'Always hide')).toBe('false')
})

test('Remove in the list removes that override', async () => {
  const uploader = { id: 5, kind: 'uploader', label: 'Some uploader', rule: 'never_hide' } as const
  respond(Response.json(preview({ overrides: [glassesHidden, uploader] })), noContent(), Response.json(preview({ overrides: [glassesHidden] })))
  render(<App />)
  await screen.findByRole('region', { name: 'Overrides' })
  const item = within(overridesSection().getByText('Never hide Uploader Some uploader').closest('li')!)
  await userEvent.click(item.getByRole('button', { name: 'Remove' }))
  await waitFor(() => expect(listedOverrides()).toEqual(['Always hide female:glasses']))
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/overrides/5', 'DELETE', undefined]])
  expect(previewBodies()).toHaveLength(2)
})

function form() {
  return within(screen.getByRole('form', { name: 'Add override' }))
}

test('the form adds an override for a tag, an uploader or a category', async () => {
  respond(Response.json(preview()), noContent(), Response.json(preview()), noContent(), Response.json(preview()), noContent(), Response.json(preview()))
  render(<App />)
  await screen.findByRole('region', { name: 'Overrides' })
  const add = () => form().getByRole('button', { name: 'Add override' }) as HTMLButtonElement
  const tag = () => form().getByRole('combobox', { name: 'Tag (namespace:value)' }) as HTMLInputElement
  const uploader = () => form().getByRole('textbox', { name: 'Uploader (exactly as Panda writes it)' }) as HTMLInputElement
  expect(add().disabled).toBe(true)
  await userEvent.type(tag(), '   ')
  expect(add().disabled).toBe(true)
  await userEvent.type(tag(), 'other:tag  ')
  await userEvent.click(add())
  await waitFor(() => expect(tag().value).toBe(''))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))

  await userEvent.click(form().getByRole('radio', { name: 'Uploader' }))
  await userEvent.click(form().getByRole('radio', { name: 'Never hide' }))
  await userEvent.type(uploader(), ' Someone ')
  await userEvent.click(add())
  await waitFor(() => expect(uploader().value).toBe(''))

  await userEvent.click(form().getByRole('radio', { name: 'Category' }))
  expect(form().queryByRole('textbox')).toBeNull()
  await userEvent.selectOptions(form().getByRole('combobox', { name: 'Category' }), 'Cosplay')
  await userEvent.click(add())
  await waitFor(() => expect(previewBodies()).toHaveLength(4))
  expect(changeCalls()).toEqual([
    ['/api/collector/catalog/score/overrides', 'POST', '{"kind":"tag","value":"other:tag","rule":"always_hide"}'],
    ['/api/collector/catalog/score/overrides', 'POST', '{"kind":"uploader","value":"Someone","rule":"never_hide"}'],
    ['/api/collector/catalog/score/overrides', 'POST', '{"kind":"category","value":"Cosplay","rule":"never_hide"}'],
  ])
})

test('a failed override change shows a message, keeps the state and calculates nothing', async () => {
  respond(
    Response.json(preview({ overrides: [glassesHidden] })),
    Response.json({ error: 'tag_not_found' }, { status: 404 }),
    Response.json({ error: 'collector_unavailable' }, { status: 502 }),
  )
  render(<App />)
  await screen.findByRole('region', { name: 'Overrides' })
  await userEvent.type(form().getByRole('combobox', { name: 'Tag (namespace:value)' }), 'no:such tag')
  await userEvent.click(form().getByRole('button', { name: 'Add override' }))
  expect((await form().findByRole('alert')).textContent).toBe('Could not change the override. No gallery in the Panda catalog has this tag.')
  expect((form().getByRole('combobox', { name: 'Tag (namespace:value)' }) as HTMLInputElement).value).toBe('no:such tag')
  expect(listedOverrides()).toEqual(['Always hide female:glasses'])
  expect(previewBodies()).toHaveLength(1)

  await userEvent.click(weightRow('Uploader Some uploader').getByRole('button', { name: 'Never hide' }))
  expect((await newModel().findByRole('alert')).textContent).toMatch(/^Could not change the override\. The request could not be confirmed\./)
  expect(form().queryByRole('alert')).toBeNull()
  expect(pressed('Uploader Some uploader', 'Never hide')).toBe('false')
  expect(listedOverrides()).toEqual(['Always hide female:glasses'])
  expect(previewBodies()).toHaveLength(1)
})

function pinsSection() {
  return within(screen.getByRole('region', { name: 'Pinned weights' }))
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

function pinField(label: string) {
  return screen.getByRole('spinbutton', { name: `Pinned weight of ${label}` }) as HTMLInputElement
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
  await screen.findByRole('region', { name: 'Pinned weights' })
  expect(rows()[0]).toEqual(['female:glasses', '-1.42', '2', '31', '12', '-1.10'])
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
  respond(Response.json(preview({ candidate: null, no_model: 'no_verdicts', active: null })))
  render(<App />)
  await screen.findByRole('region', { name: 'Pinned weights' })
  expect(pinsSection().getByText('No score model is active: pinned weights have no effect.')).toBeTruthy()
  expect(pinsSection().getByText('No pinned weights.')).toBeTruthy()
  expect(listedPins()).toEqual([])
})

test('Pin, Ignore and Remove pin change the pin at once, keep the threshold and calculate again', async () => {
  respond(
    Response.json(measured({ favorites: false })),
    Response.json(uploaderPin), Response.json(measured({ favorites: false, pins: [uploaderPin] })),
    Response.json({ ...glassesPin, weight: 0 }), Response.json(measured({ favorites: false, pins: [uploaderPin, { ...glassesPin, weight: 0 }] })),
  )
  let finishPreview!: (response: Response) => void
  render(<App />)
  await screen.findByRole('region', { name: 'Pinned weights' })
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
  finishPreview(Response.json(measured({ favorites: false, pins: [{ ...glassesPin, weight: 0 }] })))
  await waitFor(() => expect(previewBodies()).toHaveLength(4))

  expect(changeCalls()).toEqual([
    ['/api/collector/catalog/score/pins', 'POST', '{"kind":"uploader","value":"Some uploader","weight":-1.5}'],
    ['/api/collector/catalog/score/pins', 'POST', '{"kind":"tag","value":"female:glasses","weight":0}'],
    ['/api/collector/catalog/score/pins/4', 'DELETE', undefined],
  ])
  expect(previewBodies()).toEqual(['{"favorites":null}', '{"favorites":null}', '{"favorites":null}', '{"favorites":null}'])
  await waitFor(() => expect(button(newModel(), 'Apply model and threshold').disabled).toBe(false))
  expect(field('Threshold').value).toBe('-0.2')
})

test('a saved pin replaces the pin of its value or is added before the calculation answers', async () => {
  let finishFirst!: (response: Response) => void
  let finishSecond!: (response: Response) => void
  respond(Response.json(preview({ pins: [glassesPin] })), Response.json({ ...glassesPin, weight: 3 }))
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishFirst = resolve }))
  render(<App />)
  await screen.findByRole('region', { name: 'Pinned weights' })
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
  await screen.findByRole('region', { name: 'Pinned weights' })
  for (const label of ['female:glasses', 'Uploader Some uploader', 'Category Doujinshi']) {
    expect(within(newModel().getByRole('table')).getByText(label).getAttribute('title')).toBe(label)
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
  await screen.findByRole('region', { name: 'Pinned weights' })
  await userEvent.type(pinField('female:glasses'), '{Enter}')
  await userEvent.type(pinField('Uploader Some uploader'), '-1.5{Enter}')
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/pins', 'POST', '{"kind":"uploader","value":"Some uploader","weight":-1.5}']])
})

test('Remove in the list removes that pin', async () => {
  respond(Response.json(preview({ pins: [glassesPin, uploaderPin] })), noContent(), Response.json(preview({ pins: [glassesPin] })))
  render(<App />)
  await screen.findByRole('region', { name: 'Pinned weights' })
  await userEvent.click(within(pinsSection().getByText('Uploader Some uploader').closest('tr')!).getByRole('button', { name: 'Remove' }))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/pins/4', 'DELETE', undefined]])
  expect(listedPins()).toEqual(['female:glasses 2'])
})

test('each row of the list edits its pin and shows the weight of its value in the new model', async () => {
  const cosplayPin: ScorePin = { id: 5, kind: 'category', label: 'Cosplay', weight: 0 }
  respond(Response.json(preview({ pins: [{ ...glassesPin, model_weight: -0.5 }, { ...uploaderPin, model_weight: 0.5 }, cosplayPin] })))
  render(<App />)
  await screen.findByRole('region', { name: 'Pinned weights' })
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
  await screen.findByRole('region', { name: 'Pinned weights' })
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
  await screen.findByRole('region', { name: 'Pinned weights' })
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
  await screen.findByRole('region', { name: 'Pinned weights' })
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
  respond(
    Response.json(preview({ pins: [glassesPin, uploaderPin] })),
    Response.json(preview({ favorites: false, pins: [{ ...glassesPin }, { ...uploaderPin }] })),
    Response.json(preview({ pins: [{ ...glassesPin, weight: 4 }, { ...uploaderPin, weight: 5 }] })),
  )
  render(<App />)
  await screen.findByRole('region', { name: 'Pinned weights' })
  await userEvent.clear(listField('female:glasses'))
  await userEvent.type(listField('female:glasses'), '7')
  await userEvent.clear(pinField('Uploader Some uploader'))
  await userEvent.type(pinField('Uploader Some uploader'), '9')
  const favorites = () => screen.getByRole('switch', { name: 'Count Panda favorites as Interesting' })
  await userEvent.click(favorites())
  await waitFor(() => expect(favorites().getAttribute('aria-checked')).toBe('false'))
  expect(listField('female:glasses').value).toBe('7')
  expect(pinField('Uploader Some uploader').value).toBe('9')
  await waitFor(() => expect(favorites().hasAttribute('disabled')).toBe(false))
  await userEvent.click(favorites())
  await waitFor(() => expect(listField('female:glasses').value).toBe('4'))
  expect(pinField('Uploader Some uploader').value).toBe('5')
})

test('a failed save in the list shows a message in the section, keeps the text and calculates nothing', async () => {
  respond(Response.json(preview({ pins: [glassesPin] })), Response.json({ error: 'collector_unavailable' }, { status: 502 }))
  render(<App />)
  await screen.findByRole('region', { name: 'Pinned weights' })
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
  await screen.findByRole('region', { name: 'Pinned weights' })
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
  finishSecond(Response.json(switched()))
  await newModel().findByText('Suggested: -0.3')
  finishFirst(Response.json(measured({ candidate: { ...measured().candidate!, suggested_threshold_tenths: -1 } })))
  await waitFor(() => expect(button(newModel(), 'Apply model and threshold').disabled).toBe(false))
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(newModel().getByText('Suggested: -0.3')).toBeTruthy()
  expect(newModel().queryByText('Suggested: -0.1')).toBeNull()
})

test('a failed pin change shows a message beside the control, keeps the pins and calculates nothing', async () => {
  respond(
    Response.json(preview({ pins: [glassesPin] })),
    Response.json({ error: 'collector_unavailable' }, { status: 502 }),
    Response.json({ error: 'collector_unavailable' }, { status: 502 }),
  )
  render(<App />)
  await screen.findByRole('region', { name: 'Pinned weights' })
  await userEvent.click(button(weightRow('Uploader Some uploader'), 'Ignore'))
  expect((await newModel().findByRole('alert')).textContent).toMatch(/^Could not change the pin\. The request could not be confirmed\./)
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

function pinForm() {
  return within(screen.getByRole('form', { name: 'Add pin' }))
}

test('the pin form pins a tag, an uploader or a category, also without a candidate or an active model', async () => {
  const otherPin: ScorePin = { id: 8, kind: 'tag', label: 'other:tag', weight: 0 }
  const someonePin: ScorePin = { id: 9, kind: 'uploader', label: 'Someone', weight: -1.5 }
  const cosplayPin: ScorePin = { id: 10, kind: 'category', label: 'Cosplay', weight: 2 }
  const empty = preview({ candidate: null, no_model: 'no_verdicts', active: null })
  let finishPreview!: (response: Response) => void
  respond(Response.json(empty), Response.json(otherPin))
  fetchMock.mockReturnValueOnce(new Promise((resolve) => { finishPreview = resolve }))
  render(<App />)
  await screen.findByRole('region', { name: 'Pinned weights' })
  const add = () => button(pinForm(), 'Add pin')
  const value = () => pinForm().getByRole('combobox', { name: 'Tag (namespace:value)' }) as HTMLInputElement
  const weight = () => pinForm().getByRole('spinbutton', { name: 'Weight' }) as HTMLInputElement
  expect(pinForm().getByRole('heading', { name: 'Add pin' })).toBeTruthy()
  expect(weight().step).toBe('any')
  expect(add().disabled).toBe(true)
  await userEvent.type(value(), '   ')
  await userEvent.type(weight(), '0')
  expect(add().disabled).toBe(true)
  await userEvent.type(value(), 'other:tag  ')
  await userEvent.clear(weight())
  expect(add().disabled).toBe(true)
  await userEvent.type(weight(), '0')
  await userEvent.click(add())
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(listedPins()).toEqual(['other:tag 0'])
  expect(value().value).toBe('')
  expect(weight().value).toBe('')
  finishPreview(Response.json({ ...empty, pins: [otherPin] }))

  respond(Response.json(someonePin), Response.json({ ...empty, pins: [otherPin, someonePin] }))
  await userEvent.click(pinForm().getByRole('radio', { name: 'Uploader' }))
  await userEvent.type(pinForm().getByRole('textbox', { name: 'Uploader (exactly as Panda writes it)' }), ' Someone ')
  await userEvent.type(weight(), '-1.5')
  await userEvent.click(add())
  await waitFor(() => expect(previewBodies()).toHaveLength(3))
  await waitFor(() => expect(listedPins()).toEqual(['other:tag 0', 'Uploader Someone -1.5']))

  respond(Response.json(cosplayPin), Response.json({ ...empty, pins: [otherPin, someonePin, cosplayPin] }))
  await userEvent.click(pinForm().getByRole('radio', { name: 'Category' }))
  expect(pinForm().queryByRole('textbox')).toBeNull()
  await userEvent.selectOptions(pinForm().getByRole('combobox', { name: 'Category' }), 'Cosplay')
  await userEvent.type(weight(), '2')
  await userEvent.click(add())
  await waitFor(() => expect(previewBodies()).toHaveLength(4))
  await waitFor(() => expect(listedPins()).toEqual(['other:tag 0', 'Uploader Someone -1.5', 'Category Cosplay 2']))
  expect(changeCalls()).toEqual([
    ['/api/collector/catalog/score/pins', 'POST', '{"kind":"tag","value":"other:tag","weight":0}'],
    ['/api/collector/catalog/score/pins', 'POST', '{"kind":"uploader","value":"Someone","weight":-1.5}'],
    ['/api/collector/catalog/score/pins', 'POST', '{"kind":"category","value":"Cosplay","weight":2}'],
  ])
})

test('a failed pin from the form shows a message in the form, keeps the fields and the pins, and calculates nothing', async () => {
  respond(
    Response.json(preview({ pins: [glassesPin] })),
    Response.json({ error: 'tag_not_found' }, { status: 404 }),
    Response.json({ error: 'invalid_pin' }, { status: 400 }),
  )
  render(<App />)
  await screen.findByRole('region', { name: 'Pinned weights' })
  const value = () => pinForm().getByRole('combobox', { name: 'Tag (namespace:value)' }) as HTMLInputElement
  const weight = () => pinForm().getByRole('spinbutton', { name: 'Weight' }) as HTMLInputElement
  await userEvent.type(value(), 'no:such tag')
  await userEvent.type(weight(), '1.5')
  await userEvent.click(button(pinForm(), 'Add pin'))
  expect((await pinForm().findByRole('alert')).textContent).toBe('Could not change the pin. No gallery in the Panda catalog has this tag.')
  expect(value().value).toBe('no:such tag')
  expect(weight().value).toBe('1.5')

  await userEvent.clear(value())
  await userEvent.type(value(), 'nonamespace')
  await userEvent.click(button(pinForm(), 'Add pin'))
  await waitFor(() => expect(pinForm().getByRole('alert').textContent).toBe('Could not change the pin. Enter a tag as namespace:value, an uploader name, or a Panda gallery category.'))
  expect(value().value).toBe('nonamespace')
  expect(weight().value).toBe('1.5')
  expect(listedPins()).toEqual(['female:glasses 2'])
  expect(pinsSection().getAllByRole('alert')).toHaveLength(1)
  expect(newModel().queryByRole('alert')).toBeNull()
  expect(previewBodies()).toHaveLength(1)
})

const bigBreasts = { start: 0, end: 5, items: [
  { namespace: 'female', value: 'big breasts', term: 'female:"big breasts$"' },
  { namespace: 'female', value: 'big ass', term: 'female:"big ass$"' },
] }

function completionURLs() {
  return completionsMock.mock.calls.map(([input]) => String(input))
}

/** Lets the completion delay of the tag field pass. */
const afterCompletionDelay = () => new Promise((resolve) => setTimeout(resolve, 250))

test('the tag field of the pin form completes Panda tags, Enter chooses the plain tag and the next Enter adds the pin', async () => {
  const bigPin: ScorePin = { id: 8, kind: 'tag', label: 'female:big breasts', weight: 1.5 }
  completionsMock.mockImplementation(async () => Response.json(bigBreasts))
  respond(Response.json(preview()), Response.json(bigPin), Response.json(preview({ pins: [bigPin] })))
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('region', { name: 'Pinned weights' })
  const value = () => pinForm().getByRole('combobox', { name: 'Tag (namespace:value)' }) as HTMLInputElement
  await user.type(pinForm().getByRole('spinbutton', { name: 'Weight' }), '1.5')
  await user.type(value(), 'f:big')
  const suggestions = await pinForm().findAllByRole('option')
  expect(suggestions.map((option) => option.textContent)).toEqual(['femalebig breasts', 'femalebig ass'])
  expect(completionURLs()).toContain(`${completionsPath}?q=f%3Abig&cursor=5`)

  await user.keyboard('{Enter}')
  expect(value().value).toBe('female:big breasts')
  expect(pinForm().queryByRole('option')).toBeNull()
  expect(changeCalls()).toEqual([])

  await user.keyboard('{Enter}')
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/pins', 'POST', '{"kind":"tag","value":"female:big breasts","weight":1.5}']])
})

test('a clicked suggestion in the override form becomes the value of the override', async () => {
  completionsMock.mockImplementation(async () => Response.json(bigBreasts))
  respond(Response.json(preview()), noContent(), Response.json(preview()))
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('region', { name: 'Overrides' })
  const value = () => form().getByRole('combobox', { name: 'Tag (namespace:value)' }) as HTMLInputElement
  await user.type(value(), 'f:big')
  await user.click(await form().findByRole('option', { name: /big breasts/ }))
  expect(value().value).toBe('female:big breasts')
  await user.click(form().getByRole('button', { name: 'Add override' }))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/overrides', 'POST', '{"kind":"tag","value":"female:big breasts","rule":"always_hide"}']])
})

test('the uploader field of both forms has no suggestions and the category select stays', async () => {
  completionsMock.mockImplementation(async () => Response.json(bigBreasts))
  respond(Response.json(preview()))
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('region', { name: 'Overrides' })
  for (const scope of [form(), pinForm()]) {
    await user.click(scope.getByRole('radio', { name: 'Uploader' }))
    expect(scope.queryByRole('combobox')).toBeNull()
    await user.type(scope.getByRole('textbox', { name: 'Uploader (exactly as Panda writes it)' }), 'Some')
    await user.click(scope.getByRole('radio', { name: 'Category' }))
    expect(scope.getByRole('combobox', { name: 'Category' }).tagName).toBe('SELECT')
  }
  await afterCompletionDelay()
  expect(completionURLs()).toEqual([])
  expect(screen.queryByRole('option', { name: /big breasts/ })).toBeNull()
})

test('a failed completion request leaves the tag field a plain text field', async () => {
  completionsMock.mockImplementation(async () => Response.json({ error: 'collector_unavailable' }, { status: 502 }))
  const otherPin: ScorePin = { id: 8, kind: 'tag', label: 'other:tag', weight: 1 }
  respond(Response.json(preview()), Response.json(otherPin), Response.json(preview({ pins: [otherPin] })))
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('region', { name: 'Pinned weights' })
  const value = () => pinForm().getByRole('combobox', { name: 'Tag (namespace:value)' }) as HTMLInputElement
  await user.type(value(), 'other:tag')
  await afterCompletionDelay()
  expect(completionURLs()).not.toEqual([])
  expect(screen.queryByRole('alert')).toBeNull()
  expect(pinForm().queryByRole('option')).toBeNull()
  expect(value().value).toBe('other:tag')
  await user.type(pinForm().getByRole('spinbutton', { name: 'Weight' }), '1{Enter}')
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/pins', 'POST', '{"kind":"tag","value":"other:tag","weight":1}']])
})

function authorsSection() {
  return within(screen.getByRole('region', { name: 'Favorite authors' }))
}

function listedAuthors() {
  const list = authorsSection().queryByRole('list')
  return list ? within(list).getAllByRole('listitem').map((item) => [...item.querySelectorAll('[data-author-cell]')].map((cell) => cell.textContent)) : []
}

function author(index: number, overrides: Partial<ScoreFavoriteAuthor> = {}): ScoreFavoriteAuthor {
  return { label: `artist:author ${index}`, favorites: 2, weight: null, ...overrides }
}

test('lists the favorite authors in the preview order, also without a candidate', async () => {
  respond(Response.json(preview({
    candidate: null, no_model: 'no_verdicts', favorite_author_count: 1234,
    favorite_authors: [author(0, { label: 'artist:most', favorites: 31, weight: 1.5 }), author(1, { label: 'artist:fewer', favorites: 2, weight: null })],
  })))
  render(<App />)
  await screen.findByRole('region', { name: 'Favorite authors' })
  expect(authorsSection().getByText('1,234 favorite authors: artist tags that at least 2 Panda favorites carry.')).toBeTruthy()
  expect(listedAuthors()).toEqual([['artist:most', '31 favorites', '+1.50'], ['artist:fewer', '2 favorites', '–']])
  expect(authorsSection().getByText('artist:most').getAttribute('title')).toBe('artist:most')
  expect(authorsSection().queryByText(/No score model is active/)).toBeNull()
})

test('shows 50 favorite authors at first and 50 more per click', async () => {
  respond(Response.json(preview({ favorite_author_count: 120, favorite_authors: Array.from({ length: 120 }, (_, index) => author(index)) })))
  render(<App />)
  await screen.findByRole('region', { name: 'Favorite authors' })
  expect(listedAuthors()).toHaveLength(50)
  await userEvent.click(button(authorsSection(), 'Show more'))
  expect(listedAuthors()).toHaveLength(100)
  await userEvent.click(button(authorsSection(), 'Show more'))
  expect(listedAuthors()).toHaveLength(120)
  expect(listedAuthors()[119][0]).toBe('artist:author 119')
  expect(authorsSection().queryByRole('button', { name: 'Show more' })).toBeNull()
})

test('without an active model the section says that the author bonus has no effect', async () => {
  respond(Response.json(preview({ candidate: null, no_model: 'no_verdicts', active: null })))
  render(<App />)
  await screen.findByRole('region', { name: 'Favorite authors' })
  expect(authorsSection().getByText('No score model is active: the author bonus has no effect.')).toBeTruthy()
})

test('Save bonus and Turn off bonus change the bonus, keep the threshold and calculate again', async () => {
  respond(
    Response.json(measured()),
    noContent(), Response.json(measured({ author_bonus: 1.5 })),
    noContent(), Response.json(measured()),
  )
  render(<App />)
  await screen.findByRole('region', { name: 'Favorite authors' })
  const bonus = () => authorsSection().getByRole('spinbutton', { name: 'Author bonus' }) as HTMLInputElement
  expect(bonus().value).toBe('')
  expect(bonus().step).toBe('0.1')
  expect(authorsSection().getByText('The author bonus is off.')).toBeTruthy()
  expect(authorsSection().queryByRole('button', { name: 'Turn off bonus' })).toBeNull()
  expect(button(authorsSection(), 'Save bonus').disabled).toBe(true)
  await userEvent.type(bonus(), '0')
  expect(button(authorsSection(), 'Save bonus').disabled).toBe(true)
  await userEvent.clear(bonus())
  await userEvent.type(bonus(), '-1')
  expect(button(authorsSection(), 'Save bonus').disabled).toBe(true)

  await enter('Threshold', '-0.2')
  await userEvent.clear(bonus())
  await userEvent.type(bonus(), '1.50')
  await userEvent.click(button(authorsSection(), 'Save bonus'))
  await authorsSection().findByText("Each favorite author's tag weighs at least +1.50.")
  expect(bonus().value).toBe('1.5')
  expect(field('Threshold').value).toBe('-0.2')

  await userEvent.click(button(authorsSection(), 'Turn off bonus'))
  await authorsSection().findByText('The author bonus is off.')
  expect(bonus().value).toBe('')
  expect(field('Threshold').value).toBe('-0.2')
  expect(changeCalls()).toEqual([
    ['/api/collector/catalog/score/author-bonus', 'PUT', '{"bonus":1.5}'],
    ['/api/collector/catalog/score/author-bonus', 'PUT', '{"bonus":null}'],
  ])
  expect(previewBodies()).toEqual(['{"favorites":null}', '{"favorites":true}', '{"favorites":true}'])
})

test('a failed bonus change shows a message in the section and calculates nothing', async () => {
  respond(Response.json(preview()), Response.json({ error: 'invalid_author_bonus' }, { status: 400 }))
  render(<App />)
  await screen.findByRole('region', { name: 'Favorite authors' })
  await userEvent.type(authorsSection().getByRole('spinbutton', { name: 'Author bonus' }), '9999')
  await userEvent.click(button(authorsSection(), 'Save bonus'))
  expect((await authorsSection().findByRole('alert')).textContent).toBe('Could not save the author bonus. Enter a number above 0.')
  expect(authorsSection().getByText('The author bonus is off.')).toBeTruthy()
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
  await screen.findByRole('region', { name: 'Favorite authors' })
  await userEvent.type(pinField('artist:most'), '2.5')
  await userEvent.click(button(authorsSection(), 'Pin'))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  expect(listedPins()).toEqual(['artist:most 2.5'])
  expect(authorsSection().getByRole('button', { name: 'Remove pin' })).toBeTruthy()
  expect(changeCalls()).toEqual([['/api/collector/catalog/score/pins', 'POST', '{"kind":"tag","value":"artist:most","weight":2.5}']])
})

test('a failed pin change from the author list shows its message in the section', async () => {
  respond(
    Response.json(preview({ favorite_author_count: 1, favorite_authors: [author(0)] })),
    Response.json({ error: 'collector_unavailable' }, { status: 502 }),
  )
  render(<App />)
  await screen.findByRole('region', { name: 'Favorite authors' })
  await userEvent.click(button(authorsSection(), 'Ignore'))
  expect((await authorsSection().findByRole('alert')).textContent).toMatch(/^Could not change the pin\. The request could not be confirmed\./)
  expect(pinsSection().queryByRole('alert')).toBeNull()
  expect(newModel().queryByRole('alert')).toBeNull()
  expect(previewBodies()).toHaveLength(1)
})

function filterAuthors() {
  return authorsSection().getByRole('textbox', { name: 'Filter favorite authors' }) as HTMLInputElement
}

test('the favorite authors filter searches every listed author by its tag and pages through the matches', async () => {
  respond(Response.json(preview({ favorite_author_count: 300, favorite_authors: Array.from({ length: 120 }, (_, index) => author(index)) })))
  render(<App />)
  await screen.findByRole('region', { name: 'Favorite authors' })
  expect(authorsSection().getByText('50 of 300 favorite authors, most favorites first.')).toBeTruthy()
  expect(filterAuthors().placeholder).toBe('Filter by artist')
  await typeFilter(filterAuthors(), ' AUTHOR 11 ')
  expect(listedAuthors().map(([label]) => label)).toEqual(['artist:author 11', ...Array.from({ length: 10 }, (_, index) => `artist:author ${110 + index}`)])
  expect(authorsSection().getByText('11 of 11 matching favorite authors, most favorites first. Only the 120 authors with the most favorites of 300 are searched.')).toBeTruthy()
  await typeFilter(filterAuthors(), 'zzz')
  expect(authorsSection().queryByRole('list')).toBeNull()
  expect(authorsSection().getByText('No favorite authors match. Only the 120 authors with the most favorites of 300 are searched.')).toBeTruthy()
  await typeFilter(filterAuthors(), 'artist')
  expect(authorsSection().getByText(/^50 of 120 matching favorite authors, most favorites first\./)).toBeTruthy()
  await userEvent.click(button(authorsSection(), 'Show more'))
  expect(listedAuthors()).toHaveLength(100)
  await userEvent.type(filterAuthors(), ':')
  expect(listedAuthors()).toHaveLength(50)
})

test('the favorite authors filter says nothing about its scope when every author is listed, and stays after a pin change', async () => {
  const authors = Array.from({ length: 120 }, (_, index) => author(index))
  const body = preview({ favorite_author_count: 120, favorite_authors: authors })
  const pin: ScorePin = { id: 12, kind: 'tag', label: 'artist:author 70', weight: 1 }
  respond(Response.json(body), Response.json(pin), Response.json({ ...body, pins: [pin] }))
  render(<App />)
  await screen.findByRole('region', { name: 'Favorite authors' })
  await typeFilter(filterAuthors(), 'artist')
  expect(authorsSection().getByText('50 of 120 matching favorite authors, most favorites first.')).toBeTruthy()
  await userEvent.click(button(authorsSection(), 'Show more'))
  await userEvent.type(pinField('artist:author 70'), '1')
  await userEvent.click(within(pinField('artist:author 70').closest('li')!).getByRole('button', { name: 'Pin' }))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))
  await waitFor(() => expect(listedPins()).toEqual(['artist:author 70 1']))
  expect(filterAuthors().value).toBe('artist')
  expect(listedAuthors()).toHaveLength(100)
  await typeFilter(filterAuthors(), 'zzz')
  expect(authorsSection().getByText('No favorite authors match.')).toBeTruthy()
})

test('a preview without favorite authors shows no filter and no count line', async () => {
  respond(Response.json(preview()))
  render(<App />)
  await screen.findByRole('region', { name: 'Favorite authors' })
  expect(authorsSection().queryByRole('textbox', { name: 'Filter favorite authors' })).toBeNull()
  expect(authorsSection().queryByText(/most favorites first/)).toBeNull()
})
