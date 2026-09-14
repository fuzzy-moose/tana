import { useId, useState } from 'react'
import type { SubmitEvent } from 'react'
import { Button, Callout, Card, Flex, Heading, Text, TextField } from '@radix-ui/themes'
import type { Library } from './api'

interface LibraryFormProps {
  library?: Library
  onSave: (name: string, path: string) => Promise<void>
  onCancel: () => void
}

export default function LibraryForm({ library, onSave, onCancel }: LibraryFormProps) {
  const [name, setName] = useState(library?.name ?? '')
  const [path, setPath] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const pathId = useId()
  const title = library ? 'Rename library' : 'Add library'

  async function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    if (saving) return
    if (!name.trim()) {
      setError('Enter a library name.')
      return
    }
    setSaving(true)
    setError('')
    try {
      await onSave(name.trim(), path)
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Could not save the library. Try again.')
    } finally {
      setSaving(false)
    }
  }

  return <Card size="3">
    <form aria-label={title} onSubmit={submit}>
      <Flex direction="column" gap="4">
        <Heading as="h2" size="4">{title}</Heading>
        <Text as="label" size="2" weight="medium">
          <Flex direction="column" gap="2">
            Name
            <TextField.Root autoFocus name="name" autoComplete="off" required disabled={saving} value={name} onChange={(event) => setName(event.target.value)} />
          </Flex>
        </Text>
        {!library && <Flex direction="column" gap="2">
          <Text as="label" htmlFor={pathId} size="2" weight="medium">Library root</Text>
          <TextField.Root id={pathId} name="path" autoComplete="off" spellCheck={false} required disabled={saving} value={path} onChange={(event) => setPath(event.target.value)} aria-describedby={`${pathId}-help`} placeholder="/mnt/comics" />
          <Text id={`${pathId}-help`} size="1" color="gray">Absolute directory path on the machine running Tana.</Text>
        </Flex>}
        {library && <Text as="p" size="2" color="gray" className="library-path">{library.path}</Text>}
        {error && <Callout.Root color="red" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>}
        <Flex gap="2">
          <Button type="submit" disabled={saving}>{saving ? 'Saving…' : library ? 'Save name' : 'Add library'}</Button>
          <Button variant="soft" color="gray" type="button" disabled={saving} onClick={onCancel}>Cancel</Button>
        </Flex>
      </Flex>
    </form>
  </Card>
}
