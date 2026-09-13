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

const base = '/api/collector/metadata/proxies'
const messages = {
  ...collectorMessages,
  proxy_invalid_configuration: 'Check the channel name, proxy URL, credentials and user-agent. Supported proxies: HTTP, HTTPS and SOCKS5.',
  proxy_duplicate: 'A channel already uses this proxy address and port, including channels being removed.',
  proxy_not_found: 'This channel was removed. Refresh the proxy list.',
  proxy_changing: 'This channel is being removed. Wait for its active batch to finish.',
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

export function proxyState(state: MetadataProxyChannel['state']) {
  return {
    disabled: 'Disabled', idle: 'Ready', running: 'Fetching metadata', waiting_retry: 'Waiting to retry',
    waiting_cooldown: 'Panda cooldown', authentication_required: 'Credentials required',
    updating: 'Finishing batch before update', stopping: 'Finishing batch before stopping', removing: 'Removing channel',
  }[state]
}

export function proxyFailure(error: string) {
  if (error.startsWith('upstream_http_')) return `Upstream returned HTTP ${error.slice('upstream_http_'.length)}.`
  return {
    proxy_authentication_required: 'Proxy authentication failed. Update the URL or credentials to resume.',
    panda_banned: 'Panda has temporarily banned this proxy.',
    proxy_timeout: 'The proxy request timed out.',
    metadata_proxy_failed: 'Metadata retrieval failed. Check the proxy connection and collector storage.',
  }[error] ?? 'Metadata retrieval failed.'
}

export function channelInput(channel: MetadataProxyChannel): MetadataProxyInput {
  return { name: channel.name, proxy_url: channel.proxy_url, username: channel.username, user_agent: channel.user_agent, enabled: channel.enabled }
}
