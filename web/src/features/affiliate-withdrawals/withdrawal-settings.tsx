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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { PasswordInput } from '@/components/password-input'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { FormNavigationGuard } from '@/features/system-settings/components/form-navigation-guard'
import {
  SettingsForm,
  SettingsFormGrid,
} from '@/features/system-settings/components/settings-form-layout'
import { SettingsPageFormActions } from '@/features/system-settings/components/settings-page-context'
import { SettingsSection } from '@/features/system-settings/components/settings-section'
import { useSettingsForm } from '@/features/system-settings/hooks/use-settings-form'
import { AuthOperationError } from '@/lib/secure-verification'

import {
  getWithdrawalConfig,
  saveWithdrawalConfig,
  withdrawalConfigDigest,
} from './api'
import type { WithdrawalConfig } from './types'
import { WithdrawalHistory } from './withdrawal-history'

const schema = z
  .object({
    enabled: z.boolean(),
    mode: z.enum(['manual', 'unipay']),
    gateway: z.string().max(2048),
    pid: z.string().max(64),
    api_key: z.string().regex(/^([0-9a-fA-F]{64})?$/),
    notify_url: z.string().max(2048),
    cny_per_unit: z
      .string()
      .regex(/^\d+(\.\d{1,6})?$/)
      .refine((value) => Number(value) > 0 && Number(value) <= 1000000),
    min_cents: z.coerce.number().int().min(1),
    max_cents: z.coerce.number().int().min(1).max(100000000),
    scene: z.string().max(64),
    scene_infos_json: z.string().refine((value) => {
      try {
        return z
          .array(
            z.object({
              info_type: z.string().min(1).max(64),
              info_content: z.string().min(1).max(300),
            })
          )
          .max(10)
          .safeParse(JSON.parse(value)).success
      } catch {
        return false
      }
    }, 'Invalid payout scene information.'),
  })
  .refine((value) => value.max_cents >= value.min_cents, {
    path: ['max_cents'],
    message: 'Invalid withdrawal details or amount.',
  })
  .superRefine((value, context) => {
    // UniPay fields are only required when payouts go through the gateway.
    if (value.mode !== 'unipay') return
    for (const name of ['gateway', 'notify_url'] as const) {
      if (!z.url().safeParse(value[name]).success) {
        context.addIssue({
          code: 'custom',
          path: [name],
          message: 'Must be a valid URL',
        })
      }
    }
    for (const name of ['pid', 'scene'] as const) {
      if (value[name].trim() === '') {
        context.addIssue({ code: 'custom', path: [name], message: 'Required' })
      }
    }
  })
type Values = z.infer<typeof schema>

