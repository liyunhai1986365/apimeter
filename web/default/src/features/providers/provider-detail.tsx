/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useMemo } from 'react'
import { Link, useParams } from '@tanstack/react-router'
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
import { Skeleton } from '@/components/ui/skeleton'
import { PublicLayout } from '@/components/layout'
import { NotFoundError } from '@/features/errors/not-found-error'
import { usePricingData } from '@/features/pricing/hooks'
import { getSEOCategory } from '@/features/pricing/seo-categories'
import {
  buildProviderDirectory,
  getProviderModelCategory,
} from './lib/provider-directory'
import { ProviderLogo } from './provider-logo'

export function ProviderDetail() {
  const { t } = useTranslation()
  const { providerSlug } = useParams({ from: '/providers/$providerSlug/' })
  const { models, vendors, isLoading } = usePricingData()
  const providers = useMemo(
    () => buildProviderDirectory(models, vendors),
    [models, vendors]
  )
  const provider = providers.find((item) => item.slug === providerSlug)

  if (isLoading) return <ProviderDetailSkeleton />
  if (!provider) return <NotFoundError />

  return (
    <PublicLayout showMainContainer={false}>
      <main className='mx-auto flex w-full max-w-7xl flex-col gap-10 px-4 pt-28 pb-16 sm:px-6 lg:px-8'>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink render={<Link to='/providers' />}>
                {t('AI model providers')}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{provider.vendor.name}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>

        <header className='grid gap-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end'>
          <div className='flex max-w-4xl flex-col gap-5'>
            <ProviderLogo
              name={provider.vendor.name}
              icon={provider.vendor.icon}
              size={32}
            />
            <div className='flex flex-col gap-3'>
              <p className='text-primary text-sm font-medium'>
                {t('Provider model directory')}
              </p>
              <h1 className='text-4xl font-bold tracking-tight sm:text-5xl'>
                {t('{{provider}} AI models and APIs', {
                  provider: provider.vendor.name,
                })}
              </h1>
              <p className='text-muted-foreground text-base leading-7 sm:text-lg'>
                {provider.vendor.description ||
                  t(
                    'Compare this provider’s available models, supported API types and current pricing.'
                  )}
              </p>
            </div>
            <div className='flex flex-wrap gap-2'>
              {provider.categories.map((category) => (
                <Badge
                  key={category}
                  variant='outline'
                  render={
                    <Link
                      to='/pricing/categories/$category'
                      params={{ category }}
                    />
                  }
                >
                  {t(getSEOCategory(category)?.titleKey || category)}
                </Badge>
              ))}
            </div>
          </div>
          <div className='flex flex-col gap-3 sm:flex-row lg:flex-col'>
            <Button
              render={
                <Link to='/pricing' search={{ vendor: provider.vendor.name }} />
              }
            >
              {t('Compare provider pricing')}
              <HugeiconsIcon icon={ArrowRight02Icon} data-icon='inline-end' />
            </Button>
            <Button variant='outline' render={<Link to='/providers' />}>
              {t('All providers')}
            </Button>
          </div>
        </header>

        <section
          className='grid gap-4 sm:grid-cols-3'
          aria-label={t('Provider summary')}
        >
          <SummaryCard
            label={t('Available models')}
            value={provider.models.length}
          />
          <SummaryCard
            label={t('Model categories')}
            value={provider.categories.length}
          />
          <SummaryCard
            label={t('Supported API types')}
            value={provider.endpointTypes.length}
          />
        </section>

        {provider.endpointTypes.length > 0 && (
          <section
            className='flex flex-col gap-3'
            aria-labelledby='provider-api-types'
          >
            <h2 id='provider-api-types' className='text-xl font-semibold'>
              {t('Supported API types')}
            </h2>
            <div className='flex flex-wrap gap-2'>
              {provider.endpointTypes.map((endpoint) => (
                <Badge key={endpoint} variant='secondary'>
                  {endpoint}
                </Badge>
              ))}
            </div>
          </section>
        )}

        <section aria-labelledby='provider-models'>
          <div className='mb-5 flex flex-col gap-1'>
            <h2
              id='provider-models'
              className='text-2xl font-semibold tracking-tight'
            >
              {t('{{provider}} model APIs', {
                provider: provider.vendor.name,
              })}
            </h2>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Open a model page for current prices, endpoints and capabilities.'
              )}
            </p>
          </div>
          <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
            {provider.models.map((model) => (
              <Card key={model.model_name} className='h-full'>
                <CardHeader>
                  <CardTitle className='font-mono text-base break-all'>
                    {model.model_name}
                  </CardTitle>
                  <CardDescription className='line-clamp-3'>
                    {model.description ||
                      t('View current model pricing and API access details.')}
                  </CardDescription>
                </CardHeader>
                <CardContent className='flex flex-wrap gap-2'>
                  <Badge variant='outline'>
                    {t(
                      getSEOCategory(getProviderModelCategory(model))
                        ?.titleKey || 'Specialized AI model APIs'
                    )}
                  </Badge>
                  {(model.supported_endpoint_types ?? [])
                    .slice(0, 2)
                    .map((endpoint) => (
                      <Badge key={endpoint} variant='secondary'>
                        {endpoint}
                      </Badge>
                    ))}
                </CardContent>
                <CardFooter>
                  <Button
                    variant='link'
                    render={
                      <Link
                        to='/pricing/$modelId'
                        params={{ modelId: model.model_name }}
                      />
                    }
                  >
                    {t('View API pricing')}
                    <HugeiconsIcon
                      icon={ArrowRight02Icon}
                      data-icon='inline-end'
                    />
                  </Button>
                </CardFooter>
              </Card>
            ))}
          </div>
        </section>
      </main>
    </PublicLayout>
  )
}

function SummaryCard(props: { label: string; value: number }) {
  return (
    <Card size='sm'>
      <CardHeader>
        <CardDescription>{props.label}</CardDescription>
        <CardTitle className='text-3xl tabular-nums'>{props.value}</CardTitle>
      </CardHeader>
    </Card>
  )
}

function ProviderDetailSkeleton() {
  return (
    <PublicLayout showMainContainer={false}>
      <main className='mx-auto flex w-full max-w-7xl flex-col gap-8 px-4 pt-28 pb-16 sm:px-6 lg:px-8'>
        <Skeleton className='h-5 w-64' />
        <Skeleton className='h-48 w-full max-w-4xl rounded-xl' />
        <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
          {Array.from({ length: 6 }).map((_, index) => (
            <Skeleton key={index} className='h-52 rounded-xl' />
          ))}
        </div>
      </main>
    </PublicLayout>
  )
}
