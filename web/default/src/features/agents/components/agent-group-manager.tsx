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
import { Edit3, Eye, EyeOff, Link2, Save } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { TableEmpty } from '@/components/data-table'
import type { AgentGroupRatio } from '../types'

type AgentGroupManagerProps = {
  groupRatios: AgentGroupRatio[]
  groupName: string
  systemGroupName: string
  groupRatio: string
  groupVisible: boolean
  visibleGroups: string[]
  removeGroups: string[]
  canSave: boolean
  isPending: boolean
  onGroupNameChange: (value: string) => void
  onSystemGroupNameChange: (value: string) => void
  onGroupRatioChange: (value: string) => void
  onGroupVisibleChange: (value: boolean) => void
  onVisibleGroupsChange: (value: string[]) => void
  onRemoveGroupsChange: (value: string[]) => void
  onSave: () => void
  onEdit: (rule: AgentGroupRatio) => void
}

export function AgentGroupManager(props: AgentGroupManagerProps) {
  const { t } = useTranslation()
  const configuredGroups = useMemo(
    () => props.groupRatios.filter((item) => item.configured),
    [props.groupRatios]
  )
  const systemGroupOptions = useMemo(() => {
    const seen = new Set<string>()
    return props.groupRatios.filter((item) => {
      if (!item.system_group_name || seen.has(item.system_group_name)) {
        return false
      }
      seen.add(item.system_group_name)
      return true
    })
  }, [props.groupRatios])
  const selectedSystemGroup = systemGroupOptions.find(
    (item) => item.system_group_name === props.systemGroupName
  )
  const visibleCount = configuredGroups.filter(
    (item) => item.visible && item.available
  ).length
  const hiddenCount = configuredGroups.filter((item) => !item.visible).length
  const unavailableCount = configuredGroups.filter(
    (item) => !item.available
  ).length
  const visibleGroupOptions = configuredGroups.filter(
    (item) => item.available && item.group_name !== props.groupName
  )
  const toggleVisibleGroup = (groupName: string) => {
    if (props.visibleGroups.includes(groupName)) {
      props.onVisibleGroupsChange(
        props.visibleGroups.filter((item) => item !== groupName)
      )
      return
    }
    props.onVisibleGroupsChange([...props.visibleGroups, groupName].sort())
  }
  const removeGroupOptions = configuredGroups.filter(
    (item) => item.available && item.visible && item.group_name !== props.groupName
  )
  const toggleRemoveGroup = (groupName: string) => {
    if (props.removeGroups.includes(groupName)) {
      props.onRemoveGroupsChange(
        props.removeGroups.filter((item) => item !== groupName)
      )
      return
    }
    props.onRemoveGroupsChange([...props.removeGroups, groupName].sort())
  }

  return (
    <section className='rounded-lg border p-3'>
      <div className='mb-4 flex flex-col gap-2 md:flex-row md:items-start md:justify-between'>
        <div>
          <h3 className='text-sm font-semibold'>{t('Agent Groups')}</h3>
          <p className='text-muted-foreground mt-1 max-w-2xl text-xs'>
            {t(
              'Create proxy-facing groups, map them to original system groups, and decide which groups users can choose.'
            )}
          </p>
        </div>
        <div className='grid grid-cols-3 gap-2 text-xs'>
          <MetricPill label={t('Configured')} value={configuredGroups.length} />
          <MetricPill label={t('Visible')} value={visibleCount} />
          <MetricPill
            label={t('Unavailable')}
            value={unavailableCount}
            muted={unavailableCount === 0}
          />
        </div>
      </div>

      <div className='grid gap-4 xl:grid-cols-[340px_minmax(0,1fr)]'>
        <div className='space-y-3 xl:border-r xl:pr-4'>
          <div className='grid gap-1.5'>
            <span className='text-muted-foreground text-xs'>
              {t('Proxy group name')}
            </span>
            <Input
              value={props.groupName}
              onChange={(event) => props.onGroupNameChange(event.target.value)}
              placeholder={t('Agent group name')}
            />
          </div>

          <div className='grid gap-1.5'>
            <span className='text-muted-foreground text-xs'>
              {t('Original System Group')}
            </span>
            <select
              className='border-input bg-background h-8 rounded-md border px-2 text-sm'
              value={props.systemGroupName}
              onChange={(event) =>
                props.onSystemGroupNameChange(event.target.value)
              }
            >
              {systemGroupOptions.map((item) => (
                <option
                  key={item.system_group_name}
                  value={item.system_group_name}
                >
                  {item.system_group_name}
                </option>
              ))}
            </select>
            <span className='text-muted-foreground text-[11px]'>
              {t('Minimum discount')}: {selectedSystemGroup?.system_ratio ?? 0}
            </span>
          </div>

          <div className='grid gap-1.5'>
            <span className='text-muted-foreground text-xs'>
              {t('Agent Discount')}
            </span>
            <Input
              value={props.groupRatio}
              onChange={(event) => props.onGroupRatioChange(event.target.value)}
              type='number'
              step='0.01'
              min={selectedSystemGroup?.system_ratio ?? 0}
            />
          </div>

          <div className='flex items-center justify-between gap-3 rounded-md bg-muted/40 px-3 py-2'>
            <div>
              <div className='text-xs font-medium'>{t('Visible to users')}</div>
              <div className='text-muted-foreground text-[11px]'>
                {t('Hidden groups can still be assigned manually.')}
              </div>
            </div>
            <Switch
              checked={props.groupVisible}
              onCheckedChange={(checked) =>
                props.onGroupVisibleChange(checked === true)
              }
            />
          </div>

          <div className='grid gap-2 rounded-md bg-muted/40 px-3 py-2'>
            <div>
              <div className='text-xs font-medium'>
                {t('Additional groups for members')}
              </div>
              <div className='text-muted-foreground text-[11px]'>
                {t(
                  'Members still see globally visible groups; selected groups are appended to this user group.'
                )}
              </div>
            </div>
            <div className='flex max-h-32 flex-wrap gap-2 overflow-y-auto'>
              {visibleGroupOptions.length === 0 ? (
                <span className='text-muted-foreground text-xs'>
                  {t('No available groups to append')}
                </span>
              ) : (
                visibleGroupOptions.map((item) => (
                  <button
                    key={item.group_name}
                    type='button'
                    className='data-[selected=true]:bg-primary data-[selected=true]:text-primary-foreground rounded-md border px-2 py-1 text-xs'
                    data-selected={props.visibleGroups.includes(
                      item.group_name
                    )}
                    onClick={() => toggleVisibleGroup(item.group_name)}
                  >
                    {item.group_name}
                  </button>
                ))
              )}
            </div>
          </div>

          <div className='grid gap-2 rounded-md bg-muted/40 px-3 py-2'>
            <div>
              <div className='text-xs font-medium'>
                {t('Removed groups for members')}
              </div>
              <div className='text-muted-foreground text-[11px]'>
                {t(
                  'Selected globally visible groups are hidden from members of this user group.'
                )}
              </div>
            </div>
            <div className='flex max-h-32 flex-wrap gap-2 overflow-y-auto'>
              {removeGroupOptions.length === 0 ? (
                <span className='text-muted-foreground text-xs'>
                  {t('No visible groups to remove')}
                </span>
              ) : (
                removeGroupOptions.map((item) => (
                  <button
                    key={item.group_name}
                    type='button'
                    className='data-[selected=true]:bg-destructive data-[selected=true]:text-destructive-foreground rounded-md border px-2 py-1 text-xs'
                    data-selected={props.removeGroups.includes(
                      item.group_name
                    )}
                    onClick={() => toggleRemoveGroup(item.group_name)}
                  >
                    {item.group_name}
                  </button>
                ))
              )}
            </div>
          </div>

          <Button
            className='w-full'
            disabled={!props.canSave || props.isPending}
            onClick={props.onSave}
          >
            <Save />
            {t('Save Group')}
          </Button>
        </div>

        <div className='min-w-0'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Group')}</TableHead>
                <TableHead>{t('Mapping')}</TableHead>
                <TableHead>{t('Agent Discount')}</TableHead>
                <TableHead>{t('Effective Discount')}</TableHead>
                <TableHead>{t('Visibility')}</TableHead>
                <TableHead>{t('Appended groups')}</TableHead>
                <TableHead>{t('Removed groups')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {configuredGroups.length === 0 ? (
                <TableEmpty
                  colSpan={9}
                  title={t('No Agent Groups')}
                  description={t(
                    'Create an agent group before assigning users or showing groups in pricing.'
                  )}
                  icon={<Link2 className='size-6' />}
                />
              ) : (
                configuredGroups.map((rule) => (
                  <TableRow key={rule.group_name}>
                    <TableCell className='font-mono text-xs'>
                      {rule.group_name}
                    </TableCell>
                    <TableCell>
                      <div className='flex items-center gap-2 text-xs'>
                        <Link2 className='text-muted-foreground size-3.5' />
                        <span className='font-mono'>
                          {rule.system_group_name}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      {rule.configured ? rule.configured_ratio : '-'}
                    </TableCell>
                    <TableCell>{rule.effective_ratio}</TableCell>
                    <TableCell>
                      <Badge variant={rule.visible ? 'default' : 'outline'}>
                        {rule.visible ? (
                          <Eye className='size-3' />
                        ) : (
                          <EyeOff className='size-3' />
                        )}
                        {rule.visible ? t('Visible') : t('Hidden')}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className='text-muted-foreground max-w-[180px] truncate text-xs'>
                        {rule.visible_groups && rule.visible_groups.length > 0
                          ? rule.visible_groups.join(', ')
                          : t('No appended groups')}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className='text-muted-foreground max-w-[180px] truncate text-xs'>
                        {rule.remove_groups && rule.remove_groups.length > 0
                          ? rule.remove_groups.join(', ')
                          : t('No removed groups')}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={rule.available ? 'default' : 'destructive'}
                      >
                        {rule.available ? t('Available') : t('Unavailable')}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Button
                        variant='ghost'
                        size='sm'
                        onClick={() => props.onEdit(rule)}
                      >
                        <Edit3 className='size-4' />
                        {t('Edit')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
          {hiddenCount > 0 ? (
            <p className='text-muted-foreground mt-2 text-xs'>
              {t(
                'Hidden groups are not shown in user token selection, but admins can assign them to users.'
              )}
            </p>
          ) : null}
        </div>
      </div>
    </section>
  )
}

function MetricPill(props: { label: string; value: number; muted?: boolean }) {
  return (
    <div className='rounded-md border px-3 py-2 text-right'>
      <div className='text-muted-foreground whitespace-nowrap text-[11px]'>
        {props.label}
      </div>
      <div
        className={
          props.muted ? 'text-muted-foreground text-sm' : 'text-sm font-semibold'
        }
      >
        {props.value}
      </div>
    </div>
  )
}
