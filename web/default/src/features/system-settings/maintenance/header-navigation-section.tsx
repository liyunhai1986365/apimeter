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
import { useEffect, useMemo, useState } from 'react'
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  getBuiltInHeaderNavItem,
  getHeaderNavModuleEnabled,
  getHeaderNavModuleNewWindow,
  getHeaderNavModuleRequireAuth,
  isExternalHeaderNavHref,
  isHeaderNavBuiltInModule,
  mergeHeaderNavOrder,
  normalizeOpenMosaicBaseUrl,
  serializeHeaderNavModules,
  withAICreationBaseUrl,
  withHeaderNavModuleEnabled,
  withHeaderNavModuleNewWindow,
  withHeaderNavModuleRequireAuth,
  type HeaderNavBuiltInModule,
  type HeaderNavCustomLink,
  type HeaderNavModules,
} from '@/lib/nav-modules'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { HEADER_NAV_DEFAULT } from './config'

type HeaderNavigationSectionProps = {
  config: HeaderNavModules
  initialSerialized: string
  title?: string
  description?: string
  saveLabel?: string
  onSave?: (serialized: string) => Promise<unknown> | unknown
  saving?: boolean
}

const BUILT_IN_DESCRIPTIONS: Record<HeaderNavBuiltInModule, string> = {
  home: 'Landing page with system overview.',
  agentAccess: 'Agent integration documentation and onboarding guide.',
  aiCreation:
    'Open the configured OpenMosaic site, sign in automatically, and enter image creation.',
  console: 'User dashboard and quota controls.',
  pricing: 'Public model catalog and pricing page.',
  subscription: 'Public subscription plan storefront and billing guide.',
  rankings: 'Public rankings page based on live usage data.',
  docs: 'Documentation or external knowledge base.',
  about: 'Static page describing the platform.',
}

const createCustomLinkId = (
  title: string,
  links: HeaderNavCustomLink[]
): string => {
  const base = title
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  const prefix = base || 'link'
  let id = prefix
  let index = 2
  while (links.some((link) => link.id === id)) {
    id = `${prefix}-${index}`
    index += 1
  }
  return id
}

function normalizeConfig(config: HeaderNavModules): HeaderNavModules {
  return {
    ...config,
    aiCreation: { ...config.aiCreation },
    pricing: { ...config.pricing },
    subscription: { ...config.subscription },
    rankings: { ...config.rankings },
    newWindow: { ...config.newWindow },
    order: mergeHeaderNavOrder(config.order, config.customLinks),
    customLinks: config.customLinks.map((link) => ({ ...link })),
  }
}

function moveItem(order: string[], id: string, direction: -1 | 1): string[] {
  const index = order.indexOf(id)
  const nextIndex = index + direction
  if (index < 0 || nextIndex < 0 || nextIndex >= order.length) return order

  const next = [...order]
  const current = next[index]
  next[index] = next[nextIndex]
  next[nextIndex] = current
  return next
}

