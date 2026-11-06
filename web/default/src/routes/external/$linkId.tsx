/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import {
  createFileRoute,
  Link,
  notFound,
  redirect,
} from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { getStatus } from '@/lib/api'
import {
  getEmbeddedHeaderNavLink,
  parseHeaderNavModulesFromStatus,
} from '@/lib/nav-modules'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'

export const Route = createFileRoute('/external/$linkId')({
  beforeLoad: async ({ params, location }) => {
    const status = await getStatus()
    const link = getEmbeddedHeaderNavLink(
      parseHeaderNavModulesFromStatus(status),
      params.linkId,
      window.location.origin
    )
    if (!link) throw notFound()
    if (link.requireAuth && !useAuthStore.getState().auth.user) {
      throw redirect({
        to: '/sign-in',
        search: { redirect: location.href },
      })
    }
    return { embeddedLink: link }
  },
  component: ExternalPage,
})

function ExternalPage() {
  const { t } = useTranslation()
  const { embeddedLink } = Route.useRouteContext()
  const isMixedContent =
    window.location.protocol === 'https:' &&
    new URL(embeddedLink.href).protocol === 'http:'

  return (
    <main
      aria-label={embeddedLink.title}
      className='bg-background text-foreground flex h-dvh w-full flex-col overflow-hidden'
    >
      <Button
        variant='secondary'
        size='sm'
        className='fixed top-3 left-3 z-10 shadow-sm'
        render={<Link to='/' />}
        nativeButton={false}
      >
        {t('Back to Home')}
      </Button>
      {isMixedContent ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>{t('Unable to embed this page')}</EmptyTitle>
            <EmptyDescription>
              {t(
                'This HTTPS site cannot embed an HTTP page. Configure an HTTPS URL.'
              )}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <iframe
          key={embeddedLink.href}
          src={embeddedLink.href}
          title={embeddedLink.title}
          className='min-h-0 w-full flex-1 border-0'
          referrerPolicy='no-referrer'
          sandbox='allow-forms allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox allow-modals allow-downloads'
          allowFullScreen
        />
      )}
    </main>
  )
}
