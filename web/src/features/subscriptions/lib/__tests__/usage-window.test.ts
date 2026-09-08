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
import type { TFunction } from 'i18next'
import { describe, expect, test } from 'vitest'

import { formatUsageWindow } from '../format'
import {
  formValuesToPlanPayload,
  getPlanFormSchema,
  PLAN_FORM_DEFAULTS,
} from '../plan-form'

const t = ((key: string) => key) as TFunction

describe('subscription usage window', () => {
  test('accepts an overnight range and preserves it in the API payload', () => {
    const values = {
      ...PLAN_FORM_DEFAULTS,
      title: 'Night plan',
      usage_window_start: '22:00',
      usage_window_end: '06:00',
      usage_window_timezone: 'Asia/Shanghai',
    }

    expect(getPlanFormSchema(t).safeParse(values).success).toBe(true)
    expect(formValuesToPlanPayload(values).plan).toMatchObject({
      usage_window_start: '22:00',
      usage_window_end: '06:00',
      usage_window_timezone: 'Asia/Shanghai',
    })
    expect(formatUsageWindow(values, t)).toBe('22:00-06:00 (Asia/Shanghai)')
  })

  test('rejects incomplete and zero-length ranges', () => {
    const schema = getPlanFormSchema(t)

    expect(
      schema.safeParse({
        ...PLAN_FORM_DEFAULTS,
        title: 'Day plan',
        usage_window_start: '09:00',
      }).success
    ).toBe(false)
    expect(
      schema.safeParse({
        ...PLAN_FORM_DEFAULTS,
        title: 'Day plan',
        usage_window_start: '09:00',
        usage_window_end: '09:00',
      }).success
    ).toBe(false)
  })

  test('renders an empty range as all-day access', () => {
    expect(formatUsageWindow(PLAN_FORM_DEFAULTS, t)).toBe('All day')
  })
})
