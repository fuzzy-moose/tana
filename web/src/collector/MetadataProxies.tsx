import { useEffect, useRef, useState } from 'react'
import { channelInput, deleteMetadataProxy, getMetadataProxies, importMetadataProxies, proxyFailure, proxyState, saveMetadataProxy, setMetadataProxies } from './metadataProxies'
import type { MetadataProxyChannel, MetadataProxyImportInput, MetadataProxyImportResult, MetadataProxyInput, MetadataProxyStatus } from './metadataProxies'
import { useCollectorResource } from './useCollectorResource'
import ResourceStatus from './ResourceStatus'

interface Draft extends MetadataProxyInput {
  id: string
  password: string
  hasPassword: boolean
  clearPassword: boolean
}

const date = (value: string) => new Date(value).toLocaleString()

export default function MetadataProxies({ available, refreshKey }: { available: boolean, refreshKey: number }) {
  const resource = useCollectorResource(getMetadataProxies, available, refreshKey)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [importDraft, setImportDraft] = useState<MetadataProxyImportInput | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const mutation = useRef<AbortController | null>(null)
  useEffect(() => () => mutation.current?.abort(), [])
  const disabled = !available || resource.stale || pending || !resource.data

  async function change(action: (signal: AbortSignal) => Promise<MetadataProxyStatus | MetadataProxyImportResult>, message = '') {
    if (disabled || mutation.current) return false
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setError('')
    setNotice('')
    try {
      const result = await action(controller.signal)
      if (controller.signal.aborted) return false
      if ('status' in result) {
        resource.update(result.status)
        setNotice(`Added ${result.added} ${result.added === 1 ? 'channel' : 'channels'}. Skipped ${result.duplicates} ${result.duplicates === 1 ? 'duplicate' : 'duplicates'}.`)
      } else {
        resource.update(result)
        setNotice(message)
      }
      return true
    } catch (error) {
      if (!controller.signal.aborted) setError((error as Error).message)
      return false
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) setPending(false)
    }
  }

  function edit(channel?: MetadataProxyChannel) {
    setDraft(channel
      ? { ...channelInput(channel), id: channel.id, password: '', hasPassword: channel.has_password, clearPassword: false }
      : { id: '', name: '', proxy_url: '', username: '', password: '', user_agent: resource.data?.default_user_agent ?? '', enabled: true, hasPassword: false, clearPassword: false })
    setError('')
    setNotice('')
  }

  async function save() {
    if (!draft) return
    const input: MetadataProxyInput = {
      name: draft.name, proxy_url: draft.proxy_url, username: draft.username, user_agent: draft.user_agent, enabled: draft.enabled,
    }
    if (draft.clearPassword) input.password = ''
    else if (draft.password || !draft.hasPassword) input.password = draft.password
    if (await change((signal) => saveMetadataProxy(draft.id, input, signal), 'Channel saved.')) setDraft(null)
  }

  async function importList() {
    if (!importDraft) return
    if (await change((signal) => importMetadataProxies(importDraft, signal))) {
      setImportDraft(null)
    }
  }

  return <section className="collector-panel" aria-labelledby="metadata-proxies-title">
    <div className="collector-section-heading">
      <h2 id="metadata-proxies-title">Metadata proxy channels</h2>
      <div className="button-group">
        <button className="button" disabled={disabled || draft !== null || importDraft !== null} onClick={() => {
          setImportDraft({ proxies: '', protocol: 'socks5', enabled: true })
          setError('')
          setNotice('')
        }}>Paste proxy list</button>
        <button className="button" disabled={disabled || draft !== null || importDraft !== null} onClick={() => edit()}>Add channel</button>
      </div>
    </div>
    <p>Use fixed proxies to collect background metadata and validate reference imports. Each channel has its own pacing and Panda cooldown.</p>
    <ResourceStatus name="Proxy channels" loaded={!!resource.data} available={available} error={resource.error} stale={resource.stale} at={resource.at} />
    {resource.data && <>
      <label className="proxy-toggle">
        <input type="checkbox" checked={resource.data.enabled} disabled={disabled}
          onChange={(event) => { void change((signal) => setMetadataProxies(event.target.checked, signal), 'Proxy collection settings saved.') }} />
        Enable proxy metadata collection
      </label>
      <p className="field-help">Settings apply to this collector for all Tana clients. Enabled channels resume after a restart. Disabling finishes the active batch first.</p>
      <p className="field-help">Up to 25 galleries per batch · one batch per channel every {resource.data.rate_interval_ms / 1000} seconds.</p>
    </>}
    {notice && <p className="collector-notice" role="status">{notice}</p>}
    {error && <p className="error-message" role="alert">{error}</p>}

    {importDraft && <form className="proxy-form" onSubmit={(event) => { event.preventDefault(); void importList() }}>
      <h3>Paste proxy list</h3>
      <p className="field-help">Add several channels at once. Existing proxy addresses are skipped.</p>
      <fieldset disabled={disabled}>
        <div className="proxy-form-fields">
          <label className="form-field">Proxy type
            <select className="text-input" value={importDraft.protocol} onChange={(event) => setImportDraft({ ...importDraft, protocol: event.target.value as MetadataProxyImportInput['protocol'] })}>
              <option value="http">HTTP</option>
              <option value="https">HTTPS</option>
              <option value="socks5">SOCKS5</option>
            </select>
            <span className="field-help">Applies to every address. SOCKS4 is not supported.</span>
          </label>
          <label className="form-field proxy-list-addresses">Proxy addresses
            <textarea className="text-input" required rows={7} spellCheck={false} placeholder={'192.0.2.10:1080\n192.0.2.11:1080'} value={importDraft.proxies}
              onChange={(event) => setImportDraft({ ...importDraft, proxies: event.target.value })} />
            <span className="field-help">One host:port per line, without a URL scheme.</span>
          </label>
        </div>
        <label className="proxy-toggle"><input type="checkbox" checked={importDraft.enabled} onChange={(event) => setImportDraft({ ...importDraft, enabled: event.target.checked })} />Enable imported channels</label>
        {!resource.data?.enabled && <p className="field-help">Channels can run once proxy metadata collection is enabled.</p>}
        <div className="button-group">
          <button className="button" type="submit">{pending ? 'Importing…' : 'Import channels'}</button>
          <button className="button" type="button" onClick={() => setImportDraft(null)}>Cancel</button>
        </div>
      </fieldset>
    </form>}

    {draft && <form className="proxy-form" onSubmit={(event) => { event.preventDefault(); void save() }}>
      <h3>{draft.id ? 'Edit channel' : 'New channel'}</h3>
      <fieldset disabled={disabled}>
        <div className="proxy-form-fields">
          <label className="form-field">Channel name
            <input className="text-input" required maxLength={120} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
          </label>
          <label className="form-field">Proxy URL
            <input className="text-input" required type="url" placeholder="http://proxy.example:8080" autoComplete="off" value={draft.proxy_url} onChange={(event) => setDraft({ ...draft, proxy_url: event.target.value })} />
            <span className="field-help">HTTP, HTTPS or SOCKS5 address. Enter credentials below.</span>
          </label>
          <label className="form-field">Proxy username
            <input className="text-input" maxLength={255} autoComplete="off" value={draft.username} onChange={(event) => setDraft({ ...draft, username: event.target.value })} />
          </label>
          <label className="form-field">Proxy password
            <input className="text-input" type="password" maxLength={255} autoComplete="new-password" disabled={draft.clearPassword}
              placeholder={draft.hasPassword ? 'Blank keeps password only if proxy URL is unchanged' : ''} value={draft.password} onChange={(event) => setDraft({ ...draft, password: event.target.value })} />
            {draft.hasPassword && <span className="field-help">A password is saved. It cannot be displayed.</span>}
          </label>
          <label className="form-field proxy-user-agent">User-agent
            <input className="text-input" required maxLength={1024} autoComplete="off" value={draft.user_agent} onChange={(event) => setDraft({ ...draft, user_agent: event.target.value })} />
          </label>
        </div>
        {draft.hasPassword && <label className="proxy-toggle"><input type="checkbox" checked={draft.clearPassword} onChange={(event) => setDraft({ ...draft, clearPassword: event.target.checked })} />Clear saved password</label>}
        <label className="proxy-toggle"><input type="checkbox" checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} />Enable this channel</label>
        <div className="button-group">
          <button className="button" type="submit">{pending ? 'Saving…' : 'Save channel'}</button>
          <button className="button" type="button" onClick={() => setDraft(null)}>Cancel</button>
        </div>
      </fieldset>
    </form>}

    {resource.data?.channels.length === 0 && !draft && !importDraft && <p className="download-empty">No proxy channels configured. Paste a proxy list or add a channel to get started.</p>}
    <div className="proxy-channels">
      {resource.data?.channels.map((channel) => <article className="proxy-channel" key={channel.id} aria-label={channel.name}>
        <div className="collector-section-heading">
          <h3>{channel.name}</h3>
          <span className="collector-badge">{proxyState(channel.state)}</span>
        </div>
        <p className="proxy-endpoint">{channel.proxy_url}</p>
        <div className="proxy-channel-details">
          {channel.batch_size > 0 && <span>{channel.batch_size} galleries in active batch</span>}
          <span>{channel.last_success_at ? `Last success ${date(channel.last_success_at)}` : 'No successful batches yet'}</span>
          {channel.ban_until && <span>Panda cooldown until {date(channel.ban_until)}</span>}
          {channel.retry_at && <span>Retry after {date(channel.retry_at)}</span>}
          {channel.last_error && <span>Last error: {proxyFailure(channel.last_error)}</span>}
        </div>
        <div className="proxy-channel-actions">
          <label className="proxy-toggle">
            <input type="checkbox" checked={channel.enabled} disabled={disabled || channel.state === 'removing'}
              onChange={(event) => { void change((signal) => saveMetadataProxy(channel.id, { ...channelInput(channel), enabled: event.target.checked }, signal), 'Channel settings saved.') }} />
            Enable {channel.name}
          </label>
          <div className="button-group">
            <button className="button" disabled={disabled || draft !== null || importDraft !== null || channel.state === 'removing'} onClick={() => edit(channel)}>Edit</button>
            <button className="button" disabled={disabled || draft !== null || importDraft !== null || channel.state === 'removing'} onClick={() => { void change((signal) => deleteMetadataProxy(channel.id, signal), 'Channel removal requested. Any active batch will finish first.') }}>Remove</button>
          </div>
        </div>
      </article>)}
    </div>
  </section>
}
