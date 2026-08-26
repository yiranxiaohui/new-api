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
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'

import { DefaultLegalDocument } from '../default-legal-document'

vi.mock('@/components/layout', () => ({
  PublicLayout: ({ children }: { children: React.ReactNode }) => (
    <main>{children}</main>
  ),
}))

describe('DefaultLegalDocument', () => {
  beforeEach(() => {
    i18next.addResourceBundle('en', 'translation', en.translation, true, true)
  })

  test.each([
    ['terms', 'Service and eligibility'],
    ['usage-policy', 'Prohibited conduct'],
    ['supported-regions', 'Availability principles'],
    ['service-specific-terms', 'Routing and upstream services'],
  ] as const)(
    'renders the complete %s document content',
    (documentId, sectionHeading) => {
      render(<DefaultLegalDocument documentId={documentId} />)

      expect(
        screen.getByRole('heading', { name: new RegExp(sectionHeading) })
      ).toBeVisible()
      expect(screen.getByText('Version:', { exact: true })).toBeVisible()
      expect(screen.getByText(/2026-08-26/)).toBeVisible()
    }
  )
})
