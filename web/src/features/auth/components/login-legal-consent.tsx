/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { ChevronRight, ExternalLink, FileText, ShieldCheck } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import type { SystemStatus } from '../types'

interface LoginLegalConsentProps {
  status: SystemStatus | null
  checked: boolean
  onCheckedChange: (nextValue: boolean) => void
  className?: string
}

type LegalDocumentLink = {
  href: string
  icon: typeof FileText
  label: string
}

export function LoginLegalConsent({
  status,
  checked,
  onCheckedChange,
  className,
}: LoginLegalConsentProps) {
  const { t } = useTranslation()
  const [isOpen, setIsOpen] = useState(false)
  const hasShownInitialDialog = useRef(false)
  const hasUserAgreement = Boolean(status?.user_agreement_enabled)
  const hasPrivacyPolicy = Boolean(status?.privacy_policy_enabled)
  const requiresLegalConsent = hasUserAgreement || hasPrivacyPolicy

  useEffect(() => {
    if (requiresLegalConsent && !hasShownInitialDialog.current) {
      hasShownInitialDialog.current = true
      setIsOpen(true)
    }
  }, [requiresLegalConsent])

  if (!requiresLegalConsent) {
    return null
  }

  const documents: LegalDocumentLink[] = []
  if (hasUserAgreement) {
    documents.push({
      href: '/user-agreement',
      icon: FileText,
      label: t('User Agreement'),
    })
  }
  if (hasPrivacyPolicy) {
    documents.push({
      href: '/privacy-policy',
      icon: ShieldCheck,
      label: t('Privacy Policy'),
    })
  }

  const handleAgree = () => {
    onCheckedChange(true)
    setIsOpen(false)
  }

  const handleDecline = () => {
    onCheckedChange(false)
    setIsOpen(false)
  }

  return (
    <>
      <Button
        type='button'
        variant={checked ? 'secondary' : 'outline'}
        className={cn(
          'h-auto w-full justify-between gap-3 px-3 py-2.5 text-left',
          className
        )}
        onClick={() => setIsOpen(true)}
      >
        <span className='flex min-w-0 items-center gap-2'>
          <ShieldCheck className='text-primary h-4 w-4 shrink-0' />
          <span className='truncate'>
            {checked
              ? t('Legal terms accepted')
              : t('Review and agree to the legal terms')}
          </span>
        </span>
        <ChevronRight className='text-muted-foreground h-4 w-4 shrink-0' />
      </Button>

      <Dialog
        open={isOpen}
        onOpenChange={setIsOpen}
        title={t('Terms update')}
        description={t(
          'Please review the legal documents below before signing in.'
        )}
        contentClassName='max-w-lg'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button type='button' variant='outline' onClick={handleDecline}>
              {t('Decline')}
            </Button>
            <Button type='button' onClick={handleAgree}>
              {t('Agree and continue')}
            </Button>
          </>
        }
      >
        <div className='space-y-2'>
          <h3 className='text-sm font-semibold'>{t('Related documents')}</h3>
          <div className='grid gap-3 sm:grid-cols-2'>
            {documents.map((document) => {
              const Icon = document.icon
              return (
                <a
                  key={document.href}
                  href={document.href}
                  target='_blank'
                  rel='noopener noreferrer'
                  className='border-border bg-background hover:bg-muted/50 flex min-w-0 items-center gap-3 rounded-lg border p-3 transition-colors'
                >
                  <span className='bg-muted flex h-9 w-9 shrink-0 items-center justify-center rounded-md'>
                    <Icon className='text-muted-foreground h-4 w-4' />
                  </span>
                  <span className='min-w-0 flex-1'>
                    <span className='block truncate text-sm font-medium'>
                      {document.label}
                    </span>
                    <span className='text-muted-foreground text-xs'>
                      {t('View document')}
                    </span>
                  </span>
                  <ExternalLink className='text-muted-foreground h-4 w-4 shrink-0' />
                </a>
              )
            })}
          </div>
        </div>
        <p className='text-muted-foreground text-xs leading-5'>
          {t('Please agree to the legal terms first')}
        </p>
      </Dialog>
    </>
  )
}
