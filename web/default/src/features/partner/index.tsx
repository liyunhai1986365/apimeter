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
import { Link } from '@tanstack/react-router'
import {
  ArrowRight01Icon,
  BarChartIcon,
  Briefcase01Icon,
  CheckmarkCircle02Icon,
  CodeIcon,
  CustomerSupportIcon,
  GiftIcon,
  GlobalIcon,
  Link01Icon,
  Megaphone01Icon,
  MoneyReceiveCircleIcon,
  PaintBrush02Icon,
  Rocket02Icon,
  Share01Icon,
  Shield01Icon,
  SparklesIcon,
  StudentIcon,
  UserMultiple02Icon,
  WorkIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon, type IconSvgElement } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { cn } from '@/lib/utils'
import { useOpenCustomerService } from '@/hooks/use-open-customer-service'
import { useStatus } from '@/hooks/use-status'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import {
  formatInviteRewardRatio,
  getInviteRewardConfig,
} from '@/features/invite/lib/reward-config'

type IconContent = IconSvgElement

type FeatureItem = {
  icon: IconContent
  title: string
  description: string
}

const BENEFITS: FeatureItem[] = [
  {
    icon: Rocket02Icon,
    title: 'Start without model inventory',
    description:
      'The platform handles model access and technical operations, so you can focus on users and growth.',
  },
  {
    icon: PaintBrush02Icon,
    title: 'Build with your own identity',
    description:
      'Brand partners can manage domains, brand presentation, pricing, and user groups from one console.',
  },
  {
    icon: BarChartIcon,
    title: 'See every result clearly',
    description:
      'Follow invited users, reward records, usage, profit, and withdrawals with traceable data.',
  },
  {
    icon: Shield01Icon,
    title: 'Grow on a stable foundation',
    description:
      'Shared infrastructure and ongoing platform updates reduce the operational burden of running an AI API business.',
  },
]

const AUDIENCES: FeatureItem[] = [
  {
    icon: Megaphone01Icon,
    title: 'Creators and community leaders',
    description:
      'Turn trusted content, audiences, and communities into a sustainable referral channel.',
  },
  {
    icon: CodeIcon,
    title: 'Developers and project owners',
    description:
      'Give users a reliable model service while adding a new revenue stream to your product.',
  },
  {
    icon: Briefcase01Icon,
    title: 'Teams with customer resources',
    description:
      'Use your sales network and industry experience to serve customers with clearer pricing and support.',
  },
  {
    icon: StudentIcon,
    title: 'Independent builders',
    description:
      'Begin with a referral link, learn what your users need, and upgrade your partnership as you grow.',
  },
]

const FAQS = [
  {
    question: 'What is the difference between the two partner paths?',
    answer:
      'Referral partners share an invite link and earn under the active reward policy. Brand partners receive additional agent capabilities such as domain binding, branding, pricing, user management, and business analytics.',
  },
  {
    question: 'How are referral rewards calculated?',
    answer:
      'Rewards follow the current platform policy and may include registration, top-up, or net-consumption rewards. Your partner center shows the effective rules and each reward record.',
  },
  {
    question: 'When can I use or withdraw my rewards?',
    answer:
      'Available rewards can be transferred to your account balance or submitted for withdrawal when they meet the minimum amount and account requirements shown in the partner center.',
  },
  {
    question: 'How do I become a brand partner?',
    answer:
      'Create an account first, then contact the platform operator to discuss your audience, service plan, domain, and settlement needs. Brand partner access is enabled after review.',
  },
  {
    question: 'Can brand partners set their own prices?',
    answer:
      'Brand partners can configure customer-facing pricing and group rules within the limits set by the platform, while the system records usage, cost, and profit separately.',
  },
  {
    question: 'Do I need to maintain upstream model channels?',
    answer:
      'No. The platform maintains upstream channels and core infrastructure. Partners focus on customer acquisition, service, positioning, and operations.',
  },
]

