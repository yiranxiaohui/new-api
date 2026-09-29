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
import { describe, expect, it } from 'vitest'

import {
  buildDeniedRedirect,
  parseAppAuthorizationRequest,
  parseLoopbackRedirect,
  utf8ByteLength,
} from '../request'

const challenge = 'E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM'
const valid = {
  client_name: 'Pier',
  redirect_uri: 'http://127.0.0.1:53127/newapi/callback',
  code_challenge: challenge,
  code_challenge_method: 'S256',
  state: 'abc',
  key_name: 'Pier · laptop',
}

describe('parseLoopbackRedirect', () => {
  it.each([
    'http://127.0.0.1:53127/callback',
    'http://[::1]:8080/cb',
    'http://127.0.0.1:1',
  ])('accepts %s', (value) => {
    expect(parseLoopbackRedirect(value)).not.toBeNull()
  })

  it.each([
    'https://127.0.0.1:53127/callback',
    'http://localhost:53127/callback',
    'http://127.0.0.1/callback',
    'http://127.0.0.1:0/callback',
    'http://127.0.0.1:53127/callback?next=https://attacker.example',
    'http://127.0.0.1:53127/callback#fragment',
    'http://user@127.0.0.1:53127/callback',
    'http://127.0.0.1.attacker.example:53127/callback',
    'javascript:alert(1)',
    '',
  ])('rejects %s', (value) => {
    expect(parseLoopbackRedirect(value)).toBeNull()
  })
})

describe('parseAppAuthorizationRequest', () => {
  it('returns the normalized request', () => {
    expect(parseAppAuthorizationRequest(valid)).toEqual({
      clientName: 'Pier',
      redirectUri: valid.redirect_uri,
      redirectHost: '127.0.0.1:53127',
      codeChallenge: challenge,
      state: 'abc',
      keyName: 'Pier · laptop',
      scope: 'token',
    })
  })

  it('reads the requested scope', () => {
    expect(parseAppAuthorizationRequest({ ...valid, scope: 'token' })?.scope).toBe('token')
    expect(parseAppAuthorizationRequest({ ...valid, scope: 'account' })?.scope).toBe(
      'account'
    )
  })

  it.each([
    { code_challenge_method: 'plain' },
    { code_challenge: 'short' },
    { code_challenge: undefined },
    { client_name: ' ' },
    { client_name: 'Pier\u202e' },
    { client_name: 'a'.repeat(65) },
    { redirect_uri: 'https://attacker.example/callback' },
    { state: 's'.repeat(513) },
    { scope: 'admin' },
    { scope: 'account token' },
  ])('rejects %o', (override) => {
    expect(parseAppAuthorizationRequest({ ...valid, ...override })).toBeNull()
  })

  it('falls back to the app name and keeps key names within 50 bytes', () => {
    expect(
      parseAppAuthorizationRequest({ ...valid, key_name: undefined })?.keyName
    ).toBe('Pier')
    const long = parseAppAuthorizationRequest({
      ...valid,
      key_name: '云'.repeat(30),
    })?.keyName
    expect(long).toBe('云'.repeat(16))
    expect(utf8ByteLength(long ?? '')).toBeLessThanOrEqual(50)
  })
})

describe('buildDeniedRedirect', () => {
  it('reports access_denied with the original state', () => {
    const request = parseAppAuthorizationRequest(valid)
    if (!request) throw new Error('expected a valid request')
    const url = new URL(buildDeniedRedirect(request))
    expect(url.origin).toBe('http://127.0.0.1:53127')
    expect(url.searchParams.get('error')).toBe('access_denied')
    expect(url.searchParams.get('state')).toBe('abc')
  })
})
