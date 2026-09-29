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
import { useSearch } from '@tanstack/react-router'
import { CheckCircle2, XCircle } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { AuthLayout } from '@/features/auth/auth-layout'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { useAuthStore } from '@/stores/auth-store'

import { AppAuthorizationForm } from './components/app-authorization-form'
import { AppSignInForm } from './components/app-sign-in-form'
import { parseAppAuthorizationRequest } from './lib/request'

/**
 * Consent page for native apps that request an API key or to sign in to the
 * account (`/app-auth`). The route guard has already made sure the user is
 * signed in.
 */
export function AppAuthorization() {
  const { t } = useTranslation()
  const search = useSearch({ from: '/(auth)/app-auth' })
  const request = useMemo(() => parseAppAuthorizationRequest(search), [search])
  const { status, loading } = useStatus()
  const { systemName } = useSystemConfig()
  const user = useAuthStore((s) => s.auth.user)
  const [outcome, setOutcome] = useState<'approved' | 'denied' | null>(null)

  const redirect = (next: 'approved' | 'denied', url: string) => {
    setOutcome(next)
    window.location.assign(url)
  }

  let content: React.ReactNode
  if (!request) {
    content = (
      <ErrorState
        title={t('Invalid authorization request')}
        description={t(
          'The link is incomplete or was not created by a supported app. Start the sign-in again from the app.'
        )}
      />
    )
  } else if (!status && loading) {
    content = <LoadingState />
  } else if (status?.app_authorization_enabled !== true) {
    content = (
      <ErrorState
        title={t('App authorization is not enabled')}
        description={t(
          'The administrator of this site has not enabled app authorization.'
        )}
      />
    )
  } else if (outcome) {
    const Icon = outcome === 'approved' ? CheckCircle2 : XCircle
    content = (
      <ErrorState
        icon={Icon}
        title={
          outcome === 'approved'
            ? t('Returning to {{app}}', { app: request.clientName })
            : t('Authorization cancelled')
        }
        description={t(
          'You can close this page and go back to {{app}}.',
          { app: request.clientName }
        )}
      />
    )
  } else if (user && request.scope === 'account') {
    content = (
      <AppSignInForm
        request={request}
        siteName={systemName}
        user={user}
        onRedirect={redirect}
      />
    )
  } else if (user) {
    content = (
      <AppAuthorizationForm
        request={request}
        siteName={systemName}
        user={user}
        defaultUseAutoGroup={status.default_use_auto_group === true}
        onRedirect={redirect}
      />
    )
  } else {
    content = <LoadingState />
  }

  return <AuthLayout>{content}</AuthLayout>
}
