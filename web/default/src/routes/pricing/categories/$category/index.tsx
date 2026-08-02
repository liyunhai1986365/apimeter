/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { createFileRoute, notFound, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { getFreshModuleAccess } from '@/lib/nav-modules'
import { PricingCategoryLanding } from '@/features/pricing/category-landing'
import { isSEOCategory } from '@/features/pricing/seo-categories'

export const Route = createFileRoute('/pricing/categories/$category/')({
  beforeLoad: async ({ location, params }) => {
    if (!isSEOCategory(params.category)) {
      throw notFound()
    }
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
  component: PricingCategoryLanding,
})
