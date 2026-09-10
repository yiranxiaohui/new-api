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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { formatQuota } from '@/lib/format'
import { AuthOperationError } from '@/lib/secure-verification'

import { createWithdrawal, getWithdrawalQuote } from './api'
import {
  parseCNYCents,
  type WithdrawalInput,
  type WithdrawalPolicy,
} from './types'

const schema = z.object({
  amount: z
    .string()
    .refine(
      (value) => parseCNYCents(value) !== null,
      'Invalid withdrawal details or amount.'
    ),
  payee_account: z.string().trim().min(1).max(100),
  payee_name: z.string().trim().min(1).max(100),
})
type Values = z.infer<typeof schema>
export function WithdrawalForm(props: {
  availableQuota: number
  policy: WithdrawalPolicy
  onSuccess: () => void
  onBusyChange?: (busy: boolean) => void
}) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const submitting = useRef(false)
  const [id] = useState(() => `wd_${crypto.randomUUID().replaceAll('-', '')}`)
  const verification = useSecureVerification()
  const mutation = useMutation({
    gcTime: 0,
    // The caller shows a sanitized error; the global logger includes request data.
    onError: () => {},
    mutationFn: ({ input, proof }: { input: WithdrawalInput; proof: string }) =>
      createWithdrawal(input, proof),
  })
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { amount: '', payee_account: '', payee_name: '' },
  })
  const cents = parseCNYCents(form.watch('amount'))
  const validAmount =
    cents !== null &&
    cents >= props.policy.min_cents &&
    cents <= props.policy.max_cents
  const quote = useQuery({
    queryKey: ['withdrawal-quote', cents],
    queryFn: ({ signal }) => getWithdrawalQuote(cents ?? 0, signal),
    enabled: props.policy.enabled && validAmount,
    retry: false,
    staleTime: 0,
  })
  const enough =
    quote.data !== undefined && quote.data.quota <= props.availableQuota
  const submit = form.handleSubmit(async (values) => {
    if (
      submitting.current ||
      !props.policy.enabled ||
      cents === null ||
      !validAmount ||
      !enough ||
      quote.isFetching ||
      !quote.data
    ) {
      return
    }
    submitting.current = true
    setBusy(true)
    props.onBusyChange?.(true)
    const input = {
      id,
      amount_cents: cents,
      quota: quote.data.quota,
      payee_account: values.payee_account,
      payee_name: values.payee_name,
    }
    try {
      const proof = await verification.requestVerification({
        scope: 'withdrawal.create',
        context: input,
        description: `${values.payee_account} · ${values.payee_name} · CNY ${(cents / 100).toFixed(2)} · ${formatQuota(input.quota)}`,
      })
      if (!proof) return
      await mutation.mutateAsync({ input, proof: proof.proof_token })
      toast.success(t('Withdrawal request submitted'))
      props.onSuccess()
    } catch (error) {
      toast.error(t(AuthOperationError.from(error).message))
    } finally {
      mutation.reset()
      submitting.current = false
      setBusy(false)
      props.onBusyChange?.(false)
    }
  })
  return (
    <>
      <Form {...form}>
        <form onSubmit={submit} className='space-y-4'>
          <fieldset disabled={busy} className='space-y-4'>
            <FormField
              control={form.control}
              name='amount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Withdrawal amount (CNY)')}</FormLabel>
                  <FormControl>
                    <Input {...field} inputMode='decimal' autoComplete='off' />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <p className='text-muted-foreground text-sm'>
              {t('Withdrawal limits: {{min}}–{{max}} CNY', {
                min: (props.policy.min_cents / 100).toFixed(2),
                max: (props.policy.max_cents / 100).toFixed(2),
              })}
            </p>
            <FormField
              control={form.control}
              name='payee_account'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Alipay account')}</FormLabel>
                  <FormControl>
                    <Input {...field} maxLength={100} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='payee_name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Recipient legal name')}</FormLabel>
                  <FormControl>
                    <Input {...field} maxLength={100} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </fieldset>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Enable two-factor authentication or a passkey before requesting withdrawals.'
            )}
          </p>
          {validAmount && quote.isFetching && (
            <p role='status'>{t('Loading...')}</p>
          )}
          {validAmount && quote.error && (
            <p role='alert'>
              {t(AuthOperationError.from(quote.error).message)}
            </p>
          )}
          {validAmount && quote.data && (
            <p>
              {t('Referral quota to freeze: {{quota}}', {
                quota: formatQuota(quote.data.quota),
              })}
            </p>
          )}
          {validAmount && quote.data && !enough && (
            <p role='alert'>{t('Insufficient available referral rewards.')}</p>
          )}
          <Button
            type='submit'
            disabled={
              busy ||
              !props.policy.enabled ||
              cents === null ||
              !validAmount ||
              !enough ||
              quote.isFetching
            }
          >
            {t('Request withdrawal')}
          </Button>
        </form>
      </Form>
      <SecureVerificationDialog {...verification.dialogProps} />
    </>
  )
}
