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
import { api } from '@/lib/api'
import { authRequestOptions, authResult } from '@/lib/secure-verification'

import type {
  Withdrawal,
  WithdrawalConfig,
  WithdrawalInput,
  WithdrawalPolicy,
} from './types'

const base = '/api/user/withdrawals'
export function getWithdrawalPolicy() {
  return authResult<WithdrawalPolicy>(
    api.get(`${base}/policy`, authRequestOptions)
  )
}
export function getWithdrawalQuote(cents: number, signal?: AbortSignal) {
  return authResult<{ quota: number; amount_cents: number }>(
    api.get(`${base}/quote`, {
      ...authRequestOptions,
      params: { amount_cents: cents },
      signal,
    })
  )
}
export function getWithdrawals(admin: boolean, page: number) {
  return authResult<{ items: Withdrawal[]; has_more: boolean }>(
    api.get(admin ? `${base}/admin/` : base, {
      ...authRequestOptions,
      params: { page },
    })
  )
}
export function createWithdrawal(input: WithdrawalInput, proof: string) {
  return authResult<Withdrawal>(
    api.post(base, input, {
      ...authRequestOptions,
      headers: { 'X-Security-Proof': proof },
    })
  )
}
export function reviewWithdrawal(id: string, approve: boolean, proof: string) {
  return authResult<Record<string, never>>(
    api.post(
      `${base}/admin/review`,
      { id, approve },
      { ...authRequestOptions, headers: { 'X-Security-Proof': proof } }
    )
  )
}
export function getWithdrawalConfig() {
  return authResult<{ config: WithdrawalConfig; key_configured: boolean }>(
    api.get(`${base}/admin/config`, authRequestOptions)
  )
}
export function saveWithdrawalConfig(config: WithdrawalConfig, proof: string) {
  return authResult<Record<string, never>>(
    api.put(`${base}/admin/config`, config, {
      ...authRequestOptions,
      headers: { 'X-Security-Proof': proof },
    })
  )
}
export async function withdrawalConfigDigest(config: WithdrawalConfig) {
  // Matches the server's canonical struct order; no credential enters a URL.
  const body = JSON.stringify({
    enabled: config.enabled,
    gateway: config.gateway,
    pid: config.pid,
    api_key: config.api_key,
    notify_url: config.notify_url,
    cny_per_unit: config.cny_per_unit,
    min_cents: config.min_cents,
    max_cents: config.max_cents,
    scene: config.scene,
    scene_infos: config.scene_infos.map((s) => ({
      info_type: s.info_type,
      info_content: s.info_content,
    })),
  })
  // Go's JSON encoder escapes HTML and JavaScript line separators.
  const canonical = body.replaceAll(
    /[<>&\u2028\u2029]/g,
    (char) => `\\u${char.charCodeAt(0).toString(16).padStart(4, '0')}`
  )
  const digest = await crypto.subtle.digest(
    'SHA-256',
    new TextEncoder().encode(canonical)
  )
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, '0')
  ).join('')
}
