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
export type WithdrawalPolicy = {
  enabled: boolean
  min_cents: number
  max_cents: number
  cny_per_unit: string
}
export type WithdrawalConfig = WithdrawalPolicy & {
  gateway: string
  pid: string
  api_key: string
  notify_url: string
  scene: string
  scene_infos: { info_type: string; info_content: string }[]
}
export type Withdrawal = {
  id: string
  user_id: number
  quota: number
  amount_cents: number
  status: 'pending' | 'processing' | 'succeeded' | 'failed' | 'rejected'
  payout_no: string
  failure_code: string
  payee_account: string
  payee_name: string
  created_at: number
}
export type WithdrawalInput = {
  id: string
  amount_cents: number
  quota: number
  payee_account: string
  payee_name: string
}

export function parseCNYCents(value: string): number | null {
  if (!/^\d{1,7}(\.\d{1,2})?$/.test(value)) return null
  const [whole, fraction = ''] = value.split('.')
  const cents = Number(whole) * 100 + Number(fraction.padEnd(2, '0'))
  return cents > 0 && cents <= 100000000 ? cents : null
}