export function PartnerProgram() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const { auth } = useAuthStore()
  const openCustomerService = useOpenCustomerService()
  const config = getInviteRewardConfig(status)
  const isAuthenticated = Boolean(auth.user)
  const topUpReward =
    config.topupRewardRatio > 0
      ? formatInviteRewardRatio(config.topupRewardRatio)
      : t('Configured by admin')
  const consumptionReward =
    config.consumeRewardRatio > 0
      ? formatInviteRewardRatio(config.consumeRewardRatio)
      : t('Configured by admin')

  return (
    <PublicLayout showMainContainer={false}>
      <main className='bg-background overflow-hidden'>
        <section className='relative isolate px-4 pt-[calc(var(--app-header-height)+var(--invite-promo-banner-height)+var(--system-notice-banner-height)+4.5rem)] pb-20 sm:px-6 sm:pb-24'>
          <div className='bg-primary/10 absolute top-12 left-[8%] -z-10 size-80 rounded-full blur-3xl' />
          <div className='bg-primary/5 absolute top-28 right-[4%] -z-10 size-96 rounded-full blur-3xl' />
          <div className='border-border/60 absolute inset-x-0 top-[calc(var(--app-header-height)+var(--invite-promo-banner-height)+var(--system-notice-banner-height))] -z-10 border-t' />

          <div className='mx-auto grid max-w-7xl gap-14 lg:grid-cols-[minmax(0,1.02fr)_minmax(380px,0.78fr)] lg:items-center'>
            <div className='max-w-3xl'>
              <Badge
                variant='secondary'
                className='mb-6 rounded-full px-3 py-1'
              >
                <HugeiconsIcon icon={SparklesIcon} />
                {t('Partner Program')}
              </Badge>
              <h1 className='text-5xl leading-[1.02] font-semibold tracking-tight text-balance sm:text-6xl lg:text-7xl'>
                {t('Build your AI business,')}{' '}
                <span className='text-primary'>
                  {t('share long-term value.')}
                </span>
              </h1>
              <p className='text-muted-foreground mt-6 max-w-2xl text-lg leading-8 sm:text-xl'>
                {t(
                  'Start with one invite link or build a branded API business. We provide the model supply, infrastructure, settlement, and management tools you need to grow.'
                )}
              </p>
              <div className='mt-8 flex flex-col gap-3 sm:flex-row sm:flex-wrap'>
                <PartnerPrimaryLink isAuthenticated={isAuthenticated}>
                  {isAuthenticated
                    ? t('Open partner center')
                    : t('Start as a partner')}
                </PartnerPrimaryLink>
                <a
                  href='#partner-paths'
                  className={buttonVariants({
                    variant: 'outline',
                    size: 'lg',
                    className: 'h-11 rounded-full px-5',
                  })}
                >
                  {t('Explore partnership paths')}
                </a>
                <Button
                  type='button'
                  variant='outline'
                  size='lg'
                  className='h-11 rounded-full px-5'
                  onClick={openCustomerService}
                >
                  <HugeiconsIcon
                    icon={CustomerSupportIcon}
                    data-icon='inline-start'
                  />
                  {t('Contact us to apply')}
                </Button>
              </div>

              <div className='mt-10 flex flex-wrap gap-x-6 gap-y-3'>
                {[
                  'No model inventory',
                  'Transparent records',
                  'Two growth paths',
                ].map((item) => (
                  <div
                    key={item}
                    className='text-muted-foreground flex items-center gap-2 text-sm'
                  >
                    <HugeiconsIcon
                      icon={CheckmarkCircle02Icon}
                      className='text-primary size-4'
                    />
                    {t(item)}
                  </div>
                ))}
              </div>
            </div>

            <PartnerWorkspacePreview
              topUpReward={topUpReward}
              consumptionReward={consumptionReward}
            />
          </div>
        </section>

        <section className='border-border/60 border-y px-4 py-16 sm:px-6 sm:py-20'>
          <div className='mx-auto max-w-7xl'>
            <SectionHeading
              eyebrow={t('Why partner with us')}
              title={t(
                'Focus on relationships. Let the platform handle complexity.'
              )}
              description={t(
                'A complete commercial foundation connects acquisition, delivery, accounting, and ongoing operations.'
              )}
            />
            <div className='mt-10 grid gap-4 md:grid-cols-2 lg:grid-cols-4'>
              {BENEFITS.map((benefit) => (
                <FeatureCard key={benefit.title} {...benefit} />
              ))}
            </div>
          </div>
        </section>

        <section id='partner-paths' className='scroll-mt-24 px-4 py-20 sm:px-6'>
          <div className='mx-auto max-w-7xl'>
            <SectionHeading
              eyebrow={t('Two partnership paths')}
              title={t('Start simply, then grow into your own brand.')}
              description={t(
                'Choose the model that fits your current audience, resources, and operating goals.'
              )}
            />

            <div className='mt-10 grid gap-5 lg:grid-cols-2'>
              <PartnershipCard
                icon={Share01Icon}
                badge={t('Open to every user')}
                title={t('Referral Partner')}
                description={t(
                  'Share your unique link and earn rewards when qualified users register, top up, or consume according to the active policy.'
                )}
                points={[
                  t('Get a unique invite link after registration'),
                  t('Track users and rewards from the partner center'),
                  t('Transfer rewards to balance or request withdrawal'),
                ]}
                footer={
                  <PartnerPrimaryLink isAuthenticated={isAuthenticated}>
                    {isAuthenticated
                      ? t('View my rewards')
                      : t('Create account')}
                  </PartnerPrimaryLink>
                }
              />
              <PartnershipCard
                icon={GlobalIcon}
                badge={t('For established operators')}
                title={t('Brand Partner')}
                description={t(
                  'Operate a branded API service with dedicated domain access, customer pricing, user management, and profit analytics.'
                )}
                points={[
                  t('Bind an independent domain and brand presentation'),
                  t('Manage customer groups and pricing rules'),
                  t('Review usage, profit, balance, and withdrawals'),
                ]}
                footer={
                  <Button
                    type='button'
                    variant='outline'
                    size='lg'
                    className='h-11 rounded-full px-5'
                    onClick={openCustomerService}
                  >
                    <HugeiconsIcon
                      icon={CustomerSupportIcon}
                      data-icon='inline-start'
                    />
                    {t('Contact us to apply')}
                    <HugeiconsIcon
                      icon={ArrowRight01Icon}
                      data-icon='inline-end'
                    />
                  </Button>
                }
              />
            </div>
          </div>
        </section>

        <section className='bg-muted/30 px-4 py-20 sm:px-6'>
          <div className='mx-auto max-w-7xl'>
            <SectionHeading
              eyebrow={t('Who it is for')}
              title={t('Turn the resources you already have into growth.')}
              description={t(
                'You do not need to be an AI infrastructure expert. Audience trust, customer insight, or product reach is enough to begin.'
              )}
            />
            <div className='mt-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
              {AUDIENCES.map((audience) => (
                <FeatureCard key={audience.title} {...audience} />
              ))}
            </div>
          </div>
        </section>

        <section id='how-it-works' className='scroll-mt-24 px-4 py-20 sm:px-6'>
          <div className='mx-auto max-w-7xl'>
            <SectionHeading
              eyebrow={t('How it works')}
              title={t('Four steps from interest to operation.')}
              description={t(
                'The referral path is ready after registration. Brand access is reviewed and enabled by the platform operator.'
              )}
            />

            <div className='mt-12 grid gap-4 md:grid-cols-2 lg:grid-cols-4'>
              <ProcessStep
                number='01'
                icon={WorkIcon}
                title={t('Choose your path')}
                description={t(
                  'Start with referral rewards or prepare your audience and operating plan for a branded service.'
                )}
              />
              <ProcessStep
                number='02'
                icon={UserMultiple02Icon}
                title={t('Create your account')}
                description={t(
                  'Register to receive your partner center and unique invite link.'
                )}
              />
              <ProcessStep
                number='03'
                icon={Link01Icon}
                title={t('Connect your audience')}
                description={t(
                  'Share your link, or work with the operator to configure brand access and your domain.'
                )}
              />
              <ProcessStep
                number='04'
                icon={MoneyReceiveCircleIcon}
                title={t('Grow and settle')}
                description={t(
                  'Follow performance, serve your users, and manage eligible rewards or profit from one place.'
                )}
              />
            </div>
          </div>
        </section>

        <section className='border-border/60 border-t px-4 py-20 sm:px-6'>
          <div className='mx-auto grid max-w-6xl gap-10 lg:grid-cols-[0.65fr_1fr]'>
            <SectionHeading
              eyebrow={t('Frequently asked questions')}
              title={t('Everything you need to know before you begin.')}
              description={t(
                'Actual reward rates, limits, and partner permissions follow the current platform configuration.'
              )}
            />
            <Card>
              <CardContent>
                <Accordion>
                  {FAQS.map((item) => (
                    <AccordionItem key={item.question} value={item.question}>
                      <AccordionTrigger className='py-4 text-base'>
                        {t(item.question)}
                      </AccordionTrigger>
                      <AccordionContent className='text-muted-foreground pb-4 leading-7'>
                        {t(item.answer)}
                      </AccordionContent>
                    </AccordionItem>
                  ))}
                </Accordion>
              </CardContent>
            </Card>
          </div>
        </section>

        <section className='px-4 pb-20 sm:px-6'>
          <div className='bg-primary text-primary-foreground relative mx-auto max-w-7xl overflow-hidden rounded-3xl px-6 py-14 text-center sm:px-10 sm:py-16'>
            <div className='bg-primary-foreground/10 absolute -top-24 -left-20 size-72 rounded-full blur-3xl' />
            <div className='bg-primary-foreground/10 absolute -right-16 -bottom-28 size-80 rounded-full blur-3xl' />
            <div className='relative mx-auto max-w-3xl'>
              <HugeiconsIcon icon={GiftIcon} className='mx-auto mb-5 size-9' />
              <h2 className='text-3xl font-semibold tracking-tight text-balance sm:text-4xl'>
                {t('Ready to build something valuable together?')}
              </h2>
              <p className='text-primary-foreground/75 mx-auto mt-4 max-w-2xl leading-7'>
                {t(
                  'Begin with a referral link today. When your audience and business are ready, move into a deeper brand partnership.'
                )}
              </p>
              <div className='mt-8 flex justify-center'>
                <PartnerPrimaryLink isAuthenticated={isAuthenticated} inverted>
                  {isAuthenticated
                    ? t('Open partner center')
                    : t('Join the partner program')}
                </PartnerPrimaryLink>
              </div>
            </div>
          </div>
        </section>
      </main>
      <Footer />
    </PublicLayout>
  )
}

