import { Button, Card, Dialog, Flex, Grid, Heading, Select, Switch, Table, Text, TextArea, TextField, VisuallyHidden } from '@radix-ui/themes'
import { useState } from 'react'
import type { ReactNode } from 'react'
import { useBanReset } from './useBanReset'
import { channelInput, deleteMetadataProxy, getMetadataProxies, importMetadataProxies, proxyFailure, proxyState, saveMetadataProxy, setMetadataProxies } from './metadataProxies'
import type { MetadataProxyChannel, MetadataProxyImportInput, MetadataProxyImportResult, MetadataProxyInput, MetadataProxyStatus } from './metadataProxies'
import type { CollectorPageProps } from './pageProps'
import ResourceStatus from './ResourceStatus'
import RowActions from './RowActions'
import Stat from './Stat'
import StatusBadge from './StatusBadge'
import type { Tone } from './StatusBadge'
import TableCard from './TableCard'
import { useCollectorResource } from './useCollectorResource'
import { useMutation } from './useMutation'
import './MetadataProxies.css'
import { formatDate } from './formatDate'

interface Draft extends MetadataProxyInput {
  id: string
  password: string
  hasPassword: boolean
  clearPassword: boolean
}

const tones: Record<MetadataProxyChannel['state'], Tone> = {
  disabled: 'gray', idle: 'green', running: 'blue', waiting_retry: 'gray', waiting_cooldown: 'amber',
  authentication_required: 'red', updating: 'blue', stopping: 'blue', removing: 'gray',
}

function stateDetail(channel: MetadataProxyChannel): ReactNode {
  const failure = channel.last_error && proxyFailure(channel.last_error)
  const parts: ReactNode[] = []
  if (channel.state === 'waiting_cooldown' && channel.ban_until) parts.push(`Until ${formatDate(channel.ban_until)}`)
  else if (channel.batch_size > 0) parts.push(channel.state === 'running' ? `${channel.batch_size} galleries in active batch` : `Finishing ${channel.batch_size} galleries in active batch`)
  if (channel.state === 'waiting_retry' && channel.retry_at) parts.push(`Retry after ${formatDate(channel.retry_at)}`)
  if (failure && channel.state !== 'waiting_cooldown') parts.push(channel.state === 'authentication_required' ? <Text key="failure" color="red">{failure}</Text> : failure)
  if (parts.length === 0) return undefined
  return parts.flatMap((part, index) => index === 0 ? [part] : [' · ', part])
}

