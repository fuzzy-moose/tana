import { Flex, Switch, Text } from '@radix-ui/themes'
import { useEffect, useRef, useState } from 'react'
import { setMainBackgroundPaused } from './api'

export default function MetadataCollection({ paused, available, onChanged }: {
  paused: boolean
  available: boolean
  onChanged: (paused: boolean) => void
}) {
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const mutation = useRef<AbortController | null>(null)
  useEffect(() => () => mutation.current?.abort(), [])

  async function update() {
    if (mutation.current || !available) return
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setError('')
    try {
      await setMainBackgroundPaused(!paused, controller.signal)
      if (!controller.signal.aborted) onChanged(!paused)
    } catch (error) {
      if (!controller.signal.aborted) setError((error as Error).message)
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) setPending(false)
    }
  }

  return <Flex direction="column" gap="1" align="start">
    <Text as="label" size="2" color="gray">
      <Flex as="span" align="center" gap="2">
        <Switch checked={!paused} disabled={!available || pending} onCheckedChange={() => void update()} />
        Background collection
      </Flex>
    </Text>
    {error && <Text as="p" size="2" color="red" role="alert">{error}</Text>}
  </Flex>
}
