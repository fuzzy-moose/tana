import { Text } from '@radix-ui/themes'
export default function ResourceStatus({ name, loaded, available, error, stale, at }: {
  name: string, loaded: boolean, available: boolean, error: string, stale: boolean, at?: string,
}) {
  return <>
    {error && <Text as="p" size="2" color="red" role="alert">{error}</Text>}
    {stale && <Text as="p" size="2" color="green" role="status">{name} are stale.{at && ` Last updated ${new Date(at).toLocaleString()}.`}</Text>}
    {!loaded && !error && <Text as="p" size="1" color="gray">{available ? `Loading ${name.toLowerCase()}…` : `${name} are unavailable while the collector is disconnected.`}</Text>}
  </>
}
