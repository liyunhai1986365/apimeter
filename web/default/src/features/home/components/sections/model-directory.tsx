/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Link } from '@tanstack/react-router'
import { ArrowRight02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { SEO_CATEGORIES } from '@/features/pricing/seo-categories'

export function ModelDirectory() {
  const { t } = useTranslation()

  return (
    <section
      aria-labelledby='model-api-directory'
      className='border-border/40 border-y px-6 py-16 md:py-24'
    >
      <div className='mx-auto flex max-w-6xl flex-col gap-10'>
        <div className='flex max-w-3xl flex-col gap-3'>
          <p className='text-primary text-sm font-medium'>
            {t('Explore AI model APIs by category')}
          </p>
          <h2
            id='model-api-directory'
            className='text-3xl font-bold tracking-tight md:text-4xl'
          >
            {t('Find the right API for every AI workload')}
          </h2>
          <p className='text-muted-foreground leading-7'>
            {t(
              'Browse dedicated model directories with current pricing, supported endpoints, capabilities and direct links to every available API.'
            )}
          </p>
        </div>

        <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
          {SEO_CATEGORIES.map((category) => (
            <Card
              key={category.slug}
              className='group ring-border/70 shadow-foreground/5 hover:ring-foreground/20 hover:shadow-foreground/10 relative isolate min-h-[17rem] gap-0 py-0 shadow-sm transition-[transform,box-shadow] duration-300 ease-out hover:-translate-y-0.5 hover:shadow-lg motion-reduce:transition-none'
            >
              <img
                src={`/images/model-categories/${category.slug}-minimal.avif`}
                alt=''
                aria-hidden='true'
                loading='lazy'
                decoding='async'
                className='absolute inset-0 -z-10 size-full object-cover object-bottom transition-transform duration-700 ease-out group-hover:scale-[1.02] motion-reduce:transition-none'
              />
              <CardHeader className='relative pt-5 text-white drop-shadow-sm'>
                <CardTitle>{t(category.titleKey)}</CardTitle>
                <CardDescription className='line-clamp-3 leading-6 text-white/70'>
                  {t(category.descriptionKey)}
                </CardDescription>
              </CardHeader>
              <CardFooter className='relative border-0 bg-transparent pt-0'>
                <Button
                  variant='link'
                  className='text-white drop-shadow-sm hover:text-white'
                  render={
                    <Link
                      to='/pricing/categories/$category'
                      params={{ category: category.slug }}
                    />
                  }
                >
                  {t('Browse category')}
                  <HugeiconsIcon
                    icon={ArrowRight02Icon}
                    data-icon='inline-end'
                  />
                </Button>
              </CardFooter>
            </Card>
          ))}
        </div>

        <Card>
          <CardHeader>
            <CardTitle>{t('Browse by AI provider')}</CardTitle>
            <CardDescription>
              {t(
                'Compare every available provider, its model catalog and supported API types in one directory.'
              )}
            </CardDescription>
          </CardHeader>
          <CardFooter>
            <Button variant='outline' render={<Link to='/providers' />}>
              {t('Explore AI providers')}
              <HugeiconsIcon icon={ArrowRight02Icon} data-icon='inline-end' />
            </Button>
          </CardFooter>
        </Card>
      </div>
    </section>
  )
}