function PartnerPrimaryLink(props: {
  isAuthenticated: boolean
  inverted?: boolean
  children: React.ReactNode
}) {
  const className = buttonVariants({
    variant: props.inverted ? 'secondary' : 'default',
    size: 'lg',
    className: cn('h-11 rounded-full px-5', props.inverted && 'shadow-sm'),
  })
  const content = (
    <>
      {props.children}
      <HugeiconsIcon icon={ArrowRight01Icon} data-icon='inline-end' />
    </>
  )

  if (props.isAuthenticated) {
    return (
      <Link to='/invite-rewards' className={className}>
        {content}
      </Link>
    )
  }

  return (
    <Link
      to='/sign-up'
      search={{ redirect: '/invite-rewards' }}
      className={className}
    >
      {content}
    </Link>
  )
}

function PartnerWorkspacePreview(props: {
  topUpReward: string
  consumptionReward: string
}) {
  const { t } = useTranslation()

  return (
    <div className='relative mx-auto w-full max-w-xl'>
      <div className='border-border/60 bg-background/80 rounded-3xl border p-4 shadow-2xl backdrop-blur-xl'>
        <div className='bg-card rounded-2xl border p-5'>
          <div className='flex items-center justify-between gap-4'>
            <div>
              <p className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
                {t('Partner workspace')}
              </p>
              <p className='mt-1 text-lg font-semibold'>
                {t('One audience, multiple ways to grow')}
              </p>
            </div>
            <div className='bg-primary/10 text-primary flex size-11 items-center justify-center rounded-xl'>
              <HugeiconsIcon icon={SparklesIcon} className='size-5' />
            </div>
          </div>

          <Separator className='my-5' />

          <div className='grid gap-3 sm:grid-cols-2'>
            <PreviewMetric
              icon={MoneyReceiveCircleIcon}
              label={t('Top-up reward')}
              value={props.topUpReward}
            />
            <PreviewMetric
              icon={BarChartIcon}
              label={t('Consumption reward')}
              value={props.consumptionReward}
            />
          </div>

          <div className='bg-muted/50 mt-4 rounded-xl border p-4'>
            <div className='flex items-center justify-between gap-4'>
              <div className='flex items-center gap-3'>
                <div className='bg-background text-primary flex size-9 items-center justify-center rounded-lg border'>
                  <HugeiconsIcon icon={Link01Icon} className='size-4' />
                </div>
                <div>
                  <p className='text-sm font-medium'>{t('Share your link')}</p>
                  <p className='text-muted-foreground text-xs'>
                    {t('Invite users into your partner network')}
                  </p>
                </div>
              </div>
              <HugeiconsIcon
                icon={ArrowRight01Icon}
                className='text-muted-foreground size-4'
              />
            </div>
          </div>
        </div>
      </div>

      <div className='bg-background absolute -right-3 -bottom-6 hidden items-center gap-3 rounded-2xl border p-3 shadow-lg sm:flex'>
        <div className='bg-primary/10 text-primary flex size-10 items-center justify-center rounded-xl'>
          <HugeiconsIcon icon={GlobalIcon} className='size-5' />
        </div>
        <div>
          <p className='text-sm font-semibold'>{t('Brand-ready')}</p>
          <p className='text-muted-foreground text-xs'>
            {t('Domain · Pricing · Users')}
          </p>
        </div>
      </div>
    </div>
  )
}

