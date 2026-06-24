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
import { useCallback, useEffect, useState } from 'react'
import { Cloud, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Switch } from '@/components/ui/switch'
import { PasswordInput } from '@/components/password-input'
import { testUserImageStorage, updateUserSettings } from '../../api'
import { parseUserSettings } from '../../lib'
import type { UserImageStorageSetting, UserProfile } from '../../types'

interface StorageTabProps {
  profile: UserProfile | null
  onUpdate: () => void
}

const defaultImageStorage: UserImageStorageSetting = {
  enabled: false,
  type: 'aliyun_oss',
  endpoint: '',
  accelerate_endpoint: '',
  bucket: '',
  access_key_id: '',
  access_key_secret: '',
  access_key_secret_configured: false,
  object_prefix: '',
  public_base_url: '',
}

const STORAGE_TYPES = [
  { value: 'aliyun_oss', label: 'Aliyun OSS' },
  { value: 'cloudflare_r2', label: 'Cloudflare R2' },
] as const

export function StorageTab({ profile, onUpdate }: StorageTabProps) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testURL, setTestURL] = useState('')
  const [imageStorage, setImageStorage] =
    useState<UserImageStorageSetting>(defaultImageStorage)
  const storageType = imageStorage.type ?? 'aliyun_oss'
  const isR2 = storageType === 'cloudflare_r2'

  useEffect(() => {
    const parsed = parseUserSettings(profile?.setting)
    setImageStorage({
      ...defaultImageStorage,
      ...(parsed.image_storage ?? {}),
      type: parsed.image_storage?.type ?? 'aliyun_oss',
      access_key_secret: parsed.image_storage?.access_key_secret ?? '',
      access_key_secret_configured:
        parsed.image_storage?.access_key_secret_configured ?? false,
    })
  }, [profile])

  const updateField = useCallback(
    <K extends keyof UserImageStorageSetting>(
      field: K,
      value: UserImageStorageSetting[K]
    ) => {
      setImageStorage((prev) => ({
        ...prev,
        [field]: value,
      }))
      setTestURL('')
    },
    []
  )

  const handleSave = async () => {
    try {
      setLoading(true)
      const current = parseUserSettings(profile?.setting)
      const response = await updateUserSettings({
        ...current,
        notify_type: current.notify_type ?? 'email',
        quota_warning_threshold: current.quota_warning_threshold ?? 0,
        image_storage: {
          ...imageStorage,
          type: storageType,
        },
      })

      if (response.success) {
        toast.success(t('Settings updated successfully'))
        onUpdate()
      } else {
        toast.error(response.message || t('Failed to update settings'))
      }
    } catch (_error) {
      toast.error(t('Failed to update settings'))
    } finally {
      setLoading(false)
    }
  }

  const handleTestUpload = async () => {
    try {
      setTesting(true)
      setTestURL('')
      const response = await testUserImageStorage({
        ...imageStorage,
        enabled: true,
        type: storageType,
      })

      if (response.success) {
        const url = response.data ?? ''
        setTestURL(url)
        toast.success(t('Storage test upload succeeded'))
      } else {
        toast.error(response.message || t('Storage test upload failed'))
      }
    } catch (_error) {
      toast.error(t('Storage test upload failed'))
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className='space-y-4 sm:space-y-6'>
      <div className='flex items-start justify-between gap-3 rounded-lg border p-3 sm:items-center sm:p-4'>
        <div className='flex min-w-0 gap-3'>
          <Cloud className='text-muted-foreground mt-0.5 h-4 w-4 shrink-0' />
          <div className='space-y-0.5'>
            <Label htmlFor='imageStorageEnabled'>
              {t('Use Custom Image Storage')}
            </Label>
            <p className='text-muted-foreground text-xs sm:text-sm'>
              {t(
                'Store transferred image results in your own cloud storage bucket'
              )}
            </p>
          </div>
        </div>
        <Switch
          id='imageStorageEnabled'
          className='shrink-0'
          checked={imageStorage.enabled ?? false}
          onCheckedChange={(checked) => updateField('enabled', checked)}
        />
      </div>

      {imageStorage.enabled && (
        <div className='grid gap-3 rounded-lg border p-3 sm:grid-cols-2 sm:p-4'>
          <div className='space-y-2 sm:col-span-2'>
            <Label>{t('Storage Provider')}</Label>
            <RadioGroup
              value={storageType}
              onValueChange={(value) =>
                updateField(
                  'type',
                  value as NonNullable<UserImageStorageSetting['type']>
                )
              }
              className='grid grid-cols-2 gap-2'
            >
              {STORAGE_TYPES.map((type) => (
                <Label
                  key={type.value}
                  htmlFor={`storage-${type.value}`}
                  className={`flex min-h-10 cursor-pointer items-center justify-center rounded-lg border px-3 text-sm font-medium transition-colors ${
                    storageType === type.value
                      ? 'border-primary bg-primary/5 text-primary'
                      : 'border-muted hover:border-muted-foreground/25 hover:bg-muted/50'
                  }`}
                >
                  <RadioGroupItem
                    id={`storage-${type.value}`}
                    value={type.value}
                    className='sr-only'
                  />
                  {t(type.label)}
                </Label>
              ))}
            </RadioGroup>
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='storageEndpoint'>
              {isR2 ? t('R2 Endpoint') : t('OSS Endpoint')}
            </Label>
            <Input
              id='storageEndpoint'
              className='h-9'
              value={imageStorage.endpoint ?? ''}
              onChange={(e) => updateField('endpoint', e.target.value)}
              placeholder={
                isR2
                  ? 'https://<account-id>.r2.cloudflarestorage.com'
                  : 'https://oss-cn-hangzhou.aliyuncs.com'
              }
            />
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='storageBucket'>
              {isR2 ? t('R2 Bucket') : t('OSS Bucket')}
            </Label>
            <Input
              id='storageBucket'
              className='h-9'
              value={imageStorage.bucket ?? ''}
              onChange={(e) => updateField('bucket', e.target.value)}
              placeholder={t('Bucket name')}
            />
          </div>
          {!isR2 && (
            <div className='space-y-1.5 sm:col-span-2'>
              <Label htmlFor='ossAccelerateEndpoint'>
                {t('OSS Transfer Acceleration Endpoint')}
              </Label>
              <Input
                id='ossAccelerateEndpoint'
                className='h-9'
                value={imageStorage.accelerate_endpoint ?? ''}
                onChange={(e) =>
                  updateField('accelerate_endpoint', e.target.value)
                }
                placeholder='https://oss-accelerate.aliyuncs.com'
              />
            </div>
          )}
          <div className='space-y-1.5'>
            <Label htmlFor='storageAccessKeyId'>{t('AccessKey ID')}</Label>
            <Input
              id='storageAccessKeyId'
              className='h-9'
              value={imageStorage.access_key_id ?? ''}
              onChange={(e) => updateField('access_key_id', e.target.value)}
              placeholder={t('Enter AccessKey ID')}
            />
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='storageAccessKeySecret'>
              {t('AccessKey Secret')}
            </Label>
            <PasswordInput
              id='storageAccessKeySecret'
              value={imageStorage.access_key_secret ?? ''}
              onChange={(e) =>
                updateField('access_key_secret', e.target.value)
              }
              placeholder={t('Leave empty to keep existing secret')}
            />
            {imageStorage.access_key_secret_configured && (
              <p className='text-muted-foreground text-xs'>
                {t('A secret is already configured')}
              </p>
            )}
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='ossPrefix'>{t('Object Prefix')}</Label>
            <Input
              id='ossPrefix'
              className='h-9'
              value={imageStorage.object_prefix ?? ''}
              onChange={(e) => updateField('object_prefix', e.target.value)}
              placeholder='relay-temp-images'
            />
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='ossPublicBaseUrl'>{t('Public Base URL')}</Label>
            <Input
              id='ossPublicBaseUrl'
              className='h-9'
              value={imageStorage.public_base_url ?? ''}
              onChange={(e) => updateField('public_base_url', e.target.value)}
              placeholder='https://cdn.example.com'
            />
          </div>
          <p className='text-muted-foreground sm:col-span-2 text-xs'>
            {t(
              'When this is enabled and complete, image transfers use your storage configuration. Otherwise the site default storage is used.'
            )}
          </p>
          {testURL && (
            <div className='sm:col-span-2 rounded-md border bg-muted/30 p-2 text-xs'>
              <span className='text-muted-foreground'>
                {t('Test file URL')}
              </span>
              <a
                href={testURL}
                target='_blank'
                rel='noreferrer'
                className='text-primary mt-1 block break-all hover:underline'
              >
                {testURL}
              </a>
            </div>
          )}
        </div>
      )}

      <div className='flex flex-col-reverse gap-2 sm:flex-row sm:justify-end'>
        {imageStorage.enabled && (
          <Button
            type='button'
            variant='outline'
            onClick={handleTestUpload}
            disabled={testing || loading}
          >
            {testing && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {testing ? t('Testing...') : t('Test Upload')}
          </Button>
        )}
        <Button onClick={handleSave} disabled={loading}>
          {loading && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
          {loading ? t('Saving...') : t('Save Settings')}
        </Button>
      </div>
    </div>
  )
}