export default function MetadataProxies({ available, refreshKey, interval, refreshSummary }: CollectorPageProps) {
  const resource = useCollectorResource(getMetadataProxies, available, refreshKey, interval)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [importDraft, setImportDraft] = useState<MetadataProxyImportInput | null>(null)
  const mutation = useMutation()
  const { pending, error } = mutation
  const disabled = !available || resource.stale || pending || !resource.data
  const banReset = useBanReset('proxies', available && !pending, () => { mutation.setNotice('Panda cooldowns reset.'); resource.refresh() })
  const dialogOpen = draft !== null || importDraft !== null

  async function change(action: (signal: AbortSignal) => Promise<MetadataProxyStatus | MetadataProxyImportResult>, message = '', settings = false) {
    if (disabled) return false
    return mutation.run(action, {
      onSuccess: (result) => {
        if (settings) refreshSummary?.()
        if ('status' in result) {
          resource.update(result.status)
          return `Added ${result.added} ${result.added === 1 ? 'channel' : 'channels'}. Skipped ${result.duplicates} ${result.duplicates === 1 ? 'duplicate' : 'duplicates'}.`
        }
        resource.update(result)
        return message
      },
    })
  }

  function edit(channel?: MetadataProxyChannel) {
    setDraft(channel
      ? { ...channelInput(channel), id: channel.id, password: '', hasPassword: channel.has_password, clearPassword: false }
      : { id: '', name: '', proxy_url: '', username: '', password: '', user_agent: resource.data?.default_user_agent ?? '', enabled: true, hasPassword: false, clearPassword: false })
    mutation.clear()
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
    if (await change((signal) => importMetadataProxies(importDraft, signal))) setImportDraft(null)
  }

  const data = resource.data
  const channels = data?.channels ?? []
  const collecting = channels.filter((channel) => channel.state === 'running' || channel.state === 'idle').length
  const cooling = channels.filter((channel) => channel.ban_until).length
  const attention = channels.filter((channel) => channel.state === 'authentication_required').length
  const dialogError = error && dialogOpen && <Text as="p" size="2" color="red" role="alert">{error}</Text>

  return <Flex direction="column" gap="4" asChild><section aria-labelledby="metadata-proxies-title">
    <VisuallyHidden><Heading as="h2" id="metadata-proxies-title">Proxy configuration</Heading></VisuallyHidden>
    <ResourceStatus name="Proxy channels" plural loaded={!!data} available={available} error={resource.error} stale={resource.stale} at={resource.at} />
    {mutation.notice && <Text as="p" size="2" color="green" role="status">{mutation.notice}</Text>}
    {error && !dialogOpen && <Text as="p" size="2" color="red" role="alert">{error}</Text>}
    {data && <>
      <div className="metadata-proxies-top">
        <Card size="2" className="metadata-proxies-settings"><Flex direction="column" gap="4" asChild><section aria-labelledby="proxy-settings-title">
          <Flex direction="column" gap="1">
            <Heading as="h2" size="4" id="proxy-settings-title">Settings</Heading>
            <Text size="1" color="gray">Apply to this collector for all Tana clients.</Text>
          </Flex>
          <Flex align="start" gap="2">
              <Switch checked={data.enabled} disabled={disabled} onCheckedChange={(checked) => { void change((signal) => setMetadataProxies({ enabled: checked, auto_remove_inactive: data.auto_remove_inactive }, signal), 'Proxy collection settings saved.', true) }} id="proxy-enabled" />
            <Flex direction="column" gap="1">
              <Text as="label" htmlFor="proxy-enabled" size="2" weight="medium">Proxy metadata collection</Text>
              <Text size="1" color="gray">Enabled channels resume after a restart. Turning this off finishes the active batch first.</Text>
            </Flex>
          </Flex>
          <Flex align="start" gap="2">
              <Switch checked={data.auto_remove_inactive} disabled={disabled} onCheckedChange={(checked) => { void change((signal) => setMetadataProxies({ enabled: data.enabled, auto_remove_inactive: checked }, signal), 'Proxy cleanup settings saved.', true) }} id="proxy-cleanup" />
            <Flex direction="column" gap="1">
              <Text as="label" htmlFor="proxy-cleanup" size="2" weight="medium">Remove proxies after 5 minutes without a successful request</Text>
              <Text size="1" color="gray">Includes idle and disabled channels. New channels get 5 minutes to succeed. Active batches finish before removal.</Text>
            </Flex>
          </Flex>
        </section></Flex></Card>
        <Card size="2" className="metadata-proxies-status"><Flex direction="column" gap="4" height="100%" asChild><section aria-labelledby="proxy-summary-title">
          <Flex direction="column" gap="1">
            <Heading as="h2" size="4" id="proxy-summary-title">Channel status</Heading>
            <Text size="1" color="gray">Up to 25 galleries per batch · one batch per channel every {data.rate_interval_ms / 1000} s.</Text>
          </Flex>
          <Grid columns="3" gap="4" asChild><dl style={{ margin: 0 }}>
            <Stat label="Collecting" value={collecting} color="green" />
            <Stat label="In cooldown" value={cooling} color="amber" />
            <Stat label="Need attention" value={attention} color="red" />
          </dl></Grid>
          {cooling > 0 && <div className="metadata-proxies-cooldown" style={{ marginTop: 'auto' }}>
            <Text size="1" color="amber">{cooling} {cooling === 1 ? 'channel' : 'channels'} in Panda cooldown</Text>
            <Button size="2" variant="soft" color="amber" style={{ width: 136 }} disabled={disabled || banReset.pending}
              onClick={banReset.reset}>{banReset.pending ? 'Resetting…' : 'Reset cooldowns'}</Button>
            {banReset.error && <Text as="p" size="1" color="red" role="alert">{banReset.error}</Text>}
          </div>}
        </section></Flex></Card>
      </div>

      <TableCard title={`Proxy channels · ${channels.length}`} titleId="proxy-channels-title" caption="Proxy channels" collectorPage="proxies"
        aside="Channels must pass an IP leak check before collecting. Failed checks remove the proxy; checks repeat hourly."
        actions={<>
          <Button size="2" variant="soft" color="gray" disabled={disabled || dialogOpen} onClick={() => { setImportDraft({ proxies: '', protocol: 'socks5', enabled: true }); mutation.clear() }}>Paste proxy list</Button>
          <Button size="2" disabled={disabled || dialogOpen} onClick={() => edit()}>Add channel</Button>
        </>}
        columns={[72, undefined, '34%', 150, 146]} minWidth={900}
        empty={channels.length === 0 && <Text as="p" size="2" role="status">No proxy channels configured. Paste a proxy list or add a channel to get started.</Text>}>
        <Table.Header><Table.Row>
          <Table.ColumnHeaderCell scope="col">On</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">Channel</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">State</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">Last success</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col" justify="end"><VisuallyHidden>Actions</VisuallyHidden></Table.ColumnHeaderCell>
        </Table.Row></Table.Header>
        <Table.Body>{channels.map((channel) => {
          const removing = channel.state === 'removing'
          return <Table.Row key={channel.id} className={channel.state === 'authentication_required' ? 'collector-row-failed' : undefined}>
            <Table.Cell><Switch aria-label={`Enable ${channel.name}`} checked={channel.enabled} disabled={disabled || removing}
              onCheckedChange={(checked) => { void change((signal) => saveMetadataProxy(channel.id, { ...channelInput(channel), enabled: checked }, signal), 'Channel settings saved.') }} /></Table.Cell>
            <Table.RowHeaderCell scope="row">
              {channel.name}
              <Text as="div" size="1" color="gray" weight="regular" className="metadata-proxies-url" title={channel.proxy_url}>{channel.proxy_url}</Text>
            </Table.RowHeaderCell>
            <Table.Cell><StatusBadge tone={tones[channel.state]} detail={stateDetail(channel)}>{proxyState(channel.state)}</StatusBadge></Table.Cell>
            <Table.Cell>{channel.last_success_at ? formatDate(channel.last_success_at) : 'No successes yet'}</Table.Cell>
            <Table.Cell><RowActions slot1Width={84} deleteSlot
              slot1={<Button size="2" variant="soft" color="gray" style={{ width: '100%' }} disabled={disabled || dialogOpen || removing} onClick={() => edit(channel)}>Edit</Button>}
              onDelete={removing ? undefined : () => { void change((signal) => deleteMetadataProxy(channel.id, signal), 'Channel removal requested. Any active batch will finish first.') }}
              deleteLabel={`Remove ${channel.name}`} deleteDisabled={disabled || dialogOpen} /></Table.Cell>
          </Table.Row>
        })}</Table.Body>
      </TableCard>
    </>}

    <Dialog.Root open={importDraft !== null} onOpenChange={(open) => { if (!open && !pending) { setImportDraft(null); mutation.clear() } }}>
      <Dialog.Content maxWidth="560px">
        {importDraft && <form onSubmit={(event) => { event.preventDefault(); void importList() }}><Flex direction="column" gap="3">
          <Dialog.Title mb="0">Paste proxy list</Dialog.Title>
          <Dialog.Description size="1" color="gray">Add several channels at once. Existing proxy addresses are skipped.</Dialog.Description>
          <Grid gap="3" asChild><fieldset disabled={disabled}>
            <Flex direction="column" gap="2" asChild><label>Proxy type
              <Select.Root value={importDraft.protocol} onValueChange={(value) => setImportDraft({ ...importDraft, protocol: value as MetadataProxyImportInput['protocol'] })}><Select.Trigger aria-label="Proxy type" /><Select.Content>
                <Select.Item value="http">HTTP</Select.Item>
                <Select.Item value="https">HTTPS</Select.Item>
                <Select.Item value="socks5">SOCKS5</Select.Item>
              </Select.Content></Select.Root>
              <Text size="1" color="gray">Applies to every address. SOCKS4 is not supported.</Text>
            </label></Flex>
            <Flex direction="column" gap="2" asChild><label>Proxy addresses
              <TextArea size="2" required rows={7} spellCheck={false} placeholder={'192.0.2.10:1080\n192.0.2.11:1080'} value={importDraft.proxies} onChange={(event) => setImportDraft({ ...importDraft, proxies: event.target.value })} />
              <Text size="1" color="gray">One host:port per line, without a URL scheme.</Text>
            </label></Flex>
            <Text as="label" size="2"><Flex align="center" gap="2"><Switch checked={importDraft.enabled} onCheckedChange={(checked) => setImportDraft({ ...importDraft, enabled: checked })} />Enable imported channels</Flex></Text>
            {!data?.enabled && <Text as="p" size="1" color="gray">Channels can run once proxy metadata collection is enabled.</Text>}
          </fieldset></Grid>
          {dialogError}
          <Flex justify="end" gap="2">
            <Button size="2" variant="soft" color="gray" type="button" disabled={pending} onClick={() => { setImportDraft(null); mutation.clear() }}>Cancel</Button>
            <Button size="2" type="submit" disabled={disabled}>{pending ? 'Importing…' : 'Import channels'}</Button>
          </Flex>
        </Flex></form>}
      </Dialog.Content>
    </Dialog.Root>

    <Dialog.Root open={draft !== null} onOpenChange={(open) => { if (!open && !pending) { setDraft(null); mutation.clear() } }}>
      <Dialog.Content maxWidth="560px" aria-describedby={undefined}>
        {draft && <form onSubmit={(event) => { event.preventDefault(); void save() }}><Flex direction="column" gap="3">
          <Dialog.Title mb="0">{draft.id ? 'Edit channel' : 'Add channel'}</Dialog.Title>
          <Grid gap="3" asChild><fieldset disabled={disabled}>
            <Grid columns={{ initial: '1', sm: '2' }} gap="3">
              <Flex direction="column" gap="2" asChild><label>Channel name
                <TextField.Root size="2" required maxLength={120} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
              </label></Flex>
              <Flex direction="column" gap="2" asChild><label>Proxy URL
                <TextField.Root size="2" required type="url" placeholder="http://proxy.example:8080" autoComplete="off" value={draft.proxy_url} onChange={(event) => setDraft({ ...draft, proxy_url: event.target.value })} />
                <Text size="1" color="gray">HTTP, HTTPS or SOCKS5 address. Enter credentials below.</Text>
              </label></Flex>
              <Flex direction="column" gap="2" asChild><label>Proxy username
                <TextField.Root size="2" maxLength={255} autoComplete="off" value={draft.username} onChange={(event) => setDraft({ ...draft, username: event.target.value })} />
              </label></Flex>
              <Flex direction="column" gap="2" asChild><label>Proxy password
                <TextField.Root size="2" type="password" maxLength={255} autoComplete="new-password" disabled={draft.clearPassword} placeholder={draft.hasPassword ? 'Blank keeps password only if proxy URL is unchanged' : ''} value={draft.password} onChange={(event) => setDraft({ ...draft, password: event.target.value })} />
                {draft.hasPassword && <Text size="1" color="gray">A password is saved. It cannot be displayed.</Text>}
              </label></Flex>
              <Flex direction="column" gap="2" style={{ gridColumn: '1 / -1' }} asChild><label>User-agent
                <TextField.Root size="2" required maxLength={1024} autoComplete="off" value={draft.user_agent} onChange={(event) => setDraft({ ...draft, user_agent: event.target.value })} />
              </label></Flex>
            </Grid>
            {draft.hasPassword && <Text as="label" size="2"><Flex align="center" gap="2"><Switch checked={draft.clearPassword} onCheckedChange={(checked) => setDraft({ ...draft, clearPassword: checked })} />Clear saved password</Flex></Text>}
            <Text as="label" size="2"><Flex align="center" gap="2"><Switch checked={draft.enabled} onCheckedChange={(checked) => setDraft({ ...draft, enabled: checked })} />Enable this channel</Flex></Text>
          </fieldset></Grid>
          {dialogError}
          <Flex justify="end" gap="2">
            <Button size="2" variant="soft" color="gray" type="button" disabled={pending} onClick={() => { setDraft(null); mutation.clear() }}>Cancel</Button>
            <Button size="2" type="submit" disabled={disabled}>{pending ? 'Saving…' : 'Save channel'}</Button>
          </Flex>
        </Flex></form>}
      </Dialog.Content>
    </Dialog.Root>
  </section></Flex>
}
