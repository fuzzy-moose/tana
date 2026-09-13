import { request } from '../api'
import { collectorMessages } from './api'

export interface MetadataProxyInput {
  name: string
  proxy_url: string
  username: string
  password?: string
  user_agent: string
  enabled: boolean
}

export interface MetadataProxyChannel extends Omit<MetadataProxyInput, 'password'> {
  id: string
  has_password: boolean
  state: 'disabled' | 'idle' | 'running' | 'waiting_retry' | 'waiting_cooldown' | 'authentication_required' | 'updating' | 'stopping' | 'removing'
  batch_size: number
  ban_until?: string
  retry_at?: string
  last_success_at?: string
  last_error?: string
}

export interface MetadataProxyStatus {
  enabled: boolean
  channels: MetadataProxyChannel[]
  rate_interval_ms: number
  default_user_agent: string
}

export interface MetadataProxyImportInput {
  proxies: string
  protocol: 'http' | 'https' | 'socks5'
  enabled: boolean
}

export interface MetadataProxyImportResult {
  status: MetadataProxyStatus
  added: number
  duplicates: number
}

const base = '/api/collector/metadata/proxies'
const messages = {
  ...collectorMessages,
  proxy_invalid_configuration: 'Check the channel name, proxy URL, credentials and user-agent. Supported proxies: HTTP, HTTPS and SOCKS5.',
  proxy_duplicate: 'A channel already uses this proxy address and port, including channels being removed.',
  proxy_not_found: 'This channel was removed. Refresh the proxy list.',
  proxy_changing: 'This channel is being removed. Wait for its active batch to finish.',
  proxy_list_invalid: 'Enter one host:port per line, without a URL scheme. Select HTTP, HTTPS or SOCKS5.',
  proxy_list_too_large: 'The proxy list exceeds the limit of 1 MiB or 10,000 entries.',
}

export const getMetadataProxies = (signal: AbortSignal) => request<MetadataProxyStatus>(base, { signal }, messages)

function mutate(path: string, method: string, input: unknown, signal: AbortSignal) {
  return request<MetadataProxyStatus>(base + path, {
    method, signal, headers: { 'Content-Type': 'application/json' },
    ...(input !== undefined ? { body: JSON.stringify(input) } : {}),
  }, messages)
}

export const setMetadataProxies = (enabled: boolean, signal: AbortSignal) => mutate('', 'PUT', { enabled }, signal)
export const saveMetadataProxy = (id: string, input: MetadataProxyInput, signal: AbortSignal) => mutate(
  '/channels' + (id ? '/' + encodeURIComponent(id) : ''), id ? 'PUT' : 'POST', input, signal,
)
export const deleteMetadataProxy = (id: string, signal: AbortSignal) => mutate('/channels/' + encodeURIComponent(id), 'DELETE', undefined, signal)
export const importMetadataProxies = (input: MetadataProxyImportInput, signal: AbortSignal) => request<MetadataProxyImportResult>(base + '/import', {
  method: 'POST', signal, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
}, messages)

export function proxyState(state: MetadataProxyChannel['state']) {
  return {
    disabled: 'Disabled', idle: 'Ready', running: 'Fetching metadata', waiting_retry: 'Waiting to retry',
    waiting_cooldown: 'Panda cooldown', authentication_required: 'Credentials required',
    updating: 'Finishing batch before update', stopping: 'Finishing batch before stopping', removing: 'Removing channel',
  }[state]
}

export function proxyFailure(error: string) {
  if (error.startsWith('upstream_http_')) return `Upstream returned HTTP ${error.slice('upstream_http_'.length)}.`
  if (error.startsWith('proxy_http_')) return `The proxy rejected the connection tunnel with HTTP ${error.slice('proxy_http_'.length)}. Check its access rules and HTTPS support.`
  return {
    proxy_authentication_required: 'Proxy authentication failed. Update the URL or credentials to resume.',
    panda_banned: 'Panda has temporarily banned this proxy.',
    proxy_timeout: 'The proxy request timed out.',
    proxy_dns_failed: 'A hostname on the proxy route could not be resolved. Check the proxy address and collector DNS.',
    proxy_connection_refused: 'A connection on the proxy route was refused. Check the proxy address, port and destination access.',
    proxy_connection_closed: 'The connection closed before metadata retrieval finished. The proxy or upstream may have dropped it.',
    proxy_connection_rejected: 'The SOCKS proxy rejected the connection under its access rules.',
    proxy_unreachable: 'The proxy route could not reach its destination. Check network access from the collector and proxy.',
    proxy_connection_failed: 'The proxy route failed to connect or transfer data. Check proxy availability and network access.',
    proxy_tls_failed: 'TLS negotiation or certificate verification failed on the proxy route. Check certificates and HTTPS support.',
    proxy_protocol_error: 'The proxy route returned an unexpected TLS response. Check whether the proxy requires HTTP, HTTPS or SOCKS5.',
    proxy_socks_failed: 'SOCKS negotiation failed. Check that the address and port support SOCKS5 and allow the destination.',
    metadata_invalid_response: 'The proxy route returned invalid or incomplete Panda metadata. A proxy error page or incompatible response may be the cause.',
    metadata_api_error: 'The Panda API reported an error while retrieving metadata through this proxy.',
    collector_storage_busy: 'The collector database is busy or locked. Metadata collection will retry.',
    collector_storage_full: 'Collector storage is full. Free disk space on the collector.',
    collector_storage_readonly: 'The collector database is read-only. Check its storage permissions and mount settings.',
    collector_storage_failed: 'The collector could not read or save metadata in its database. Check collector storage.',
    metadata_proxy_failed: 'Metadata retrieval failed; no further details are available.',
  }[error] ?? 'Metadata retrieval failed.'
}

export function channelInput(channel: MetadataProxyChannel): MetadataProxyInput {
  return { name: channel.name, proxy_url: channel.proxy_url, username: channel.username, user_agent: channel.user_agent, enabled: channel.enabled }
}
