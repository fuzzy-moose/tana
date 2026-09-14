import { Badge, Button, Card, Checkbox, Flex, Grid, Heading, Select, Text, TextArea, TextField } from '@radix-ui/themes'
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

  return <Flex direction="column" gap="4" asChild><section aria-labelledby="metadata-proxies-title">
    <Flex align="center" justify="between" wrap="wrap" gap="3">
      <Heading as="h2" size="4" id="metadata-proxies-title">Channels</Heading>
      <Flex align="center" wrap="wrap" gap="2">
        <Button size="2" variant="soft" disabled={disabled || draft !== null || importDraft !== null} onClick={() => {
          setImportDraft({ proxies: '', protocol: 'socks5', enabled: true })
          setError('')
          setNotice('')
        }} color="gray">Paste proxy list</Button>
        <Button size="2" variant="soft" disabled={disabled || draft !== null || importDraft !== null} onClick={() => edit()} color="gray">Add channel</Button>
      </Flex>
    </Flex>
    <Text as="p" size="2">Use fixed proxies to collect background metadata and validate reference imports. Each channel has its own pacing and Panda cooldown.</Text>
    <Text as="p" size="1" color="gray">Channels must pass an IP leak check before collecting. Failed checks remove the proxy; checks repeat hourly.</Text>
    <ResourceStatus name="Proxy channels" loaded={!!resource.data} available={available} error={resource.error} stale={resource.stale} at={resource.at} />
    {resource.data && <>
      <Flex align="center" gap="2" asChild><label>
        <Checkbox checked={resource.data.enabled} disabled={disabled} onCheckedChange={(checked) => { void change((signal) => setMetadataProxies({ enabled: (checked === true), auto_remove_inactive: resource.data!.auto_remove_inactive }, signal), 'Proxy collection settings saved.') }} />
        Enable proxy metadata collection
      </label></Flex>
      <Text as="p" size="1" color="gray">Settings apply to this collector for all Tana clients. Enabled channels resume after a restart. Disabling finishes the active batch first.</Text>
      <Flex align="center" gap="2" asChild><label>
        <Checkbox checked={resource.data.auto_remove_inactive} disabled={disabled} onCheckedChange={(checked) => { void change((signal) => setMetadataProxies({ enabled: resource.data!.enabled, auto_remove_inactive: (checked === true) }, signal), 'Proxy cleanup settings saved.') }} />
        Remove proxies after 5 minutes without a successful request
      </label></Flex>
      <Text as="p" size="1" color="gray">Includes idle and disabled channels. New channels get 5 minutes to succeed. Active batches finish before removal.</Text>
      <Text as="p" size="1" color="gray">Up to 25 galleries per batch · one batch per channel every {resource.data.rate_interval_ms / 1000} seconds.</Text>
    </>}
    {notice && <Text as="p" size="2" color="green" role="status">{notice}</Text>}
    {error && <Text as="p" size="2" color="red" role="alert">{error}</Text>}

    {importDraft && <Card><form onSubmit={(event) => { event.preventDefault(); void importList() }}><Flex direction="column" gap="3">
      <Heading as="h3" size="3">Paste proxy list</Heading>
      <Text as="p" size="1" color="gray">Add several channels at once. Existing proxy addresses are skipped.</Text>
      <Grid gap="3" asChild><fieldset disabled={disabled}>
        <Grid columns={{ initial: '1', sm: '2' }} gap="4">
          <Flex direction="column" gap="2" flexGrow="1" asChild><label>Proxy type
            <Select.Root value={importDraft.protocol} onValueChange={(value) => setImportDraft({ ...importDraft, protocol: value as MetadataProxyImportInput['protocol'] })}><Select.Trigger aria-label="Proxy type" /><Select.Content>
              <Select.Item value="http">HTTP</Select.Item>
              <Select.Item value="https">HTTPS</Select.Item>
              <Select.Item value="socks5">SOCKS5</Select.Item>
            </Select.Content></Select.Root>
            <Text size="1" color="gray">Applies to every address. SOCKS4 is not supported.</Text>
          </label></Flex>
          <Flex direction="column" gap="2" flexGrow="1" style={{ gridColumn: '1 / -1' }} asChild><label>Proxy addresses
            <TextArea size="2" required rows={7} spellCheck={false} placeholder={'192.0.2.10:1080\n192.0.2.11:1080'} value={importDraft.proxies} onChange={(event) => setImportDraft({ ...importDraft, proxies: event.target.value })} />
            <Text size="1" color="gray">One host:port per line, without a URL scheme.</Text>
          </label></Flex>
        </Grid>
        <Flex align="center" gap="2" asChild><label><Checkbox checked={importDraft.enabled} onCheckedChange={(checked) => setImportDraft({ ...importDraft, enabled: (checked === true) })} />Enable imported channels</label></Flex>
        {!resource.data?.enabled && <Text as="p" size="1" color="gray">Channels can run once proxy metadata collection is enabled.</Text>}
        <Flex align="center" wrap="wrap" gap="2">
          <Button size="2" variant="solid" type="submit">{pending ? 'Importing…' : 'Import channels'}</Button>
          <Button size="2" variant="soft" type="button" onClick={() => setImportDraft(null)} color="gray">Cancel</Button>
        </Flex>
      </fieldset></Grid>
    </Flex></form></Card>}

    {draft && <Card><form onSubmit={(event) => { event.preventDefault(); void save() }}><Flex direction="column" gap="3">
      <Heading as="h3" size="3">{draft.id ? 'Edit channel' : 'New channel'}</Heading>
      <Grid gap="3" asChild><fieldset disabled={disabled}>
        <Grid columns={{ initial: '1', sm: '2' }} gap="4">
          <Flex direction="column" gap="2" flexGrow="1" asChild><label>Channel name
            <TextField.Root size="2" required maxLength={120} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
          </label></Flex>
          <Flex direction="column" gap="2" flexGrow="1" asChild><label>Proxy URL
            <TextField.Root size="2" required type="url" placeholder="http://proxy.example:8080" autoComplete="off" value={draft.proxy_url} onChange={(event) => setDraft({ ...draft, proxy_url: event.target.value })} />
            <Text size="1" color="gray">HTTP, HTTPS or SOCKS5 address. Enter credentials below.</Text>
          </label></Flex>
          <Flex direction="column" gap="2" flexGrow="1" asChild><label>Proxy username
            <TextField.Root size="2" maxLength={255} autoComplete="off" value={draft.username} onChange={(event) => setDraft({ ...draft, username: event.target.value })} />
          </label></Flex>
          <Flex direction="column" gap="2" flexGrow="1" asChild><label>Proxy password
            <TextField.Root size="2" type="password" maxLength={255} autoComplete="new-password" disabled={draft.clearPassword} placeholder={draft.hasPassword ? 'Blank keeps password only if proxy URL is unchanged' : ''} value={draft.password} onChange={(event) => setDraft({ ...draft, password: event.target.value })} />
            {draft.hasPassword && <Text size="1" color="gray">A password is saved. It cannot be displayed.</Text>}
          </label></Flex>
          <Flex direction="column" gap="2" flexGrow="1" style={{ gridColumn: '1 / -1' }} asChild><label>User-agent
            <TextField.Root size="2" required maxLength={1024} autoComplete="off" value={draft.user_agent} onChange={(event) => setDraft({ ...draft, user_agent: event.target.value })} />
          </label></Flex>
        </Grid>
        {draft.hasPassword && <Flex align="center" gap="2" asChild><label><Checkbox checked={draft.clearPassword} onCheckedChange={(checked) => setDraft({ ...draft, clearPassword: (checked === true) })} />Clear saved password</label></Flex>}
        <Flex align="center" gap="2" asChild><label><Checkbox checked={draft.enabled} onCheckedChange={(checked) => setDraft({ ...draft, enabled: (checked === true) })} />Enable this channel</label></Flex>
        <Flex align="center" wrap="wrap" gap="2">
          <Button size="2" variant="solid" type="submit">{pending ? 'Saving…' : 'Save channel'}</Button>
          <Button size="2" variant="soft" type="button" onClick={() => setDraft(null)} color="gray">Cancel</Button>
        </Flex>
      </fieldset></Grid>
    </Flex></form></Card>}

    {resource.data?.channels.length === 0 && !draft && !importDraft && <Text as="p" size="2">No proxy channels configured. Paste a proxy list or add a channel to get started.</Text>}
    <Flex direction="column" gap="3">
      {resource.data?.channels.map((channel) => <Card key={channel.id} asChild><article aria-label={channel.name}><Flex direction="column" gap="3">
        <Flex align="center" justify="between" wrap="wrap" gap="3">
          <Heading as="h3" size="3">{channel.name}</Heading>
          <Badge color="gray">{proxyState(channel.state)}</Badge>
        </Flex>
        <Text as="p" size="2">{channel.proxy_url}</Text>
        <Flex wrap="wrap" gap="3">
          {channel.batch_size > 0 && <span>{channel.batch_size} galleries in active batch</span>}
          <span>{channel.last_success_at ? `Last success ${date(channel.last_success_at)}` : 'No successful batches yet'}</span>
          {channel.ban_until && <span>Panda cooldown until {date(channel.ban_until)}</span>}
          {channel.retry_at && <span>Retry after {date(channel.retry_at)}</span>}
          {channel.last_error && <span>Last error: {proxyFailure(channel.last_error)}</span>}
        </Flex>
        <Flex align="center" justify="between" wrap="wrap" gap="3">
          <Flex align="center" gap="2" asChild><label>
            <Checkbox checked={channel.enabled} disabled={disabled || channel.state === 'removing'} onCheckedChange={(checked) => { void change((signal) => saveMetadataProxy(channel.id, { ...channelInput(channel), enabled: (checked === true) }, signal), 'Channel settings saved.') }} />
            Enable {channel.name}
          </label></Flex>
          <Flex align="center" wrap="wrap" gap="2">
            <Button size="2" variant="soft" disabled={disabled || draft !== null || importDraft !== null || channel.state === 'removing'} onClick={() => edit(channel)} color="gray">Edit</Button>
            <Button size="2" variant="soft" disabled={disabled || draft !== null || importDraft !== null || channel.state === 'removing'} onClick={() => { void change((signal) => deleteMetadataProxy(channel.id, signal), 'Channel removal requested. Any active batch will finish first.') }} color="red">Remove</Button>
          </Flex>
        </Flex>
      </Flex></article></Card>)}
    </Flex>
  </section></Flex>
}
