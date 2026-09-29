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
import { Loader2, LogIn, MonitorSmartphone, UserRound } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Item,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from '@/components/ui/item'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'
import type { AuthUser } from '@/stores/auth-store'

import { authorizeAppSignIn } from '../api'
import {
  type AppAuthorizationRequest,
  buildDeniedRedirect,
} from '../lib/request'

type AppSignInFormProps = {
  request: AppAuthorizationRequest
  siteName: string
  user: AuthUser
  onRedirect: (outcome: 'approved' | 'denied', url: string) => void
}

/** Consent for an app that asks to sign in to the account (`scope=account`). */
export function AppSignInForm(props: AppSignInFormProps) {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)

  async function approve() {
    setSubmitting(true)
    try {
      const res = await authorizeAppSignIn({
        client_name: props.request.clientName,
        redirect_uri: props.request.redirectUri,
        code_challenge: props.request.codeChallenge,
        code_challenge_method: 'S256',
        state: props.request.state,
      })
      if (!res.success || !res.data?.redirect_url) {
        throw createServerError(res, t('Authorization failed'))
      }
      props.onRedirect('approved', res.data.redirect_url)
    } catch (error) {
      handleServerError(error, t('Authorization failed'))
      setSubmitting(false)
    }
  }

  const accountName = props.user.display_name || props.user.username

  return (
    <div className='w-full space-y-6'>
      <div className='flex flex-col items-center space-y-4 text-center'>
        <div className='bg-muted flex h-16 w-16 items-center justify-center rounded-2xl'>
          <LogIn className='h-8 w-8' />
        </div>
        <div className='space-y-2'>
          <h2 className='text-2xl font-semibold tracking-tight'>
            {t('Sign in to {{app}}', { app: props.request.clientName })}
          </h2>
          <p className='text-muted-foreground text-sm sm:text-base'>
            {t('{{app}} wants to sign in to your {{site}} account.', {
              app: props.request.clientName,
              site: props.siteName,
            })}
          </p>
        </div>
      </div>

      <ItemGroup className='gap-2'>
        <Item variant='outline' size='sm'>
          <ItemMedia variant='icon'>
            <UserRound />
          </ItemMedia>
          <ItemContent>
            <ItemTitle>
              {t('Signed in as {{name}}', { name: accountName })}
            </ItemTitle>
            <ItemDescription>
              {t(
                'The app can view your balance and usage and manage your API keys, like a signed-in browser.'
              )}
            </ItemDescription>
          </ItemContent>
        </Item>
        <Item variant='outline' size='sm'>
          <ItemMedia variant='icon'>
            <MonitorSmartphone />
          </ItemMedia>
          <ItemContent>
            <ItemTitle>{t('Delivered to an app on this device')}</ItemTitle>
            <ItemDescription>
              {t('The sign-in is sent only to {{address}} on this computer.', {
                address: props.request.redirectHost,
              })}
            </ItemDescription>
          </ItemContent>
        </Item>
      </ItemGroup>

      <Alert>
        <AlertDescription>
          {t(
            'The app name is provided by the app itself and is not verified. Only continue if you just started this sign-in. You can sign the app out under Login sessions at any time.'
          )}
        </AlertDescription>
      </Alert>

      <div className='grid grid-cols-2 gap-3'>
        <Button
          type='button'
          variant='outline'
          disabled={submitting}
          onClick={() =>
            props.onRedirect('denied', buildDeniedRedirect(props.request))
          }
        >
          {t('Cancel')}
        </Button>
        <Button type='button' disabled={submitting} onClick={approve}>
          {submitting && <Loader2 className='animate-spin' />}
          {t('Authorize')}
        </Button>
      </div>
    </div>
  )
}
