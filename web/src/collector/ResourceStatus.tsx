export default function ResourceStatus({ name, loaded, available, error, stale, at }: {
  name: string, loaded: boolean, available: boolean, error: string, stale: boolean, at?: string,
}) {
  return <>
    {error && <p className="error-message" role="alert">{error}</p>}
    {stale && <p className="collector-stale" role="status">{name} are stale.{at && ` Last updated ${new Date(at).toLocaleString()}.`}</p>}
    {!loaded && !error && <p className="field-help">{available ? `Loading ${name.toLowerCase()}…` : `${name} are unavailable while the collector is disconnected.`}</p>}
  </>
}