export function HeaderNavigationSection({
  config,
  initialSerialized,
  title,
  description,
  saveLabel,
  onSave,
  saving,
}: HeaderNavigationSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [navConfig, setNavConfig] = useState(() => normalizeConfig(config))
  const [newTitle, setNewTitle] = useState('')
  const [newHref, setNewHref] = useState('')

  useEffect(() => {
    setNavConfig(normalizeConfig(config))
  }, [config])

  const order = useMemo(
    () => mergeHeaderNavOrder(navConfig.order, navConfig.customLinks),
    [navConfig.customLinks, navConfig.order]
  )

  const setBuiltInEnabled = (id: HeaderNavBuiltInModule, enabled: boolean) => {
    setNavConfig((current) => withHeaderNavModuleEnabled(current, id, enabled))
  }

  const setBuiltInRequireAuth = (
    id: HeaderNavBuiltInModule,
    requireAuth: boolean
  ) => {
    setNavConfig((current) =>
      withHeaderNavModuleRequireAuth(current, id, requireAuth)
    )
  }

  const setBuiltInNewWindow = (
    id: HeaderNavBuiltInModule,
    newWindow: boolean
  ) => {
    setNavConfig((current) =>
      withHeaderNavModuleNewWindow(current, id, newWindow)
    )
  }

  const setCustomLink = (
    id: string,
    updater: (link: HeaderNavCustomLink) => HeaderNavCustomLink
  ) => {
    setNavConfig((current) => ({
      ...current,
      customLinks: current.customLinks.map((link) =>
        link.id === id ? updater(link) : link
      ),
    }))
  }

  const addCustomLink = () => {
    const title = newTitle.trim()
    const href = newHref.trim()
    if (!title || !href) {
      toast.error(t('Menu title and link are required.'))
      return
    }

    const id = createCustomLinkId(title, navConfig.customLinks)
    const link: HeaderNavCustomLink = {
      id,
      title,
      href,
      enabled: true,
      external: isExternalHeaderNavHref(href),
      newWindow: isExternalHeaderNavHref(href),
      requireAuth: false,
    }

    setNavConfig((current) => ({
      ...current,
      customLinks: [...current.customLinks, link],
      order: mergeHeaderNavOrder(
        [...current.order, `custom:${id}`],
        [...current.customLinks, link]
      ),
    }))
    setNewTitle('')
    setNewHref('')
  }

  const removeCustomLink = (id: string) => {
    setNavConfig((current) => ({
      ...current,
      customLinks: current.customLinks.filter((link) => link.id !== id),
      order: current.order.filter((item) => item !== `custom:${id}`),
    }))
  }

  const onSubmit = async () => {
    if (
      navConfig.aiCreation.enabled &&
      !normalizeOpenMosaicBaseUrl(navConfig.aiCreation.baseUrl)
    ) {
      toast.error(
        t('Enter a valid OpenMosaic site origin before enabling AI Creation.')
      )
      return
    }

    const serialized = serializeHeaderNavModules(navConfig)
    if (serialized === initialSerialized) return

    if (onSave) {
      await onSave(serialized)
      return
    }

    await updateOption.mutateAsync({
      key: 'HeaderNavModules',
      value: serialized,
    })
  }

  const resetToDefault = () => {
    setNavConfig(normalizeConfig(HEADER_NAV_DEFAULT))
  }
  const isSaving = saving ?? updateOption.isPending

  return (
    <SettingsSection
      title={title ?? t('Header navigation')}
      description={
        description ??
        t('Enable, add, or reorder top navigation menus globally.')
      }
    >
      <div className='space-y-6'>
        <div className='space-y-3'>
          {order.map((id, index) => {
            const isFirst = index === 0
            const isLast = index === order.length - 1

            if (isHeaderNavBuiltInModule(id)) {
              const item = getBuiltInHeaderNavItem(id)
              const enabled = getHeaderNavModuleEnabled(navConfig, id)
              const requireAuth = getHeaderNavModuleRequireAuth(navConfig, id)
              const newWindow = getHeaderNavModuleNewWindow(navConfig, id)
              const supportsAuth =
                id === 'pricing' || id === 'subscription' || id === 'rankings'
              const supportsNewWindow =
                id !== 'aiCreation' && (item.external || id === 'docs')

              return (
                <div
                  key={id}
                  className='grid gap-3 rounded-lg border p-4 md:grid-cols-[1fr_auto]'
                >
                  <div className='min-w-0 space-y-1'>
                    <div className='flex flex-wrap items-center gap-2'>
                      <Label className='text-base'>{t(item.titleKey)}</Label>
                      <span className='text-muted-foreground rounded-md border px-1.5 py-0.5 text-xs'>
                        {item.external ? t('External') : item.href}
                      </span>
                    </div>
                    <p className='text-muted-foreground text-sm'>
                      {t(BUILT_IN_DESCRIPTIONS[id])}
                    </p>
                    {id === 'aiCreation' ? (
                      <div className='max-w-xl space-y-2 pt-2'>
                        <Label htmlFor='openmosaic-site-url'>
                          {t('OpenMosaic site URL')}
                        </Label>
                        <Input
                          id='openmosaic-site-url'
                          type='url'
                          value={navConfig.aiCreation.baseUrl}
                          onChange={(event) =>
                            setNavConfig((current) =>
                              withAICreationBaseUrl(current, event.target.value)
                            )
                          }
                          placeholder='https://openmosaic.example.com'
                          aria-describedby='openmosaic-site-url-description'
                        />
                        <p
                          id='openmosaic-site-url-description'
                          className='text-muted-foreground text-xs'
                        >
                          {t(
                            'Enter the site origin only. The menu starts Modelsell SSO and lands on /image.'
                          )}
                        </p>
                      </div>
                    ) : null}
                    {supportsAuth ? (
                      <div className='flex items-center gap-2 pt-2'>
                        <Switch
                          checked={requireAuth}
                          disabled={!enabled}
                          onCheckedChange={(checked) =>
                            setBuiltInRequireAuth(id, checked)
                          }
                        />
                        <span className='text-muted-foreground text-sm'>
                          {t('Require login')}
                        </span>
                      </div>
                    ) : null}
                    {supportsNewWindow ? (
                      <div className='flex items-center gap-2 pt-2'>
                        <Switch
                          checked={newWindow}
                          disabled={!enabled}
                          onCheckedChange={(checked) =>
                            setBuiltInNewWindow(id, checked)
                          }
                        />
                        <span className='text-muted-foreground text-sm'>
                          {t('Open in new window')}
                        </span>
                      </div>
                    ) : null}
                  </div>

                  <div className='flex items-center gap-2 md:justify-end'>
                    <Switch
                      checked={enabled}
                      onCheckedChange={(checked) =>
                        setBuiltInEnabled(id, checked)
                      }
                    />
                    <Button
                      type='button'
                      variant='outline'
                      size='icon-sm'
                      disabled={isFirst}
                      onClick={() =>
                        setNavConfig((current) => ({
                          ...current,
                          order: moveItem(order, id, -1),
                        }))
                      }
                      aria-label={t('Move menu up')}
                    >
                      <ArrowUp />
                    </Button>
                    <Button
                      type='button'
                      variant='outline'
                      size='icon-sm'
                      disabled={isLast}
                      onClick={() =>
                        setNavConfig((current) => ({
                          ...current,
                          order: moveItem(order, id, 1),
                        }))
                      }
                      aria-label={t('Move menu down')}
                    >
                      <ArrowDown />
                    </Button>
                  </div>
                </div>
              )
            }

            const customId = id.replace(/^custom:/, '')
            const link = navConfig.customLinks.find(
              (item) => item.id === customId
            )
            if (!link) return null

            return (
              <div
                key={id}
                className='grid gap-3 rounded-lg border p-4 md:grid-cols-[1fr_auto]'
              >
                <div className='grid gap-3 md:grid-cols-2'>
                  <div className='space-y-2'>
                    <Label htmlFor={`${id}-title`}>{t('Menu title')}</Label>
                    <Input
                      id={`${id}-title`}
                      value={link.title}
                      onChange={(event) =>
                        setCustomLink(customId, (current) => ({
                          ...current,
                          title: event.target.value,
                        }))
                      }
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor={`${id}-href`}>{t('Menu link')}</Label>
                    <Input
                      id={`${id}-href`}
                      value={link.href}
                      onChange={(event) =>
                        setCustomLink(customId, (current) => ({
                          ...current,
                          href: event.target.value,
                          external: isExternalHeaderNavHref(event.target.value),
                          newWindow: isExternalHeaderNavHref(event.target.value)
                            ? current.newWindow
                            : false,
                        }))
                      }
                    />
                  </div>
                  <div className='flex items-center gap-2'>
                    <Switch
                      checked={link.newWindow}
                      disabled={!link.external}
                      onCheckedChange={(checked) =>
                        setCustomLink(customId, (current) => ({
                          ...current,
                          newWindow: checked,
                        }))
                      }
                    />
                    <span className='text-muted-foreground text-sm'>
                      {t('Open in new window')}
                    </span>
                  </div>
                  <div className='flex items-center gap-2'>
                    <Switch
                      checked={link.requireAuth ?? false}
                      onCheckedChange={(checked) =>
                        setCustomLink(customId, (current) => ({
                          ...current,
                          requireAuth: checked,
                        }))
                      }
                    />
                    <span className='text-muted-foreground text-sm'>
                      {t('Require login')}
                    </span>
                  </div>
                </div>

                <div className='flex items-center gap-2 md:justify-end'>
                  <Switch
                    checked={link.enabled}
                    onCheckedChange={(checked) =>
                      setCustomLink(customId, (current) => ({
                        ...current,
                        enabled: checked,
                      }))
                    }
                  />
                  <Button
                    type='button'
                    variant='outline'
                    size='icon-sm'
                    disabled={isFirst}
                    onClick={() =>
                      setNavConfig((current) => ({
                        ...current,
                        order: moveItem(order, id, -1),
                      }))
                    }
                    aria-label={t('Move menu up')}
                  >
                    <ArrowUp />
                  </Button>
                  <Button
                    type='button'
                    variant='outline'
                    size='icon-sm'
                    disabled={isLast}
                    onClick={() =>
                      setNavConfig((current) => ({
                        ...current,
                        order: moveItem(order, id, 1),
                      }))
                    }
                    aria-label={t('Move menu down')}
                  >
                    <ArrowDown />
                  </Button>
                  <Button
                    type='button'
                    variant='destructive'
                    size='icon-sm'
                    onClick={() => removeCustomLink(customId)}
                    aria-label={t('Remove menu')}
                  >
                    <Trash2 />
                  </Button>
                </div>
              </div>
            )
          })}
        </div>

        <div className='grid gap-3 rounded-lg border border-dashed p-4 md:grid-cols-[1fr_1fr_auto]'>
          <div className='space-y-2'>
            <Label htmlFor='new-menu-title'>{t('Menu title')}</Label>
            <Input
              id='new-menu-title'
              value={newTitle}
              onChange={(event) => setNewTitle(event.target.value)}
              placeholder={t('Agent Access')}
            />
          </div>
          <div className='space-y-2'>
            <Label htmlFor='new-menu-link'>{t('Menu link')}</Label>
            <Input
              id='new-menu-link'
              value={newHref}
              onChange={(event) => setNewHref(event.target.value)}
              placeholder='https://your-server.example.com/docs/apps'
            />
          </div>
          <div className='flex items-end'>
            <Button type='button' variant='outline' onClick={addCustomLink}>
              <Plus />
              {t('Add menu')}
            </Button>
          </div>
        </div>

        <div className='flex flex-wrap gap-3'>
          <Button type='button' variant='outline' onClick={resetToDefault}>
            {t('Reset to default')}
          </Button>
          <Button type='button' disabled={isSaving} onClick={onSubmit}>
            {isSaving ? t('Saving...') : (saveLabel ?? t('Save navigation'))}
          </Button>
        </div>
      </div>
    </SettingsSection>
  )
}
