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
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'

export type DefaultLegalDocumentId =
  | 'terms'
  | 'usage-policy'
  | 'supported-regions'
  | 'service-specific-terms'

const DOCUMENTS: Record<
  DefaultLegalDocumentId,
  { title: string; content: string }
> = {
  terms: {
    title: 'Terms of Service',
    content: 'Default Terms of Service document',
  },
  'usage-policy': {
    title: 'Usage Policy',
    content: 'Default Usage Policy document',
  },
  'supported-regions': {
    title: 'Supported Regions',
    content: 'Default Supported Regions document',
  },
  'service-specific-terms': {
    title: 'Service-Specific Terms',
    content: 'Default Service-Specific Terms document',
  },
}

export function DefaultLegalDocument({
  documentId,
}: {
  documentId: DefaultLegalDocumentId
}) {
  const { t } = useTranslation()
  const document = DOCUMENTS[documentId]
  const content = t(document.content).replace(/^# [^\n]+\n\n/u, '')

  return (
    <PublicLayout>
      <article className='mx-auto max-w-4xl space-y-8 py-8'>
        <header className='border-border space-y-3 border-b pb-6'>
          <h1 className='text-3xl font-semibold tracking-tight'>
            {t(document.title)}
          </h1>
          <p className='text-muted-foreground text-sm leading-6'>
            {t(
              'Please read this document together with the other legal documents linked on the sign-in and registration pages.'
            )}
          </p>
        </header>

        <RichContent
          mode='markdown'
          content={content}
          className='prose-neutral dark:prose-invert max-w-none'
        />
      </article>
    </PublicLayout>
  )
}
