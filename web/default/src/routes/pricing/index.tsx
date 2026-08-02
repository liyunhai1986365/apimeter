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
import z from 'zod'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { getFreshModuleAccess } from '@/lib/nav-modules'
import { Pricing } from '@/features/pricing'

const semSearchValueSchema = z
  .union([z.string(), z.number(), z.boolean()])
  .optional()

const pricingSearchSchema = z.object({
  search: z.string().optional(),
  sort: z.string().optional(),
  vendor: z.string().optional(),
  group: z.string().optional(),
  quotaType: z.string().optional(),
  endpointType: z.string().optional(),
  category: z.string().optional(),
  tag: z.string().optional(),
  inputModality: z.string().optional(),
  outputModality: z.string().optional(),
  tokenUnit: z.enum(['M', 'K']).optional(),
  view: z.enum(['card', 'table']).optional().catch(undefined),
  rechargePrice: z.boolean().optional(),
  sem: z.union([z.literal('1'), z.literal(1)]).optional(),
  utm_source: semSearchValueSchema,
  utm_medium: semSearchValueSchema,
  utm_campaign: semSearchValueSchema,
  utm_content: semSearchValueSchema,
  utm_term: semSearchValueSchema,
  campaign_id: semSearchValueSchema,
  adgroup_id: semSearchValueSchema,
  creative_id: semSearchValueSchema,
  match_type: semSearchValueSchema,
  network: semSearchValueSchema,
  device: semSearchValueSchema,
  gclid: semSearchValueSchema,
  gbraid: semSearchValueSchema,
  wbraid: semSearchValueSchema,
})

export const Route = createFileRoute('/pricing/')({
  validateSearch: pricingSearchSchema,
  beforeLoad: async ({ location }) => {
    const access = await getFreshModuleAccess('pricing')
    if (!access.enabled) {
      throw redirect({ to: '/' })
    }
    if (access.requireAuth) {
      const { auth } = useAuthStore.getState()
      if (!auth.user) {
        throw redirect({
          to: '/sign-in',
          search: { redirect: location.href },
        })
      }
    }
  },
  component: Pricing,
})
