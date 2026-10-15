import { Button, Flex, Text } from '@radix-ui/themes'
import { useEffect, useRef, useState } from 'react'
import { resetBan } from './api'
import type { BanGroup } from './api'

export default function BanReset({ group, label, available, onReset }: {
  group: BanGroup
  label: string
  available: boolean
  onReset: () => void
}) {
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const mutation = useRef<AbortController | null>(null)
  useEffect(() => () => mutation.current?.abort(), [])

  async function reset() {
    if (mutation.current || !available) return
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setError('')
    try {
      await resetBan(group, controller.signal)
      if (!controller.signal.aborted) onReset()
    } catch (error) {
      if (!controller.signal.aborted) setError((error as Error).message)
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) setPending(false)
    }
  }

  return <Flex direction="column" gap="2" align="start">
    <Button size="1" variant="soft" type="button" color="gray" disabled={!available || pending} onClick={() => void reset()}>
      {pending ? 'Resetting…' : label}
    </Button>
    <Text as="p" size="1" color="gray">Use when the cooldown was recorded in error. If Panda still bans these requests, the next one records the ban again.</Text>
    {error && <Text as="p" size="2" color="red" role="alert">{error}</Text>}
  </Flex>
}
