import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Badge, Text, TextField } from '@radix-ui/themes'
import type { SearchCompletion, TagSuggestion } from './api'
import './TagCompletionField.css'

interface TagCompletionFieldProps {
  /** The input's id. The suggestion list is `<id>-suggestions`, its options `<id>-suggestion-<index>`. */
  id: string
  value: string
  onChange: (value: string) => void
  /** Requests the suggestions for the text and the cursor (a UTF-16 offset). */
  complete: (query: string, cursor: number, signal?: AbortSignal) => Promise<SearchCompletion>
  type?: 'text' | 'search'
  placeholder?: string
  'aria-label'?: string
  /** `TextField.Slot` content, such as the search icon. */
  children?: ReactNode
  /** What a chosen suggestion inserts: its search `term` over the completed range, or its plain `namespace:value` as the whole text. */
  insert?: 'term' | 'tag'
}

const suggestionId = (id: string, index: number) => `${id}-suggestion-${index}`

export default function TagCompletionField({ id, value, onChange, complete, type = 'text', placeholder, 'aria-label': ariaLabel, children, insert = 'term' }: TagCompletionFieldProps) {
  const [cursor, setCursor] = useState(value.length)
  const [focused, setFocused] = useState(false)
  const [dismissed, setDismissed] = useState(false)
  const [composing, setComposing] = useState(false)
  const [response, setResponse] = useState<{ query: string, cursor: number, result: SearchCompletion } | null>(null)
  const [active, setActive] = useState(0)
  const input = useRef<HTMLInputElement>(null)
  const pendingCursor = useRef<number | null>(null)
  const completion = response?.query === value && response.cursor === cursor ? response.result : null
  const open = focused && !dismissed && !composing && !!completion?.items.length
  const listId = `${id}-suggestions`
  const optionId = (index: number) => suggestionId(id, index)

  useEffect(() => {
    if (!focused || dismissed || composing || !value.trim() || cursor === 0) return
    const controller = new AbortController()
    const timer = setTimeout(async () => {
      try {
        const result = await complete(value, cursor, controller.signal)
        if (!controller.signal.aborted) { setResponse({ query: value, cursor, result }); setActive(0) }
      } catch {
        // Completion is optional; a failed request must not interrupt typing.
      }
    }, 150)
    return () => { controller.abort(); clearTimeout(timer) }
  }, [value, cursor, focused, dismissed, composing, complete])

  useLayoutEffect(() => {
    if (pendingCursor.current === null) return
    input.current?.focus()
    input.current?.setSelectionRange(pendingCursor.current, pendingCursor.current)
    pendingCursor.current = null
  }, [value])

  useEffect(() => {
    if (open) document.getElementById(suggestionId(id, active))?.scrollIntoView?.({ block: 'nearest' })
  }, [id, active, open])

  function select(suggestion: TagSuggestion) {
    if (!completion) return
    const tag = `${suggestion.namespace}:${suggestion.value}`
    const next = insert === 'tag' ? tag : value.slice(0, completion.start) + suggestion.term + value.slice(completion.end)
    const position = insert === 'tag' ? tag.length : completion.start + suggestion.term.length
    pendingCursor.current = position
    onChange(next)
    setCursor(position)
    setDismissed(true)
    setResponse(null)
  }

  return (
    <div className="tag-completion-field">
      <TextField.Root ref={input} id={id} type={type} role="combobox" aria-label={ariaLabel}
        aria-autocomplete="list" aria-expanded={open} aria-controls={open ? listId : undefined}
        aria-activedescendant={open ? optionId(active) : undefined} autoComplete="off"
        value={value} placeholder={placeholder}
        onChange={(event) => {
          onChange(event.target.value)
          setCursor(event.target.selectionStart ?? event.target.value.length)
          setDismissed(false)
          setResponse(null)
        }}
        onSelect={(event) => { setCursor(event.currentTarget.selectionStart ?? value.length) }}
        onFocus={() => setFocused(true)} onBlur={() => { setFocused(false); setResponse(null) }}
        onClick={() => setDismissed(false)}
        onCompositionStart={() => setComposing(true)} onCompositionEnd={() => setComposing(false)}
        onKeyDown={(event) => {
          if (event.nativeEvent.isComposing || composing) return
          if (event.key === 'Escape') {
            event.preventDefault()
            setDismissed(true)
          } else if (open && completion) {
            if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
              event.preventDefault()
              setActive((index) => (index + (event.key === 'ArrowDown' ? 1 : -1) + completion.items.length) % completion.items.length)
            } else if (event.key === 'Enter') {
              event.preventDefault()
              select(completion.items[active])
            }
          } else if (event.key === 'Enter') {
            // The key is left to the form; a request still pending must not reopen suggestions.
            setDismissed(true)
          }
        }}>
        {children}
      </TextField.Root>
      {open && <ul id={listId} className="tag-completion-suggestions" role="listbox" aria-label="Tag suggestions">
        {completion.items.map((suggestion, index) => <li key={`${suggestion.namespace}:${suggestion.value}`}
          id={optionId(index)} role="option" aria-selected={index === active}
          onMouseDown={(event) => event.preventDefault()} onMouseEnter={() => setActive(index)}
          onClick={() => select(suggestion)}>
          <Badge color="gray" size="1">{suggestion.namespace}</Badge><Text size="2">{suggestion.value}</Text>
        </li>)}
      </ul>}
    </div>
  )
}
