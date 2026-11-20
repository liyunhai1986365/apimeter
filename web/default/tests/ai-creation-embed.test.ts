/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { buildOpenMosaicEmbedUrl } from '../src/lib/openmosaic-language'

const source = readFileSync(
  new URL('../src/routes/_authenticated/ai-creation.tsx', import.meta.url),
  'utf8'
)

describe('AI Creation iframe route', () => {
  it('gets a short-lived same-origin ticket before loading OpenMosaic', () => {
    expect(source).toContain('/api/integrations/openmosaic/embedded-authorize')
    expect(source).toContain('authorizationRequestRef')
    expect(source).toContain('const data = await pending.promise')
    expect(source).toMatch(
      /buildOpenMosaicEmbedUrl\(\s*openMosaicOrigin,\s*data,\s*interfaceLanguage\s*\)/
    )
    expect(source).not.toContain('OPENMOSAIC_SSO_CLIENT_SECRET')

    const authorization = {
      code: 'one-time+code/with?reserved&characters=',
      site_origin: 'https://api.example.com:8443',
    }
    const url = new URL(
      buildOpenMosaicEmbedUrl(
        'https://creation.example.com',
        authorization,
        'zh-CN'
      )
    )
    expect(url.origin).toBe('https://creation.example.com')
    expect(url.pathname).toBe('/auth/modelsell/embed')
    expect(Object.fromEntries(url.searchParams)).toEqual({
      ...authorization,
      redirect: '/image',
      lang: 'zhCN',
    })
  })

  it('renders OpenMosaic as the authenticated page content', () => {
    const iframe = source.match(/<iframe\b[^>]*\/>/)?.[0]
    expect(iframe).toContain("title={t('AI Creation')}")
    expect(iframe).toContain('src={src}')
    const classes = iframe?.match(/className=['"]([^'"]+)['"]/)?.[1]
    expect(new Set(classes?.split(/\s+/))).toEqual(
      new Set(['h-full', 'w-full', 'border-0', 'bg-background'])
    )
  })
})
