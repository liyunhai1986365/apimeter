/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Link, useNavigate, useParams } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { PublicLayout } from '@/components/layout'
import { ModelCard } from './components'
import { MODEL_CATEGORIES } from './constants'
import { usePricingData } from './hooks'
import { getSEOCategory } from './seo-categories'

export function PricingCategoryLanding() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { category: categorySlug } = useParams({
    from: '/pricing/categories/$category/',
  })
  const category = getSEOCategory(categorySlug)
  const { models, isLoading, priceRate, usdExchangeRate, groupDisplay } =
    usePricingData()
  const categoryModels = models.filter(
    (model) =>
      (model.category || MODEL_CATEGORIES.TEXT).toLowerCase() === category?.slug
  )

  if (!category) return null

  return (
    <PublicLayout showMainContainer={false}>
      <main className='mx-auto flex w-full max-w-7xl flex-col gap-10 px-4 pt-28 pb-16 sm:px-6 lg:px-8'>
        <header className='ring-border/70 shadow-foreground/5 relative isolate min-h-72 overflow-hidden rounded-2xl shadow-lg ring-1 sm:min-h-80'>
          <img
            src={`/images/model-categories/${category.slug}-minimal.avif`}
            alt=''
            aria-hidden='true'
            decoding='async'
            className='absolute inset-0 -z-10 size-full object-cover object-center'
          />
          <div className='flex min-h-72 flex-col gap-8 p-5 text-white drop-shadow-sm sm:min-h-80 sm:p-8'>
            <Breadcrumb>
              <BreadcrumbList className='text-white/65'>
                <BreadcrumbItem>
                  <BreadcrumbLink
                    className='hover:text-white'
                    render={<Link to='/pricing' />}
                  >
                    {t('Model Price')}
                  </BreadcrumbLink>
                </BreadcrumbItem>
                <BreadcrumbSeparator />
                <BreadcrumbItem>
                  <BreadcrumbPage className='text-white'>
                    {t(category.titleKey)}
                  </BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>

            <div className='flex max-w-4xl flex-col gap-4'>
              <p className='text-sm font-medium text-white/75'>
                {t('Explore AI model APIs by category')}
              </p>
              <h1 className='text-4xl font-bold tracking-tight sm:text-5xl'>
                {t(category.titleKey)}
              </h1>
              <p className='max-w-3xl text-base leading-7 text-white/75 sm:text-lg'>
                {t(category.descriptionKey)}
              </p>
              {!isLoading && (
                <p className='text-sm text-white/65'>
                  {t('{{count}} available models', {
                    count: categoryModels.length,
                  })}
                </p>
              )}
            </div>
          </div>
        </header>

        <section aria-labelledby='category-model-directory'>
          <div className='mb-5 flex items-end justify-between gap-4'>
            <div className='flex flex-col gap-1'>
              <h2
                id='category-model-directory'
                className='text-2xl font-semibold tracking-tight'
              >
                {t('Compare model API pricing and capabilities')}
              </h2>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Open a model page for current prices, endpoints, context limits and related APIs.'
                )}
              </p>
            </div>
            <Button variant='outline' render={<Link to='/pricing' />}>
              {t('All model pricing')}
            </Button>
          </div>

          {isLoading ? (
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
              {Array.from({ length: 6 }).map((_, index) => (
                <Skeleton key={index} className='h-[264px] rounded-lg' />
              ))}
            </div>
          ) : (
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
              {categoryModels.map((model) => (
                <ModelCard
                  key={model.model_name}
                  model={model}
                  priceRate={priceRate}
                  usdExchangeRate={usdExchangeRate}
                  showRechargePrice={false}
                  groupDisplay={groupDisplay}
                  onClick={() =>
                    navigate({
                      to: '/pricing/$modelId',
                      params: { modelId: model.model_name },
                    })
                  }
                />
              ))}
            </div>
          )}
        </section>
      </main>
    </PublicLayout>
  )
}
