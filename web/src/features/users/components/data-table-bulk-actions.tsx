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
import type { Table } from '@tanstack/react-table'
import { Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { handleServerError } from '@/lib/handle-server-error'
import { AuthOperationError } from '@/lib/secure-verification'

import { batchDeleteUsers } from '../api'
import { ERROR_MESSAGES } from '../constants'
import type { User } from '../types'
import { useUsers } from './users-provider'

interface DataTableBulkActionsProps {
  table: Table<User>
}

export function DataTableBulkActions({ table }: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { triggerRefresh, requestVerification, verificationActive } = useUsers()
  const [deleteTargets, setDeleteTargets] = useState<User[] | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)
  const selectedRows = table.getFilteredSelectedRowModel().rows

  const handleDelete = async () => {
    if (!deleteTargets?.length || isDeleting) return

    const ids = deleteTargets.map((user) => user.id)
    setIsDeleting(true)
    try {
      const proof = await requestVerification({
        scope: 'admin.user.batch_delete',
        context: { user_ids: ids },
        title: t('Verify to delete users'),
        description: t(
          'Confirm your identity before permanently deleting {{count}} accounts.',
          { count: ids.length }
        ),
      })
      if (!proof) return
      const result = await batchDeleteUsers(ids, proof.proof_token)
      if (!result.success) {
        handleServerError(result, t(ERROR_MESSAGES.DELETE_FAILED))
        return
      }
      const deleted = result.data?.deleted ?? 0
      const skipped = result.data?.skipped ?? 0
      toast.success(
        t('Successfully deleted {{count}} users', { count: deleted })
      )
      if (skipped > 0) {
        toast.warning(
          t('Skipped {{count}} users that are protected or no longer exist', {
            count: skipped,
          })
        )
      }
      table.resetRowSelection()
      setDeleteTargets(null)
      triggerRefresh()
    } catch (error) {
      handleServerError(
        AuthOperationError.from(error),
        t(ERROR_MESSAGES.UNEXPECTED)
      )
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <>
      <BulkActionsToolbar table={table} entityName={t('user')}>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='destructive'
                size='icon'
                className='size-8'
                aria-label={t('Delete selected users')}
                disabled={isDeleting}
                onClick={() =>
                  setDeleteTargets(selectedRows.map((row) => row.original))
                }
              />
            }
          >
            <Trash2 aria-hidden='true' />
          </TooltipTrigger>
          <TooltipContent>{t('Delete selected users')}</TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>
      <ConfirmDialog
        destructive
        open={deleteTargets !== null && !verificationActive}
        onOpenChange={(open) => {
          if (!open && !isDeleting) setDeleteTargets(null)
        }}
        title={t('Delete {{count}} users?', {
          count: deleteTargets?.length ?? 0,
        })}
        desc={t(
          'This will permanently delete the selected users. Super administrators and users at or above your role are skipped. This action cannot be undone.'
        )}
        confirmText={isDeleting ? t('Deleting...') : t('Delete')}
        isLoading={isDeleting}
        disabled={!deleteTargets?.length}
        handleConfirm={handleDelete}
      />
    </>
  )
}
