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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Pagination,
  PaginationContent,
  PaginationItem,
} from '@/components/ui/pagination'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { formatQuota } from '@/lib/format'
import { AuthOperationError } from '@/lib/secure-verification'
import { useAuthStore } from '@/stores/auth-store'

import { getWithdrawals, reviewWithdrawal } from './api'
import type { Withdrawal } from './types'

export function WithdrawalHistory(props: { admin?: boolean }) {
  const { t } = useTranslation()
  const userID = useAuthStore((state) => state.auth.user?.id)
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<{
    row: Withdrawal
    approve: boolean
  } | null>(null)
  const [busy, setBusy] = useState(false)
  const submitting = useRef(false)
  const client = useQueryClient()
  const verification = useSecureVerification()
  const mutation = useMutation({
    gcTime: 0,
    // The caller shows a sanitized error; the global logger includes request data.
    onError: () => {},
    mutationFn: ({
      id,
      approve,
      proof,
    }: {
      id: string
      approve: boolean
      proof: string
    }) => reviewWithdrawal(id, approve, proof),
  })
  const query = useQuery({
    queryKey: ['withdrawals', userID, !!props.admin, page],
    queryFn: () => getWithdrawals(!!props.admin, page),
    refetchInterval: 15000,
  })
  const labels = {
    pending: t('Awaiting review'),
    processing: t('Payout processing'),
    succeeded: t('Withdrawal paid'),
    failed: t('Payout failed; rewards returned'),
    rejected: t('Rejected; rewards returned'),
  }
  const review = async () => {
    if (!selected || submitting.current) return
    submitting.current = true
    setBusy(true)
    const current = selected
    try {
      const proof = await verification.requestVerification({
        scope: 'withdrawal.review',
        context: { id: current.row.id, approve: current.approve },
        description: `${current.row.payee_account} · ${current.row.payee_name} · CNY ${(current.row.amount_cents / 100).toFixed(2)}`,
      })
      if (!proof) return
      await mutation.mutateAsync({
        id: current.row.id,
        approve: current.approve,
        proof: proof.proof_token,
      })
      setSelected(null)
      await client.invalidateQueries({ queryKey: ['withdrawals'] })
    } catch (error) {
      toast.error(t(AuthOperationError.from(error).message))
    } finally {
      mutation.reset()
      submitting.current = false
      setBusy(false)
    }
  }
  if (query.isPending) return <LoadingState />
  if (query.error) return <ErrorState onRetry={() => void query.refetch()} />
  return (
    <div className='space-y-4'>
      {query.data.items.length === 0 ? (
        <EmptyState title={t('No withdrawal requests')} />
      ) : (
        <ul className='space-y-3'>
          {query.data.items.map((row) => (
            <li key={row.id} className='space-y-2 rounded-lg border p-3'>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <span className='font-semibold'>
                  CNY {(row.amount_cents / 100).toFixed(2)}
                </span>
                <Badge variant='secondary'>{labels[row.status]}</Badge>
              </div>
              <p className='text-sm break-all'>
                {row.payee_account} · {row.payee_name}
              </p>
              <p className='text-muted-foreground text-xs break-all'>
                {row.id} · {formatQuota(row.quota)}
                {props.admin ? ` · ${t('User ID')}: ${row.user_id}` : ''}
              </p>
              {row.payout_no && (
                <p className='text-muted-foreground text-xs break-all'>
                  {t('Payout reference')}: {row.payout_no}
                </p>
              )}
              {props.admin && row.status === 'pending' && (
                <div className='flex gap-2'>
                  <Button
                    size='sm'
                    disabled={busy}
                    onClick={() => setSelected({ row, approve: true })}
                  >
                    {t('Approve payout')}
                  </Button>
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={busy}
                    onClick={() => setSelected({ row, approve: false })}
                  >
                    {t('Reject withdrawal')}
                  </Button>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
      <Pagination aria-label={t('Withdrawal history')}>
        <PaginationContent>
          <PaginationItem>
            <Button
              variant='outline'
              disabled={page === 1 || query.isFetching}
              onClick={() => setPage((value) => value - 1)}
            >
              {t('Previous')}
            </Button>
          </PaginationItem>
          <PaginationItem>
            <span className='px-3'>{page}</span>
          </PaginationItem>
          <PaginationItem>
            <Button
              variant='outline'
              disabled={!query.data.has_more || query.isFetching}
              onClick={() => setPage((value) => value + 1)}
            >
              {t('Next')}
            </Button>
          </PaginationItem>
        </PaginationContent>
      </Pagination>
      <ConfirmDialog
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open && !busy) setSelected(null)
        }}
        title={selected?.approve ? t('Approve payout') : t('Reject withdrawal')}
        desc={
          selected
            ? `${selected.row.payee_account} · ${selected.row.payee_name} · CNY ${(selected.row.amount_cents / 100).toFixed(2)}`
            : ''
        }
        isLoading={busy}
        handleConfirm={() => void review()}
      />
      <SecureVerificationDialog {...verification.dialogProps} />
    </div>
  )
}