function PreviewMetric(props: {
  icon: IconContent
  label: string
  value: string
}) {
  return (
    <div className='bg-background rounded-xl border p-4'>
      <div className='text-muted-foreground flex items-center gap-2 text-xs font-medium'>
        <HugeiconsIcon icon={props.icon} className='size-4' />
        {props.label}
      </div>
      <p className='mt-3 text-xl font-semibold'>{props.value}</p>
    </div>
  )
}

function SectionHeading(props: {
  eyebrow: string
  title: string
  description: string
}) {
  return (
    <div className='max-w-3xl'>
      <p className='text-primary text-sm font-semibold tracking-wider uppercase'>
        {props.eyebrow}
      </p>
      <h2 className='mt-3 text-3xl font-semibold tracking-tight text-balance sm:text-4xl'>
        {props.title}
      </h2>
      <p className='text-muted-foreground mt-4 max-w-2xl text-base leading-7 sm:text-lg'>
        {props.description}
      </p>
    </div>
  )
}

function FeatureCard(props: FeatureItem) {
  const { t } = useTranslation()

  return (
    <Card className='h-full'>
      <CardHeader>
        <div className='bg-primary/10 text-primary mb-3 flex size-11 items-center justify-center rounded-xl'>
          <HugeiconsIcon icon={props.icon} className='size-5' />
        </div>
        <CardTitle>{t(props.title)}</CardTitle>
        <CardDescription className='leading-6'>
          {t(props.description)}
        </CardDescription>
      </CardHeader>
    </Card>
  )
}

