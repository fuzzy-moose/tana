import { useState } from 'react'
import { Badge, Button, Callout, Checkbox, Dialog, Flex, Text, TextField } from '@radix-ui/themes'
import { ExclamationTriangleIcon } from '@radix-ui/react-icons'
import { hasDefaultFilter, savePandaDefaultFilter } from './api'
import type { PandaDefaultFilter as DefaultFilter } from './api'
import { pandaCategories } from './categories'

export default function PandaDefaultFilter({ filter, error, bypassed, onSave, onRetry }: {
  filter: DefaultFilter | null
  error: string
  bypassed: boolean
  onSave: (filter: DefaultFilter) => void
  onRetry: () => void
}) {
  const [open, setOpen] = useState(false)
  return <Dialog.Root open={open} onOpenChange={setOpen}>
    <Dialog.Trigger><Button variant="soft" color="gray">Default filter {error ? <Badge color="red">Unavailable</Badge> : hasDefaultFilter(filter) && <Badge color={bypassed ? 'gray' : 'green'}>{bypassed ? 'Bypassed' : 'Active'}</Badge>}</Button></Dialog.Trigger>
    <Dialog.Content maxWidth="540px">
      <Dialog.Title>Default filter</Dialog.Title>
      <Dialog.Description size="2" color="gray" mb="4">Shared across devices. These categories and search terms also apply while you search the Panda catalog.</Dialog.Description>
      {error ? <Flex direction="column" gap="3">
        <Callout.Root color="red" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>
        <Button variant="soft" onClick={onRetry}>Retry</Button>
      </Flex> : filter ? <DefaultFilterEditor filter={filter} onSave={(next) => { onSave(next); setOpen(false) }} /> : <Text role="status">Loading default filter…</Text>}
    </Dialog.Content>
  </Dialog.Root>
}

function DefaultFilterEditor({ filter, onSave }: { filter: DefaultFilter, onSave: (filter: DefaultFilter) => void }) {
  const [query, setQuery] = useState(filter.query)
  const [categories, setCategories] = useState(filter.categories)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  return <form onSubmit={async (event) => {
    event.preventDefault()
    setSaving(true)
    setError('')
    try { onSave(await savePandaDefaultFilter({ query: query.trim(), categories })) }
    catch (error) { setError((error as Error).message) }
    finally { setSaving(false) }
  }}>
    <Flex direction="column" gap="4">
      <Flex direction="column" gap="2">
        <Text as="label" htmlFor="panda-default-query" size="2" weight="medium">Default search</Text>
        <TextField.Root id="panda-default-query" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="-l:japanese$" disabled={saving} aria-describedby="panda-default-query-help" />
        <Text id="panda-default-query-help" size="1" color="gray">Use title and tag search syntax. For example, -l:japanese$ excludes Japanese.</Text>
      </Flex>
      <fieldset className="panda-default-categories" disabled={saving}>
        <Text asChild size="2" weight="medium"><legend>Categories</legend></Text>
        <Text as="p" size="1" color="gray">No selection includes all categories.</Text>
        <div className="panda-default-category-grid">
          {pandaCategories.map((label) => {
            const category = label.toLowerCase()
            return <Text as="label" size="2" key={category}><Flex gap="2" align="center">
              <Checkbox checked={categories.includes(category)} onCheckedChange={(checked) => setCategories((current) => checked ? [...current, category] : current.filter((value) => value !== category))} />{label}
            </Flex></Text>
          })}
        </div>
      </fieldset>
      {error && <Callout.Root color="red" role="alert"><Callout.Icon><ExclamationTriangleIcon /></Callout.Icon><Callout.Text>{error}</Callout.Text></Callout.Root>}
      <Flex gap="3" justify="end">
        <Dialog.Close><Button variant="soft" color="gray" type="button" disabled={saving}>Cancel</Button></Dialog.Close>
        <Button type="submit" loading={saving}>Save default filter</Button>
      </Flex>
    </Flex>
  </form>
}
