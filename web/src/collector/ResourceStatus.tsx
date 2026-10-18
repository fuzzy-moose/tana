import { Text } from '@radix-ui/themes'
import { formatDate } from './formatDate'
export default function ResourceStatus({ name, plural = false, loaded, available, error, stale, at }: {
  name: string, plural?: boolean, loaded: boolean, available: boolean, error: string, stale: boolean, at?: string,
}) {
  const verb = plural ? 'are' : 'is'
  return <>
    {error && <Text as="p" size="2" color="red" role="alert">{error}</Text>}
    {stale && <Text as="p" size="2" color="amber" role="status">{name} {verb} stale.{at && ` Last updated ${formatDate(at)}.`}</Text>}
    {!loaded && !error && <Text as="p" size="1" color="gray">{available ? `Loading ${name.toLowerCase()}…` : `${name} ${verb} unavailable while the collector is disconnected.`}</Text>}
  </>
}
