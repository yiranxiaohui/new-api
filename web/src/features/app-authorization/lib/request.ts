/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
/**
 * Parameters of a native app authorization request (RFC 8252 loopback
 * redirect with PKCE). The server re-validates everything; these checks only
 * decide whether the consent page may be shown or redirect on cancel.
 */
export type AppAuthorizationSearch = {
  client_name?: string
  redirect_uri?: string
  code_challenge?: string
  code_challenge_method?: string
  state?: string
  key_name?: string
}

export type AppAuthorizationRequest = {
  clientName: string
  redirectUri: string
  redirectHost: string
  codeChallenge: string
  state: string
  keyName: string
}

export const APP_CLIENT_NAME_MAX_LENGTH = 64
export const APP_STATE_MAX_LENGTH = 512
/** The server limits API key names to 50 UTF-8 bytes. */
export const API_KEY_NAME_MAX_BYTES = 50
const REDIRECT_MAX_LENGTH = 512
const LOOPBACK_HOSTS = new Set(['127.0.0.1', '[::1]'])
const CONTROL_OR_FORMAT = /[\p{Cc}\p{Cf}]/u
const S256_CHALLENGE = /^[A-Za-z0-9_-]{43}$/

/**
 * Accept only loopback redirects without user info, query or fragment, the
 * same rule the server enforces.
 */
export function parseLoopbackRedirect(value: unknown): URL | null {
  if (typeof value !== 'string' || !value || value.length > REDIRECT_MAX_LENGTH) {
    return null
  }
  if (value.includes('#') || value.includes('?')) return null
  let url: URL
  try {
    url = new URL(value)
  } catch {
    return null
  }
  if (url.protocol !== 'http:' || url.username || url.password) return null
  if (!LOOPBACK_HOSTS.has(url.hostname) || !url.port) return null
  const port = Number(url.port)
  if (!Number.isInteger(port) || port < 1 || port > 65535) return null
  return url
}

const encoder = new TextEncoder()

export function utf8ByteLength(value: string): number {
  return encoder.encode(value).length
}

function truncateUtf8(value: string, maxBytes: number): string {
  let result = ''
  for (const char of value) {
    if (utf8ByteLength(result + char) > maxBytes) break
    result += char
  }
  return result
}

function normalizeClientName(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const name = value.trim()
  if (!name || [...name].length > APP_CLIENT_NAME_MAX_LENGTH) return null
  if (CONTROL_OR_FORMAT.test(name)) return null
  return name
}

/** Validate the query string of `/app-auth`, or return null when it is unusable. */
export function parseAppAuthorizationRequest(
  search: AppAuthorizationSearch
): AppAuthorizationRequest | null {
  const clientName = normalizeClientName(search.client_name)
  const redirect = parseLoopbackRedirect(search.redirect_uri)
  const state = search.state ?? ''
  if (!clientName || !redirect || typeof search.redirect_uri !== 'string') {
    return null
  }
  if (search.code_challenge_method !== 'S256') return null
  if (!search.code_challenge || !S256_CHALLENGE.test(search.code_challenge)) {
    return null
  }
  if (typeof state !== 'string' || state.length > APP_STATE_MAX_LENGTH) {
    return null
  }

  let keyName =
    typeof search.key_name === 'string' ? search.key_name.trim() : ''
  if (!keyName || CONTROL_OR_FORMAT.test(keyName)) keyName = clientName
  keyName = truncateUtf8(keyName, API_KEY_NAME_MAX_BYTES).trim()

  return {
    clientName,
    redirectUri: search.redirect_uri,
    redirectHost: redirect.host,
    codeChallenge: search.code_challenge,
    state,
    keyName,
  }
}

/** Where to send the browser when the user declines (RFC 6749 access_denied). */
export function buildDeniedRedirect(request: AppAuthorizationRequest): string {
  const url = new URL(request.redirectUri)
  url.searchParams.set('error', 'access_denied')
  if (request.state) url.searchParams.set('state', request.state)
  return url.toString()
}
