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

export type AuthorizeAppPayload = {
  client_name: string
  redirect_uri: string
  code_challenge: string
  code_challenge_method: 'S256'
  state: string
  token: {
    name: string
    group: string
    unlimited_quota: boolean
    remain_quota: number
    expired_time: number
    model_limits_enabled: boolean
    model_limits: string
    allow_ips: string
    cross_group_retry: boolean
  }
}

export type AuthorizeAppResponse = {
  success: boolean
  message?: string
  data?: { redirect_url: string }
}

export type AuthorizeAppSignInPayload = Omit<AuthorizeAppPayload, 'token'>

/** Approve an app: creates the API key and returns the loopback redirect URL. */
export async function authorizeApp(
  payload: AuthorizeAppPayload
): Promise<AuthorizeAppResponse> {
  const res = await api.post('/api/app-auth/authorize', payload)
  return res.data
}

/**
 * Let an app sign in to the account: returns the loopback redirect URL whose
 * code exchanges for a login session of the app's own.
 */
export async function authorizeAppSignIn(
  payload: AuthorizeAppSignInPayload
): Promise<AuthorizeAppResponse> {
  const res = await api.post('/api/app-auth/authorize/account', payload)
  return res.data
}
