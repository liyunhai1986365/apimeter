/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { ArrowRight02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { PublicLayout } from '@/components/layout'
import { usePricingData } from '@/features/pricing/hooks'
import { getSEOCategory } from '@/features/pricing/seo-categories'
import {
  buildProviderDirectory,
  filterProviderDirectory,
} from './lib/provider-directory'
import { ProviderLogo } from './provider-logo'

export function ProviderDirectory() {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const { models, vendors, isLoading } = usePricingData()
  const providers = useMemo(
    () => buildProviderDirectory(models, vendors),
    [models, vendors]
  )
  const filteredProviders = useMemo(
    () => filterProviderDirectory(providers, search),
    [providers, search]
  )
  const endpointCount = useMemo(
    () => new Set(providers.flatMap((provider) => provider.endpointTypes)).size,
    [providers]
  )

  return (
    <PublicLayout showMainContainer={false}>
      <main className='mx-auto flex w-full max-w-7xl flex-col gap-10 px-4 pt-28 pb-16 sm:px-6 lg:px-8'>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink render={<Link to='/' />}>
                {t('Home')}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{t('AI model providers')}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>

        <header className='grid gap-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end'>
          <div className='flex max-w-4xl flex-col gap-4'>
            <p className='text-primary text-sm font-medium'>
              {t('One API, multiple AI providers')}
            </p>
            <h1 className='text-4xl font-bold tracking-tight sm:text-5xl'>
              {t('AI model providers and APIs')}
            </h1>
            <p className='text-muted-foreground text-base leading-7 sm:text-lg'>
              {t(
                'Compare available AI providers, discover their models, supported API types and current pricing through one unified gateway.'
              )}
            </p>
          </div>
          {!isLoading && (
            <div className='grid grid-cols-3 gap-3'>
              <DirectoryMetric
                label={t('Providers')}
                value={providers.length}
              />
              <DirectoryMetric label={t('Models')} value={models.length} />
              <DirectoryMetric label={t('API types')} value={endpointCount} />
            </div>
          )}
        </header>

        <section aria-labelledby='provider-directory-heading'>
          <div className='mb-5 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between'>
            <div className='flex flex-col gap-1'>
              <h2
                id='provider-directory-heading'
                className='text-2xl font-semibold tracking-tight'
              >
                {t('Browse all available providers')}
              </h2>
              <p className='text-muted-foreground text-sm'>
                {t('Open a provider page to compare its available model APIs.')}
              </p>
            </div>
            <Input
              type='search'
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={t('Search providers or models')}
              aria-label={t('Search providers or models')}
              className='w-full sm:max-w-sm'
            />
          </div>

          {isLoading ? (
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
              {Array.from({ length: 9 }).map((_, index) => (
                <Skeleton key={index} className='h-72 rounded-xl' />
              ))}
            </div>
          ) : filteredProviders.length === 0 ? (
            <Empty className='min-h-72 border'>
              <EmptyHeader>
                <EmptyTitle>{t('No providers found')}</EmptyTitle>
                <EmptyDescription>
                  {t('Try another provider name or model ID.')}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
              {filteredProviders.map((provider) => (
                <Card key={provider.vendor.id} className='h-full'>
                  <CardHeader>
                    <div className='mb-3 flex items-start justify-between gap-3'>
                      <ProviderLogo
                        name={provider.vendor.name}
                        icon={provider.vendor.icon}
                      />
                      <Badge variant='secondary'>
                        {t('{{count}} models', {
                          count: provider.models.length,
                        })}
                      </Badge>
                    </div>
                    <CardTitle>{provider.vendor.name}</CardTitle>
                    <CardDescription className='line-clamp-4'>
                      {provider.vendor.description ||
                        t(
                          'Explore models and APIs available from this provider.'
                        )}
                    </CardDescription>
                  </CardHeader>
                  <CardContent className='flex flex-col gap-4'>
                    <div className='flex flex-wrap gap-2'>
                      {provider.categories.slice(0, 4).map((category) => (
                        <Badge key={category} variant='outline'>
                          {t(getSEOCategory(category)?.titleKey || category)}
                        </Badge>
                      ))}
                    </div>
                    <div className='flex flex-col gap-1.5'>
                      {provider.models.slice(0, 3).map((model) => (
                        <code
                          key={model.model_name}
                          className='text-muted-foreground truncate text-xs'
                        >
                          {model.model_name}
                        </code>
                      ))}
                    </div>
                  </CardContent>
                  <CardFooter>
                    <Button
                      variant='link'
                      render={
                        <Link
                          to='/providers/$providerSlug'
                          params={{ providerSlug: provider.slug }}
                        />
                      }
                    >
                      {t('View provider APIs')}
                      <HugeiconsIcon
                        icon={ArrowRight02Icon}
                        data-icon='inline-end'
                      />
                    </Button>
                  </CardFooter>
                </Card>
              ))}
            </div>
          )}
        </section>
      </main>
    </PublicLayout>
  )
}

function DirectoryMetric(props: { label: string; value: number }) {
  return (
    <div className='bg-muted/40 min-w-24 rounded-xl p-4 text-center'>
      <p className='text-2xl font-semibold tabular-nums'>{props.value}</p>
      <p className='text-muted-foreground mt-1 text-xs'>{props.label}</p>
    </div>
  )
}
