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
import { createFileRoute, redirect } from '@tanstack/react-router'
import { z } from 'zod'

import { AppAuthorization } from '@/features/app-authorization'
import { resolveAuthentication } from '@/lib/auth-session'
import { useAuthStore } from '@/stores/auth-store'

// Malformed values become undefined so the page can explain the problem
// instead of failing route validation.
const optionalText = z.string().optional().catch(undefined)
const searchSchema = z.object({
  client_name: optionalText,
  redirect_uri: optionalText,
  code_challenge: optionalText,
  code_challenge_method: optionalText,
  state: optionalText,
  key_name: optionalText,
  scope: optionalText,
})

export const Route = createFileRoute('/(auth)/app-auth')({
  component: AppAuthorization,
  validateSearch: searchSchema,
  beforeLoad: async ({ location }) => {
    // Resolve against the server so a valid refresh cookie is honored before
    // sending the user to sign in (any sign-in method returns here).
    await resolveAuthentication()
    const { auth } = useAuthStore.getState()
    if (!auth.user || !auth.accessToken) {
      throw redirect({ to: '/sign-in', search: { redirect: location.href } })
    }
  },
})
