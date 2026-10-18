import { Badge, Text } from '@radix-ui/themes'
import type { ReactNode } from 'react'

export type Tone = 'green' | 'blue' | 'amber' | 'red' | 'gray'

export default function StatusBadge({ tone, children, detail }: {
  tone: Tone, children: string, detail?: ReactNode,
}) {
  return <>
    <Badge size="1" color={tone}>{children}</Badge>
    {detail && <Text as="div" size="1" color="gray" mt="1">{detail}</Text>}
  </>
}
