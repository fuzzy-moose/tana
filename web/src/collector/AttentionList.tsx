import { Button, Callout, Flex, Heading, Text } from '@radix-ui/themes'
import { useBanReset } from './useBanReset'
import type { BanGroup } from './api'
import type { AttentionItem } from './attention'

function ResetAction({ group, label, tone, available, onReset }: {
  group: BanGroup, label: string, tone: AttentionItem['tone'], available: boolean, onReset: () => void,
}) {
  const { pending, error, reset } = useBanReset(group, available, onReset)

  return <>
    <Button className="overview-attention-action" size="2" variant="soft" color={tone} type="button"
      disabled={!available || pending} onClick={reset}>{pending ? 'Resetting…' : label}</Button>
    {error && <Text as="p" size="2" color="red" role="alert" style={{ flexBasis: '100%' }}>{error}</Text>}
  </>
}

export default function AttentionList({ items, available, onReset }: { items: AttentionItem[], available: boolean, onReset: () => void }) {
  if (items.length === 0) return null
  return <Flex direction="column" gap="2" asChild><section aria-labelledby="attention-title">
    <Heading as="h2" size="1" weight="medium" color="gray" id="attention-title" className="overview-section-label">Needs attention · {items.length}</Heading>
    {items.map((item) => <Callout.Root key={item.title} color={item.tone} size="2" variant="surface">
      <Flex align="center" gap="3" wrap="wrap">
        <span className="overview-dot" aria-hidden />
        <Text size="2" style={{ flex: '1 1 320px' }}><strong>{item.title}</strong>{item.detail && ` · ${item.detail}`}</Text>
        {'href' in item.action
          ? <Button className="overview-attention-action" size="2" variant="soft" color={item.tone} asChild><a href={item.action.href}>{item.action.label}</a></Button>
          : <ResetAction group={item.action.onReset} label={item.action.label} tone={item.tone} available={available} onReset={onReset} />}
      </Flex>
    </Callout.Root>)}
  </section></Flex>
}
