/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  new URL('../src/routes/_authenticated/ai-creation.tsx', import.meta.url),
  'utf8'
)

describe('AI Creation iframe route', () => {
  it('gets a short-lived same-origin ticket before loading OpenMosaic', () => {
    expect(source).toContain('/api/integrations/openmosaic/embedded-authorize')
    expect(source).toContain('authorizationRequestRef')
    expect(source).toContain('const data = await pending.promise')
    expect(source).toContain("url.searchParams.set('code', data.code)")
    expect(source).toContain("url.searchParams.set('site_origin', data.site_origin)")
    expect(source).not.toContain('OPENMOSAIC_SSO_CLIENT_SECRET')
  })

  it('renders OpenMosaic as the authenticated page content', () => {
    expect(source).toContain("title={t('AI Creation')}")
    expect(source).toContain("className='h-full w-full border-0 bg-background'")
  })
})
