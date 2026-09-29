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
import { useQuery } from '@tanstack/react-query'
import { KeyRound, Loader2, MonitorSmartphone, UserRound } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
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
  Item,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from '@/components/ui/item'
import {
  ApiKeyGroupCombobox,
  type ApiKeyGroupOption,
} from '@/features/keys/components/api-key-group-combobox'
import { getUserGroups } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError, requireServerSuccess } from '@/lib/server-error-message'
import type { AuthUser } from '@/stores/auth-store'

import { authorizeApp } from '../api'
import {
  API_KEY_NAME_MAX_BYTES,
  type AppAuthorizationRequest,
  buildDeniedRedirect,
  utf8ByteLength,
} from '../lib/request'

type AppAuthorizationFormProps = {
  request: AppAuthorizationRequest
  siteName: string
  user: AuthUser
  defaultUseAutoGroup: boolean
  onRedirect: (outcome: 'approved' | 'denied', url: string) => void
}

export function AppAuthorizationForm(props: AppAuthorizationFormProps) {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)

  const groupsQuery = useQuery({
    queryKey: ['user-groups'],
    queryFn: async () => requireServerSuccess(await getUserGroups()),
  })
  const groups = useMemo<ApiKeyGroupOption[]>(
    () =>
      Object.entries(groupsQuery.data?.data || {}).map(([key, info]) => ({
        value: key,
        label: key,
        desc: info.desc || key,
        ratio: info.ratio,
        baseRatio: info.base_ratio,
        customRatio: info.custom_ratio === true,
      })),
    [groupsQuery.data]
  )
  const userRatio =
    typeof groupsQuery.data?.user_ratio === 'number' &&
    groupsQuery.data.user_ratio > 0
      ? groupsQuery.data.user_ratio
      : undefined

  const schema = useMemo(
    () =>
      z.object({
        name: z
          .string()
          .trim()
          .min(1, t('Please enter a name'))
          .refine((value) => utf8ByteLength(value) <= API_KEY_NAME_MAX_BYTES, {
            message: t('The name is too long'),
          }),
        group: z.string(),
      }),
    [t]
  )
  type Values = z.infer<typeof schema>
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { name: props.request.keyName, group: '' },
  })

  // Pick the same default group as the API key drawer once groups load.
  useEffect(() => {
    if (groups.length === 0 || form.getFieldState('group').isDirty) return
    const preferAuto =
      props.defaultUseAutoGroup && groups.some((g) => g.value === 'auto')
    const fallback =
      groups.find((g) => g.value === props.user.group)?.value ??
      groups.find((g) => g.value === 'default')?.value ??
      groups[0]?.value ??
      ''
    form.setValue('group', preferAuto ? 'auto' : fallback)
  }, [groups, form, props.defaultUseAutoGroup, props.user.group])

  async function onSubmit(values: Values) {
    setSubmitting(true)
    try {
      const res = await authorizeApp({
        client_name: props.request.clientName,
        redirect_uri: props.request.redirectUri,
        code_challenge: props.request.codeChallenge,
        code_challenge_method: 'S256',
        state: props.request.state,
        token: {
          name: values.name,
          group: values.group,
          unlimited_quota: true,
          remain_quota: 0,
          expired_time: -1,
          model_limits_enabled: false,
          model_limits: '',
          allow_ips: '',
          cross_group_retry: values.group === 'auto',
        },
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
          <KeyRound className='h-8 w-8' />
        </div>
        <div className='space-y-2'>
          <h2 className='text-2xl font-semibold tracking-tight'>
            {t('Authorize {{app}}', { app: props.request.clientName })}
          </h2>
          <p className='text-muted-foreground text-sm sm:text-base'>
            {t('{{app}} wants to create an API key for your {{site}} account.', {
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
            <ItemTitle>{t('Signed in as {{name}}', { name: accountName })}</ItemTitle>
            <ItemDescription>
              {t('The new API key uses the balance of this account.')}
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
              {t('The key is sent only to {{address}} on this computer.', {
                address: props.request.redirectHost,
              })}
            </ItemDescription>
          </ItemContent>
        </Item>
      </ItemGroup>

      <Alert>
        <AlertDescription>
          {t(
            'The app name is provided by the app itself and is not verified. Only continue if you just started this sign-in. You can delete the key on the API Keys page at any time.'
          )}
        </AlertDescription>
      </Alert>

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className='grid gap-4'>
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('API key name')}</FormLabel>
                <FormControl>
                  <Input autoComplete='off' {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='group'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Group')}</FormLabel>
                <FormControl>
                  <ApiKeyGroupCombobox
                    options={groups}
                    value={field.value}
                    userRatio={userRatio}
                    onValueChange={(group) =>
                      form.setValue('group', group, { shouldDirty: true })
                    }
                    placeholder={t('Select a group')}
                    disabled={submitting || groupsQuery.isLoading}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'The key has unlimited quota and never expires. You can change this later on the API Keys page.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <div className='grid grid-cols-2 gap-3 pt-2'>
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
            <Button type='submit' disabled={submitting}>
              {submitting && <Loader2 className='animate-spin' />}
              {t('Authorize')}
            </Button>
          </div>
        </form>
      </Form>
    </div>
  )
}
