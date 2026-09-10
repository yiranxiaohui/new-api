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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card'

import { getWithdrawalPolicy } from './api'
import { WithdrawalForm } from './withdrawal-form'
import { WithdrawalHistory } from './withdrawal-history'

export function WithdrawalCard(props: {
  availableQuota: number
  onUpdate: () => void
}) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const [open, setOpen] = useState(false)
  const client = useQueryClient()
  const policy = useQuery({
    queryKey: ['withdrawal-policy'],
    queryFn: getWithdrawalPolicy,
  })
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Withdraw referral rewards')}</CardTitle>
        <CardDescription>
          {t(
            'Withdraw to Alipay after administrator approval. Pending amounts are frozen and cannot be transferred to your balance.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>
        {policy.error ? (
          <ErrorState onRetry={() => void policy.refetch()} />
        ) : (
          <Button
            onClick={() => setOpen(true)}
            disabled={!policy.data?.enabled || props.availableQuota <= 0}
          >
            {t('Request withdrawal')}
          </Button>
        )}
        {policy.data?.enabled === false && (
          <p className='text-muted-foreground text-sm'>
            {t('Affiliate withdrawals are not configured or enabled.')}
          </p>
        )}
        <WithdrawalHistory />
      </CardContent>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) setOpen(value)
        }}
        showCloseButton={!busy}
        title={t('Withdraw referral rewards')}
        contentHeight='auto'
      >
        {open && policy.data && (
          <WithdrawalForm
            onBusyChange={setBusy}
            policy={policy.data}
            availableQuota={props.availableQuota}
            onSuccess={() => {
              setOpen(false)
              props.onUpdate()
              void client.invalidateQueries({ queryKey: ['withdrawals'] })
            }}
          />
        )}
      </Dialog>
    </Card>
  )
}
