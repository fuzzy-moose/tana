import { Text } from '@radix-ui/themes'

/** A labelled count; render inside a `<dl>`. */
export default function Stat({ label, value, color }: { label: string, value: number, color?: 'green' | 'amber' | 'red' }) {
  return <div><Text size="1" color="gray" asChild><dt>{label}</dt></Text><Text size="6" weight="medium" color={color} asChild><dd style={{ margin: 0 }}>{value.toLocaleString()}</dd></Text></div>
}
