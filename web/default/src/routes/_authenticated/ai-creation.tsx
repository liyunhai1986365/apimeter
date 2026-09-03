/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useMemo, useRef, useState } from 'react'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { normalizeInterfaceLanguage } from '@/i18n/languages'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'
import {
  normalizeOpenMosaicBaseUrl,
  parseHeaderNavModulesFromStatus,
} from '@/lib/nav-modules'
import { buildOpenMosaicEmbedUrl } from '@/lib/openmosaic-language'

type EmbeddedAuthorization = {
  code: string
  site_origin: string
}

export const Route = createFileRoute('/_authenticated/ai-creation')({
  beforeLoad: () => {
    const status = readCachedStatus()
    const config = parseHeaderNavModulesFromStatus(status).aiCreation
    if (!config.enabled || !config.baseUrl) throw redirect({ to: '/dashboard' })
  },
  component: AICreationPage,
})

function readCachedStatus(): Record<string, unknown> | null {
  try {
    const raw = window.localStorage.getItem('status')
    return raw ? (JSON.parse(raw) as Record<string, unknown>) : null
  } catch {
    return null
  }
}

function AICreationPage() {
  const { t, i18n } = useTranslation()
  const [src, setSrc] = useState('')
  const [error, setError] = useState('')
  const authorizationRequestRef = useRef<{
    origin: string
    language: string
    promise: Promise<EmbeddedAuthorization>
  } | null>(null)
  const status = useMemo(() => readCachedStatus(), [])
  const openMosaicOrigin = normalizeOpenMosaicBaseUrl(
    parseHeaderNavModulesFromStatus(status).aiCreation.baseUrl
  )
  const interfaceLanguage = normalizeInterfaceLanguage(
    i18n.resolvedLanguage || i18n.language
  )

  useEffect(() => {
    let cancelled = false
    async function authorize() {
      let pending: (typeof authorizationRequestRef)['current'] = null
      try {
        if (
          !authorizationRequestRef.current ||
          authorizationRequestRef.current.origin !== openMosaicOrigin ||
          authorizationRequestRef.current.language !== interfaceLanguage
        ) {
          authorizationRequestRef.current = {
            origin: openMosaicOrigin,
            language: interfaceLanguage,
            promise: api
              .post(
                '/api/integrations/openmosaic/embedded-authorize',
                {
                  redirect_uri: `${openMosaicOrigin}/auth/modelsell/callback`,
                  site_origin: window.location.origin,
                },
                { headers: { 'Cache-Control': 'no-store' } }
              )
              .then((response) => {
                const data = response.data?.data as EmbeddedAuthorization
                if (!data?.code || !data.site_origin)
                  throw new Error(
                    t('AI Creation authorization response is incomplete.')
                  )
                return data
              }),
          }
        }
        pending = authorizationRequestRef.current
        const data = await pending.promise
        const url = buildOpenMosaicEmbedUrl(
          openMosaicOrigin,
          data,
          interfaceLanguage
        )
        if (!cancelled) setSrc(url)
      } catch (cause) {
        if (
          pending &&
          authorizationRequestRef.current?.promise === pending.promise
        ) {
          authorizationRequestRef.current = null
        }
        if (!cancelled)
          setError(
            cause instanceof Error
              ? cause.message
              : t('AI Creation authorization failed.')
          )
      }
    }
    if (openMosaicOrigin) void authorize()
    return () => {
      cancelled = true
    }
  }, [interfaceLanguage, openMosaicOrigin, t])

  const displayError =
    error ||
    (!openMosaicOrigin ? t('OpenMosaic site URL is not configured.') : '')
  if (displayError) {
    return (
      <main className='text-destructive flex h-full items-center justify-center p-6 text-sm'>
        {displayError}
      </main>
    )
  }
  if (!src) {
    return (
      <main className='text-muted-foreground flex h-full items-center justify-center gap-2 text-sm'>
        <LoaderCircle className='size-4 animate-spin' />
        {t('Securely entering AI Creation...')}
      </main>
    )
  }
  return (
    <iframe
      title={t('AI Creation')}
      src={src}
      className='bg-background h-full w-full border-0'
      allow='clipboard-read; clipboard-write; microphone; camera'
      referrerPolicy='origin'
    />
  )
}
