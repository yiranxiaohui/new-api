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

import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

import type { SystemStatus } from '../types'
import { LEGAL_DOCUMENTS } from './legal-documents'

interface LegalConsentProps {
  status: SystemStatus | null
  checked: boolean
  onCheckedChange: (nextValue: boolean) => void
  className?: string
}

export function LegalConsent({
  status,
  checked,
  onCheckedChange,
  className,
}: LegalConsentProps) {
  const { t } = useTranslation()
  const hasUserAgreement = Boolean(status?.user_agreement_enabled)
  const hasPrivacyPolicy = Boolean(status?.privacy_policy_enabled)

  if (!hasUserAgreement && !hasPrivacyPolicy) {
    return null
  }

  const handleChange = (value: boolean) => {
    onCheckedChange(value === true)
  }

  const documents = [
    ...LEGAL_DOCUMENTS,
    ...(hasUserAgreement
      ? [{ label: 'User Agreement', href: '/user-agreement' }]
      : []),
    ...(hasPrivacyPolicy
      ? [{ label: 'Privacy Policy', href: '/privacy-policy' }]
      : []),
  ]

  return (
    <div
      className={cn(
        'border-border/60 bg-muted/40 flex items-start gap-3 rounded-md border p-3',
        className
      )}
    >
      <Checkbox
        id='legal-consent'
        checked={checked}
        onCheckedChange={handleChange}
        className='mt-0.5'
      />
      <Label
        htmlFor='legal-consent'
        className='text-muted-foreground items-start gap-1 text-left text-xs leading-5 font-normal'
      >
        <span>
          {t(
            'Before registering, signing in, topping up, creating an API Key, or using the service, please read and agree to'
          )}{' '}
          {documents.map((document, index) => (
            <span key={document.href}>
              {index > 0 &&
                (index === documents.length - 1 ? ` ${t('and')} ` : ', ')}
              <a
                href={document.href}
                target='_blank'
                rel='noopener noreferrer'
                className='text-primary hover:underline'
              >
                {t(document.label)}
              </a>
            </span>
          ))}
          .{' '}
          {t(
            'By continuing, you confirm that you meet the applicable account, regional, and compliance requirements.'
          )}
        </span>
      </Label>
    </div>
  )
}
