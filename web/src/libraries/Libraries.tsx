import { useEffect, useRef, useState } from 'react'
import { Badge, Box, Button, Callout, Card, Flex, Grid, Heading, Text } from '@radix-ui/themes'
import { PlusIcon, ReloadIcon } from '@radix-ui/react-icons'
import { checkLibraryAvailability, createLibrary, listLibraries, removeLibrary, renameLibrary } from './api'
import type { Library } from './api'
import LibraryForm from './LibraryForm'
import { useScan } from '../scans/useScan'
import ScanPanel from '../scans/ScanPanel'
import './Libraries.css'

type Editor = { type: 'add' } | { type: 'rename' | 'remove', library: Library }

const availabilityLabels = {
  available: 'Available',
  unavailable: 'Unavailable',
  unknown: 'Not yet checked',
}

export default function Libraries() {
  const scan = useScan()
  const [libraries, setLibraries] = useState<Library[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [editor, setEditor] = useState<Editor | null>(null)
  const [busy, setBusy] = useState<number | null>(null)
  const [removeError, setRemoveError] = useState('')
  const [refresh, setRefresh] = useState(0)
  const checkController = useRef<AbortController | null>(null)
  const editorTrigger = useRef<HTMLButtonElement | null>(null)
  const addButton = useRef<HTMLButtonElement | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    listLibraries(controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setLibraries(result)
      })
      .catch((error: Error) => {
        if (!controller.signal.aborted) setError(error.message)
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [refresh])

  useEffect(() => () => checkController.current?.abort(), [])

  const locked = loading || busy !== null || editor !== null

  function refreshLibraries() {
    setLoading(true)
    setError('')
    setNotice('')
    setRefresh((value) => value + 1)
  }

  function openEditor(next: Editor, trigger: HTMLButtonElement) {
    editorTrigger.current = trigger
    setNotice('')
    setRemoveError('')
    setEditor(next)
  }

  function closeEditor(removed = false) {
    setEditor(null)
    // Wait for the editor to unmount and its trigger to become enabled again.
    requestAnimationFrame(() => {
      const trigger = editorTrigger.current
      const target = removed || !trigger?.isConnected ? addButton.current : trigger
      target?.focus()
    })
  }

  function updateLibrary(library: Library) {
    setLibraries((current) => current.map((item) => item.id === library.id ? library : item))
  }

  async function save(name: string, path: string) {
    if (editor?.type === 'add') {
      const library = await createLibrary(name, path)
      setLibraries((current) => [...current, library])
      setNotice(`Added “${library.name}”.`)
    } else if (editor?.type === 'rename') {
      const library = await renameLibrary(editor.library.id, name)
      updateLibrary(library)
      setNotice(`Renamed library to “${library.name}”.`)
    }
    closeEditor()
  }

  async function remove(library: Library) {
    setBusy(library.id)
    setRemoveError('')
    try {
      await removeLibrary(library.id)
      setLibraries((current) => current.filter((item) => item.id !== library.id))
      setNotice(`Removed “${library.name}”. Files were left untouched.`)
      closeEditor(true)
    } catch (error) {
      setRemoveError(error instanceof Error ? error.message : 'Could not remove the library. Try again.')
    } finally {
      setBusy(null)
    }
  }

  async function check(library: Library) {
    const controller = new AbortController()
    checkController.current = controller
    setBusy(library.id)
    setError('')
    setNotice(`Checking “${library.name}”…`)
    try {
      const result = await checkLibraryAvailability(library.id, controller.signal)
      updateLibrary(result.library)
      setNotice(result.completed
        ? `“${result.library.name}”: ${availabilityLabels[result.library.availability].toLowerCase()}.`
        : `Check requested for “${library.name}”; still waiting for a result. Refresh the list shortly.`)
    } catch (error) {
      if (!controller.signal.aborted) {
        setNotice('')
        setError(error instanceof Error ? error.message : 'Could not check availability. Try again.')
      }
    } finally {
      if (!controller.signal.aborted) setBusy(null)
    }
  }

  return <Box asChild maxWidth="1200px" mx="auto">
    <section id="library" aria-labelledby="library-title">
      <Flex direction="column" gap="5">
        <Flex justify="between" align="center" gap="3" wrap="wrap">
          <Box>
            <Heading as="h1" id="library-title" size="7">Libraries</Heading>
            <Text as="p" size="2" color="gray" mt="1">Your folders, scans, and collection maintenance.</Text>
          </Box>
          <Flex gap="2" wrap="wrap">
            <Button variant="soft" type="button" disabled={locked || scan.disabled || libraries.length === 0} onClick={() => void scan.start()}><ReloadIcon />Scan all</Button>
            <Button ref={addButton} type="button" disabled={locked} onClick={(event) => openEditor({ type: 'add' }, event.currentTarget)}><PlusIcon />Add library</Button>
          </Flex>
        </Flex>

        <Text as="p" size="2" color="gray" role="status" className="library-notice">{loading ? 'Loading libraries…' : notice}</Text>
        {error && <Callout.Root color="red" role="alert">
          <Callout.Text>{error}</Callout.Text>
          <Button variant="soft" color="red" type="button" disabled={locked} onClick={refreshLibraries}>Retry</Button>
        </Callout.Root>}

        {editor?.type === 'add' && <LibraryForm onSave={save} onCancel={() => closeEditor()} />}

        {!loading && !error && libraries.length === 0 && !editor && <Card size="5">
          <Flex direction="column" align="center" gap="3" py="6">
            <Heading as="h2" size="5">Your collection starts here.</Heading>
            <Text as="p" color="gray" align="center">Add a library root to register your local comics and manga.</Text>
            <Button type="button" onClick={(event) => openEditor({ type: 'add' }, event.currentTarget)}>Add your first library</Button>
          </Flex>
        </Card>}

        {libraries.length > 0 && <Flex asChild direction="column" gap="3" m="0" p="0">
          <ul className="library-list" aria-label="Registered libraries" aria-busy={loading}>
            {[...libraries].sort((a, b) => a.name.localeCompare(b.name) || a.id - b.id).map((library) => <Card asChild size="3" key={library.id}>
              <li>
                <Flex direction="column" gap="4">
                  <Flex justify="between" align="start" gap="3" wrap="wrap">
                    <Box minWidth="0">
                      <Flex align="center" gap="3" wrap="wrap">
                        <Heading as="h2" size="4">{library.name}</Heading>
                        <Badge color={library.availability === 'available' ? 'green' : library.availability === 'unavailable' ? 'amber' : 'gray'}>{availabilityLabels[library.availability]}</Badge>
                      </Flex>
                      <Text as="p" size="2" className="library-path" mt="2">{library.path}</Text>
                      <Text as="p" size="1" color="gray" mt="1">{library.last_checked_at
                        ? <>Last checked <time dateTime={library.last_checked_at}>{new Date(library.last_checked_at).toLocaleString()}</time></>
                        : 'No availability check recorded'}</Text>
                    </Box>
                    <Flex gap="2" wrap="wrap" aria-label={`Actions for ${library.name}`}>
                      <Button variant="soft" type="button" disabled={locked || scan.disabled} onClick={() => void scan.start(library.id)}>Scan</Button>
                      <Button variant="soft" color="gray" asChild><a href={`#/libraries/${library.id}/refresh`}>Refresh</a></Button>
                      <Button variant="outline" color="gray" type="button" disabled={locked} onClick={() => void check(library)}>{busy === library.id && !editor ? 'Checking…' : 'Check availability'}</Button>
                      <Button variant="ghost" color="gray" mx="1" type="button" disabled={locked} onClick={(event) => openEditor({ type: 'rename', library }, event.currentTarget)}>Rename</Button>
                      <Button variant="ghost" color="red" mx="1" type="button" disabled={locked} onClick={(event) => openEditor({ type: 'remove', library }, event.currentTarget)}>Remove</Button>
                    </Flex>
                  </Flex>
                  {editor?.type === 'rename' && editor.library.id === library.id && <LibraryForm library={library} onSave={save} onCancel={() => closeEditor()} />}
                  {editor?.type === 'remove' && editor.library.id === library.id && <Card role="group" aria-label={`Remove ${library.name}`}>
                    <Flex direction="column" gap="3">
                      <Heading as="h2" size="4">Remove “{library.name}”?</Heading>
                      <Text as="p" size="2">This removes the library registration. Files in the library root stay untouched.</Text>
                      {removeError && <Callout.Root color="red" role="alert"><Callout.Text>{removeError}</Callout.Text></Callout.Root>}
                      <Flex gap="2">
                        <Button autoFocus variant="soft" color="gray" type="button" disabled={busy !== null} onClick={() => closeEditor()}>Cancel</Button>
                        <Button color="red" type="button" disabled={busy !== null} onClick={() => void remove(library)}>{busy ? 'Removing…' : 'Remove library'}</Button>
                      </Flex>
                    </Flex>
                  </Card>}
                </Flex>
              </li>
            </Card>)}
          </ul>
        </Flex>}
        <ScanPanel scan={scan} libraries={libraries} />
        <Box>
          <Flex align="center" justify="between" gap="3" mb="3">
            <Heading as="h2" size="4">Maintenance</Heading>
            <Button variant="ghost" color="gray" type="button" disabled={locked} onClick={refreshLibraries}>Reload list</Button>
          </Flex>
          <Grid columns={{ initial: '1', sm: '2' }} gap="3">
            <Card asChild size="3"><a href="#/libraries/refresh" aria-labelledby="refresh-all-title" aria-describedby="refresh-all-description">
              <Heading id="refresh-all-title" as="h3" size="3" mb="1">Refresh all</Heading>
              <Text id="refresh-all-description" as="p" size="2" color="gray">Review and remove catalog entries for missing files.</Text>
            </a></Card>
            <Card asChild size="3"><a href="#/libraries/source-cleanup" aria-labelledby="source-cleanup-title" aria-describedby="source-cleanup-description">
              <Heading id="source-cleanup-title" as="h3" size="3" mb="1">Clean up older versions</Heading>
              <Text id="source-cleanup-description" as="p" size="2" color="gray">Review superseded archives and free up storage.</Text>
            </a></Card>
          </Grid>
          <Text as="p" size="1" color="gray" mt="3">Availability indicates whether the library root is present as a directory. An unavailable root stays registered.</Text>
        </Box>
      </Flex>
    </section>
  </Box>
}