function PartnershipCard(props: {
  icon: IconContent
  badge: string
  title: string
  description: string
  points: string[]
  footer: React.ReactNode
}) {
  return (
    <Card className='h-full'>
      <CardHeader>
        <div className='flex items-start justify-between gap-4'>
          <div className='bg-primary/10 text-primary flex size-12 items-center justify-center rounded-xl'>
            <HugeiconsIcon icon={props.icon} className='size-6' />
          </div>
          <Badge variant='secondary'>{props.badge}</Badge>
        </div>
        <CardTitle className='mt-4 text-2xl'>{props.title}</CardTitle>
        <CardDescription className='text-base leading-7'>
          {props.description}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Separator className='mb-5' />
        <ul className='flex flex-col gap-3'>
          {props.points.map((point) => (
            <li key={point} className='flex items-start gap-3 text-sm'>
              <HugeiconsIcon
                icon={CheckmarkCircle02Icon}
                className='text-primary mt-0.5 size-4 shrink-0'
              />
              <span>{point}</span>
            </li>
          ))}
        </ul>
      </CardContent>
      <CardFooter>{props.footer}</CardFooter>
    </Card>
  )
}

function ProcessStep(props: {
  number: string
  icon: IconContent
  title: string
  description: string
}) {
  return (
    <div className='relative flex h-full flex-col rounded-2xl border p-5'>
      <div className='flex items-center justify-between gap-4'>
        <span className='text-primary text-sm font-semibold'>
          {props.number}
        </span>
        <div className='bg-muted flex size-10 items-center justify-center rounded-xl'>
          <HugeiconsIcon icon={props.icon} className='size-5' />
        </div>
      </div>
      <h3 className='mt-8 text-lg font-semibold'>{props.title}</h3>
      <p className='text-muted-foreground mt-2 text-sm leading-6'>
        {props.description}
      </p>
    </div>
  )
}
