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
import { Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import type { VisitorPlatform } from '../lib/platform'

type SetupGuideProps = {
  appAuthorizationEnabled: boolean
  hasMirror: boolean
  visitorPlatform: VisitorPlatform | null
}

export function SetupGuide(props: SetupGuideProps) {
  const { t } = useTranslation()
  const steps = props.appAuthorizationEnabled
    ? [
        t('Download and install Pier for your device.'),
        t(
          'Open Pier, go to Settings → Models & Providers, and choose browser sign-in.'
        ),
        t(
          'Approve the request on this site. Pier creates an API key and configures the available models automatically.'
        ),
      ]
    : [
        t('Download and install Pier for your device.'),
        t('Sign in to this site and create an API key in the console.'),
        t(
          'Open Pier, go to Settings → Models & Providers, and add a custom provider with this site address and the API key.'
        ),
      ]

  return (
    <section className='grid gap-4 lg:grid-cols-[3fr_2fr]'>
      <Card>
        <CardHeader>
          <CardTitle className='text-base'>
            <h2>{t('Get started')}</h2>
          </CardTitle>
        </CardHeader>
        <CardContent>
          <ol className='space-y-3'>
            {steps.map((step, index) => (
              <li key={step} className='flex gap-3 text-sm'>
                <span
                  aria-hidden='true'
                  className='bg-primary/10 text-primary flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold'
                >
                  {index + 1}
                </span>
                <span className='pt-0.5'>{step}</span>
              </li>
            ))}
          </ol>
        </CardContent>
      </Card>

      <div className='space-y-3'>
        {props.visitorPlatform === 'ios' && (
          <Alert role='note'>
            <Info aria-hidden='true' />
            <AlertTitle>{t('iOS is not supported yet')}</AlertTitle>
            <AlertDescription>
              {t('Install Pier on a computer or an Android phone.')}
            </AlertDescription>
          </Alert>
        )}
        <Alert role='note'>
          <Info aria-hidden='true' />
          <AlertTitle>{t('First launch on macOS')}</AlertTitle>
          <AlertDescription>
            {t(
              'If macOS says the developer cannot be verified, open Pier once, then go to System Settings → Privacy & Security and choose Open Anyway.'
            )}
          </AlertDescription>
        </Alert>
        {props.hasMirror && (
          <Alert role='note'>
            <Info aria-hidden='true' />
            <AlertTitle>{t('Accelerated downloads')}</AlertTitle>
            <AlertDescription>
              {t(
                'Downloads use a GitHub acceleration mirror. Compare the SHA-256 checksum, or download directly from GitHub.'
              )}
            </AlertDescription>
          </Alert>
        )}
      </div>
    </section>
  )
}
