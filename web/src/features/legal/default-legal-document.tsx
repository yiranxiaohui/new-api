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

export type DefaultLegalDocumentId =
  | 'terms'
  | 'usage-policy'
  | 'supported-regions'
  | 'service-specific-terms'

const DOCUMENTS: Record<
  DefaultLegalDocumentId,
  { title: string; intro: string; points: string[] }
> = {
  terms: {
    title: 'Terms of Service',
    intro:
      'These Terms of Service govern your access to and use of this site and its API services.',
    points: [
      'Use the service only for lawful purposes and in accordance with these terms.',
      'You are responsible for the accounts, credentials, content, and API requests made through your account.',
      'We may suspend access when necessary to protect the service, users, or legal rights.',
    ],
  },
  'usage-policy': {
    title: 'Usage Policy',
    intro:
      'This Usage Policy describes prohibited and restricted uses of this site and its API services.',
    points: [
      'Do not use the service to violate applicable law, infringe rights, or evade access controls.',
      'Do not interfere with the service, bypass usage limits, or attempt to access another user account.',
      'You are responsible for reviewing the rules that apply to each model, provider, and integration you use.',
    ],
  },
  'supported-regions': {
    title: 'Supported Regions',
    intro:
      'Availability depends on applicable laws, sanctions, payment availability, and provider restrictions.',
    points: [
      'Do not access the service from a region where its use or the requested provider is prohibited.',
      'You are responsible for confirming that your use, payments, and connected services are permitted in your region.',
      'Availability may change when legal, security, or provider requirements change.',
    ],
  },
  'service-specific-terms': {
    title: 'Service-Specific Terms',
    intro:
      'Some models, tools, and integrations may have additional provider requirements beyond these general terms.',
    points: [
      'Provider-specific limits, availability, and content rules apply when you use an integrated service.',
      'You must have the rights and authorizations required for any account, key, or content you connect.',
      'When provider terms conflict with these terms, the stricter requirement applies to that provider service.',
    ],
  },
}

export function DefaultLegalDocument({
  documentId,
}: {
  documentId: DefaultLegalDocumentId
}) {
  const { t } = useTranslation()
  const document = DOCUMENTS[documentId]

  return (
    <PublicLayout>
      <article className='mx-auto max-w-3xl space-y-8 py-8'>
        <header className='space-y-3'>
          <h1 className='text-3xl font-semibold tracking-tight'>
            {t(document.title)}
          </h1>
          <p className='text-muted-foreground text-sm leading-6'>
            {t(document.intro)}
          </p>
        </header>

        <section className='space-y-4'>
          <h2 className='text-xl font-semibold'>{t('Overview')}</h2>
          <p className='text-muted-foreground text-sm leading-6'>
            {t(
              'Please read this document together with the other legal documents linked on the sign-in and registration pages.'
            )}
          </p>
        </section>

        <section className='space-y-4'>
          <h2 className='text-xl font-semibold'>{t('Important')}</h2>
          <ul className='text-muted-foreground list-disc space-y-3 ps-5 text-sm leading-6'>
            {document.points.map((point) => (
              <li key={point}>{t(point)}</li>
            ))}
          </ul>
        </section>

        <p className='text-muted-foreground border-border border-t pt-6 text-xs leading-5'>
          {t(
            'This page provides general service information and does not replace professional legal advice.'
          )}
        </p>
      </article>
    </PublicLayout>
  )
}
