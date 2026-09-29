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
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import { Skeleton } from '@/components/ui/skeleton'
import { useStatus } from '@/hooks/use-status'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getClientDownloads } from './api'
import { DownloadsHero } from './components/downloads-hero'
import { PlatformDownloads } from './components/platform-downloads'
import { ReleaseNotes } from './components/release-notes'
import { SetupGuide } from './components/setup-guide'
import { detectVisitorPlatform } from './lib/platform'

type NavigatorWithUAData = Navigator & {
  userAgentData?: { platform?: string }
}

export function Downloads() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const visitorPlatform = useMemo(() => {
    const nav = navigator as NavigatorWithUAData
    return detectVisitorPlatform(nav.userAgent, nav.userAgentData?.platform)
  }, [])
  const releaseQuery = useQuery({
    queryKey: ['client-downloads'],
    queryFn: async () => requireServerSuccess(await getClientDownloads()),
    staleTime: 5 * 60 * 1000,
    meta: { errorToast: false },
  })
  const release = releaseQuery.data?.data

  let content
  if (releaseQuery.isLoading) {
    content = (
      <div className='grid gap-4 md:grid-cols-2'>
        <Skeleton className='h-44 w-full rounded-xl' />
        <Skeleton className='h-44 w-full rounded-xl' />
      </div>
    )
  } else if (!release) {
    content = (
      <ErrorState
        title={t('Unable to load downloads')}
        description={
          releaseQuery.error instanceof Error
            ? releaseQuery.error.message
            : undefined
        }
        onRetry={() => void releaseQuery.refetch()}
      />
    )
  } else {
    content = (
      <>
        <PlatformDownloads
          assets={release.assets}
          visitorPlatform={visitorPlatform}
        />
        <SetupGuide
          appAuthorizationEnabled={status?.app_authorization_enabled === true}
          hasMirror={release.assets.some((asset) => Boolean(asset.mirror_url))}
          visitorPlatform={visitorPlatform}
        />
        <ReleaseNotes release={release} />
      </>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='mx-auto w-full max-w-[1080px] space-y-8 px-3 pt-16 pb-10 sm:px-6 sm:pt-20 sm:pb-12 xl:px-8'>
        <DownloadsHero release={release} visitorPlatform={visitorPlatform} />
        {content}
      </PageTransition>
    </PublicLayout>
  )
}
