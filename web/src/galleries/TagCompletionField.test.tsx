// @vitest-environment jsdom
import { useState } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import type { SearchCompletion } from './api'
import TagCompletionField from './TagCompletionField'

const complete = vi.fn<(query: string, cursor: number, signal?: AbortSignal) => Promise<SearchCompletion>>()

function Field({ id, label, onChange, insert }: { id: string, label: string, onChange?: (value: string) => void, insert?: 'term' | 'tag' }) {
  const [value, setValue] = useState('')
  return <TagCompletionField id={id} aria-label={label} value={value} complete={complete} insert={insert}
    onChange={(next) => { setValue(next); onChange?.(next) }} />
}

beforeEach(() => {
  complete.mockReset()
  complete.mockResolvedValue({ start: 0, end: 3, items: [{ namespace: 'artist', value: 'artist y', term: 'artist:"artist y$"' }] })
})
afterEach(cleanup)

test('two fields keep their suggestion ids apart', async () => {
  const user = userEvent.setup()
  render(<><Field id="pin" label="Pin" /><Field id="override" label="Override" /></>)
  const override = screen.getByRole('combobox', { name: 'Override' })
  await user.type(override, 'art')
  const list = await screen.findByRole('listbox', { name: 'Tag suggestions' })
  expect(override.getAttribute('aria-controls')).toBe('override-suggestions')
  expect(list.id).toBe('override-suggestions')
  expect(document.querySelectorAll('#override-suggestions')).toHaveLength(1)
  expect(override.getAttribute('aria-activedescendant')).toBe('override-suggestion-0')
  expect(screen.getByRole('combobox', { name: 'Pin' }).getAttribute('aria-controls')).toBeNull()
})

test('with insert="tag" a chosen suggestion replaces the whole text with its plain tag', async () => {
  complete.mockResolvedValue({ start: 4, end: 11, items: [{ namespace: 'female', value: 'long hair', term: 'female:"long hair$"' }] })
  const onChange = vi.fn()
  const user = userEvent.setup()
  render(<Field id="pin" label="Pin" insert="tag" onChange={onChange} />)
  const field = screen.getByRole<HTMLInputElement>('combobox')
  await user.type(field, 'f:long ha')
  await user.click(await screen.findByRole('option'))
  expect(onChange).toHaveBeenLastCalledWith('female:long hair')
  expect(field.value).toBe('female:long hair')
  expect(field.selectionStart).toBe('female:long hair'.length)
  expect(field.selectionEnd).toBe('female:long hair'.length)
})
