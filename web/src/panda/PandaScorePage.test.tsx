// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import App from '../App'
import type { ScorePreview, ScoreWeight } from './score'

vi.mock('../collector/FavoriteSyncAction', () => ({ default: () => null }))

const fetchMock = vi.fn<typeof fetch>()

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
  return within(screen.getByRole('table')).getAllByRole('row').slice(1).map((row) => within(row).getAllByRole('cell').slice(0, 6).map((cell) => cell.textContent))
}

beforeEach(() => {
  window.history.replaceState(null, '', '#/panda/score')
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
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
  await userEvent.click(within(within(screen.getByRole('table')).getByText('tag:value 60').closest('tr')!).getByRole('button', { name: 'Always hide' }))
  await screen.findByText('Always hide tag:value 60')
  expect(previewBodies()).toHaveLength(2)
  expect(rows()).toHaveLength(100)
})

test('warns about few verdicts and still shows the table', async () => {
  respond(Response.json(preview({ evidence: { ...preview().evidence, few_verdicts: true } })))
  render(<App />)
  expect((await screen.findByText(/Few verdicts\./)).textContent).toBe('Few verdicts. Weights and error rates from so little evidence are unreliable.')
  expect(screen.getByRole('table')).toBeTruthy()
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
  expect(screen.queryByRole('table')).toBeNull()
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
  expect(screen.getByRole('table')).toBeTruthy()
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
  return within(within(screen.getByRole('table')).getByText(label).closest('tr')!)
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
  const value = () => form().getByRole('textbox') as HTMLInputElement
  expect(add().disabled).toBe(true)
  await userEvent.type(value(), '   ')
  expect(add().disabled).toBe(true)
  await userEvent.type(value(), 'other:tag  ')
  await userEvent.click(add())
  await waitFor(() => expect(value().value).toBe(''))
  await waitFor(() => expect(previewBodies()).toHaveLength(2))

  await userEvent.click(form().getByRole('radio', { name: 'Uploader' }))
  await userEvent.click(form().getByRole('radio', { name: 'Never hide' }))
  await userEvent.type(value(), ' Someone ')
  await userEvent.click(add())
  await waitFor(() => expect(value().value).toBe(''))

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
  await userEvent.type(form().getByRole('textbox'), 'no:such tag')
  await userEvent.click(form().getByRole('button', { name: 'Add override' }))
  expect((await form().findByRole('alert')).textContent).toBe('Could not change the override. No gallery in the Panda catalog has this tag.')
  expect((form().getByRole('textbox') as HTMLInputElement).value).toBe('no:such tag')
  expect(listedOverrides()).toEqual(['Always hide female:glasses'])
  expect(previewBodies()).toHaveLength(1)

  await userEvent.click(weightRow('Uploader Some uploader').getByRole('button', { name: 'Never hide' }))
  expect((await newModel().findByRole('alert')).textContent).toMatch(/^Could not change the override\. The request could not be confirmed\./)
  expect(form().queryByRole('alert')).toBeNull()
  expect(pressed('Uploader Some uploader', 'Never hide')).toBe('false')
  expect(listedOverrides()).toEqual(['Always hide female:glasses'])
  expect(previewBodies()).toHaveLength(1)
})