function WithdrawalSettingsForm(props: {
  config: WithdrawalConfig
  keyConfigured: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const verification = useSecureVerification()
  const mutation = useMutation({
    gcTime: 0,
    // The caller shows a sanitized error; the global logger includes request data.
    onError: () => {},
    mutationFn: ({
      config,
      proof,
    }: {
      config: WithdrawalConfig
      proof: string
    }) => saveWithdrawalConfig(config, proof),
  })
  const { form, handleSubmit, isDirty, isSubmitting } = useSettingsForm<Values>(
    {
      resolver: zodResolver(schema) as Resolver<Values>,
      defaultValues: {
        ...props.config,
        api_key: '',
        scene_infos_json: JSON.stringify(
          props.config.scene_infos ?? [],
          null,
          2
        ),
      },
      onSubmit: async (data) => {
        const { scene_infos_json, ...values } = data
        const config: WithdrawalConfig = {
          ...values,
          scene_infos: JSON.parse(
            scene_infos_json
          ) as WithdrawalConfig['scene_infos'],
        }
        const digest = await withdrawalConfigDigest(config)
        const proof = await verification.requestVerification({
          scope: 'withdrawal.configure',
          context: { digest },
          description: t('Verify changes to withdrawal settings'),
        })
        if (!proof) throw new Error(t('Verification cancelled'))
        try {
          await mutation.mutateAsync({ config, proof: proof.proof_token })
        } finally {
          mutation.reset()
        }
        // useSettingsForm resets to this object after onSubmit returns.
        data.api_key = ''
        form.setValue('api_key', '')
        await queryClient.invalidateQueries({ queryKey: ['withdrawal-config'] })
        await queryClient.invalidateQueries({ queryKey: ['withdrawal-policy'] })
      },
    }
  )
  const unipay = form.watch('mode') === 'unipay'
  const modeItems = [
    { value: 'manual', label: t('Manual bank transfer') },
    { value: 'unipay', label: t('UniPay Alipay transfer') },
  ]
  const fields = [
    ['cny_per_unit', t('CNY per quota unit')],
    ['min_cents', t('Minimum withdrawal (cents)')],
    ['max_cents', t('Maximum withdrawal (cents)')],
    ...(unipay
      ? ([
          ['gateway', t('Payout gateway URL')],
          ['pid', t('Merchant PID')],
          ['notify_url', t('Payout notification URL')],
          ['scene', t('Alipay transfer scene')],
        ] as const)
      : []),
  ] as const
  return (
    <SettingsSection title={t('Referral withdrawals')}>
      <FormNavigationGuard when={isDirty} />
      <Form {...form}>
        <SettingsForm
          onSubmit={(event) =>
            void handleSubmit(event).catch((error) =>
              toast.error(t(AuthOperationError.from(error).message))
            )
          }
        >
          <SettingsPageFormActions
            onSave={() =>
              void handleSubmit().catch((error) =>
                toast.error(t(AuthOperationError.from(error).message))
              )
            }
            isSaving={isSubmitting}
          />
          <p className='text-muted-foreground text-sm'>
            {unipay
              ? t(
                  'Use the independent UniPay payout key. New withdrawals require review by the root administrator and two-factor or passkey verification.'
                )
              : t(
                  'Users submit bank account details. The root administrator transfers each withdrawal manually, then marks it as paid in the request list.'
                )}
          </p>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Keep CRYPTO_SECRET or SESSION_SECRET stable (at least 32 characters) to decrypt payout settings and recipient data after restart.'
            )}
          </p>
          <SettingsFormGrid>
            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Enable referral withdrawals')}</FormLabel>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={isSubmitting}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Disabling withdrawals stops new payouts; accepted payouts continue to be reconciled.'
                    )}
                  </FormDescription>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='mode'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Payout method')}</FormLabel>
                  <Select
                    items={modeItems}
                    value={field.value}
                    onValueChange={(value) => value && field.onChange(value)}
                    disabled={isSubmitting}
                  >
                    <FormControl>
                      <SelectTrigger className='w-full'>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {modeItems.map((item) => (
                          <SelectItem key={item.value} value={item.value}>
                            {item.label}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t(
                      'Use manual bank transfer when the Alipay transfer product is unavailable.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            {fields.map(([name, label]) => (
              <FormField
                key={name}
                control={form.control}
                name={name}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{label}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        disabled={isSubmitting}
                        type={
                          name === 'min_cents' || name === 'max_cents'
                            ? 'number'
                            : 'text'
                        }
                      />
                    </FormControl>
                    {name === 'notify_url' && (
                      <FormDescription>
                        {t(
                          'Public HTTPS URL ending in /api/user/withdrawals/notify'
                        )}
                      </FormDescription>
                    )}
                    {name === 'cny_per_unit' && (
                      <FormDescription>
                        {t(
                          'CNY paid for one quota unit. This rate is independent of recharge discounts and display currency.'
                        )}
                      </FormDescription>
                    )}
                    <FormMessage />
                  </FormItem>
                )}
              />
            ))}
            {unipay && (
              <>
                <FormField
                  control={form.control}
                  name='api_key'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('UniPay payout API key')}</FormLabel>
                      <FormControl>
                        <PasswordInput
                          {...field}
                          disabled={isSubmitting}
                          autoComplete='new-password'
                        />
                      </FormControl>
                      <FormDescription>
                        {props.keyConfigured
                          ? t(
                              'A payout key is configured. Leave blank to keep it.'
                            )
                          : t('Enter the 64-character payout key from UniPay.')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='scene_infos_json'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('Transfer scene report information (JSON)')}
                      </FormLabel>
                      <FormControl>
                        <Textarea {...field} disabled={isSubmitting} rows={5} />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Use the real scene and information required by your Alipay transfer agreement.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </>
            )}
          </SettingsFormGrid>
        </SettingsForm>
      </Form>
      <SecureVerificationDialog {...verification.dialogProps} />
    </SettingsSection>
  )
}

export function WithdrawalSettings() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['withdrawal-config'],
    queryFn: getWithdrawalConfig,
    retry: false,
  })
  if (query.isPending) return <LoadingState />
  if (query.error) return <ErrorState onRetry={() => void query.refetch()} />
  return (
    <>
      <WithdrawalSettingsForm
        config={query.data.config}
        keyConfigured={query.data.key_configured}
      />
      <SettingsSection title={t('Withdrawal requests')}>
        <WithdrawalHistory admin />
      </SettingsSection>
    </>
  )
}
