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
import { useMemo, useState } from 'react'
import { Edit3, Eye, EyeOff, RotateCcw, Save, Settings2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatDiscountPercentage } from '@/lib/group-discount'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { DiscountTooltip } from '@/components/discount-tooltip'
import { buildAgentGroupRuleRows } from '../api'
import type { AgentGroupRatio } from '../types'

type AgentGroupManagerProps = {
  groupRatios: AgentGroupRatio[]
  systemGroupName: string
  groupDescription: string
  groupRatio: string
  groupRatioFloor: number
  groupVisible: boolean
  canSave: boolean
  isPending: boolean
  onSystemGroupNameChange: (value: string) => void
  onGroupDescriptionChange: (value: string) => void
  onGroupRatioChange: (value: string) => void
  onGroupVisibleChange: (value: boolean) => void
  onSave: () => void
  onEdit: (rule: AgentGroupRatio) => void
  onResetForm: () => void
}

export function AgentGroupManager(props: AgentGroupManagerProps) {
  const { t } = useTranslation()
  const [editorOpen, setEditorOpen] = useState(false)
  const ruleRows = useMemo(
    () => buildAgentGroupRuleRows(props.groupRatios),
    [props.groupRatios]
  )
  const configuredGroups = useMemo(
    () => props.groupRatios.filter((item) => item.configured),
    [props.groupRatios]
  )
  const systemGroupOptions = useMemo(() => {
    const groups = new Map<string, AgentGroupRatio>()
    for (const item of props.groupRatios) {
      if (!item.system_group_name || groups.has(item.system_group_name)) {
        continue
      }
      groups.set(item.system_group_name, item)
    }
    return Array.from(groups.values()).sort((a, b) =>
      a.system_group_name.localeCompare(b.system_group_name)
    )
  }, [props.groupRatios])
  const visibleCount = configuredGroups.filter(
    (item) => item.visible && item.available
  ).length
  const hiddenCount = configuredGroups.filter((item) => !item.visible).length
  const unavailableCount = configuredGroups.filter(
    (item) => !item.available
  ).length
  const selectedRule = props.groupRatios.find(
    (item) => item.system_group_name === props.systemGroupName
  )

  const openRuleEditor = (rule: AgentGroupRatio) => {
    props.onEdit(rule)
    setEditorOpen(true)
  }

  return (
    <>
      <Card size='sm'>
        <CardHeader>
          <CardTitle>{t('Agent Group Rules')}</CardTitle>
          <CardDescription>
            {t(
              'Configure rules on existing system groups. New system groups are available automatically and use system defaults until a rule is saved.'
            )}
          </CardDescription>
          <CardAction>
            <div className='grid grid-cols-3 gap-2 text-xs'>
              <MetricPill
                label={t('Configured')}
                value={configuredGroups.length}
              />
              <MetricPill label={t('Visible')} value={visibleCount} />
              <MetricPill
                label={t('Unavailable')}
                value={unavailableCount}
                muted={unavailableCount === 0}
              />
            </div>
          </CardAction>
        </CardHeader>
        <CardContent>
          <div className='min-w-0'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('System Group')}</TableHead>
                  <TableHead>{t('Rule Status')}</TableHead>
                  <TableHead>{t('Agent Discount')}</TableHead>
                  <TableHead>{t('Effective Discount')}</TableHead>
                  <TableHead>{t('Visibility')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {ruleRows.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={7}>
                      <div className='flex flex-col items-center gap-2 py-8 text-center'>
                        <Settings2 />
                        <div className='text-sm font-medium'>
                          {t('No System Groups')}
                        </div>
                        <div className='text-muted-foreground text-xs'>
                          {t('System groups will appear here automatically.')}
                        </div>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : (
                  ruleRows.map((row) => {
                    const rule =
                      props.groupRatios.find(
                        (item) => item.system_group_name === row.systemGroupName
                      ) ??
                      ({
                        group_name: row.systemGroupName,
                        system_group_name: row.systemGroupName,
                        system_ratio: 0,
                        configured_ratio: 0,
                        effective_ratio: Number(row.effectiveDiscount) || 0,
                        configured: false,
                        visible: row.visible,
                        available: row.available,
                      } satisfies AgentGroupRatio)
                    return (
                      <TableRow key={row.systemGroupName}>
                        <TableCell className='font-mono text-xs'>
                          {row.systemGroupName}
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant={
                              row.status === 'configured'
                                ? 'default'
                                : 'secondary'
                            }
                          >
                            {row.status === 'configured'
                              ? t('Configured')
                              : t('System default')}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <AgentDiscountValue ratio={row.agentDiscount} />
                        </TableCell>
                        <TableCell>
                          <AgentDiscountValue ratio={row.effectiveDiscount} />
                        </TableCell>
                        <TableCell>
                          <Badge variant={row.visible ? 'default' : 'outline'}>
                            {row.visible ? <Eye /> : <EyeOff />}
                            {row.visible ? t('Visible') : t('Hidden')}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant={row.available ? 'default' : 'destructive'}
                          >
                            {row.available ? t('Available') : t('Unavailable')}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <Button
                            variant='ghost'
                            size='sm'
                            onClick={() => openRuleEditor(rule)}
                          >
                            <Edit3 />
                            {row.status === 'configured'
                              ? t('Edit')
                              : t('Create Rule')}
                          </Button>
                        </TableCell>
                      </TableRow>
                    )
                  })
                )}
              </TableBody>
            </Table>
            {hiddenCount > 0 ? (
              <p className='text-muted-foreground mt-2 text-xs'>
                {t(
                  'Hidden system groups are not shown in user token selection unless a user group rule enables them.'
                )}
              </p>
            ) : null}
          </div>
        </CardContent>
      </Card>

      <Dialog open={editorOpen} onOpenChange={setEditorOpen}>
        <DialogContent className='sm:max-w-lg'>
          <DialogHeader>
            <DialogTitle>
              {selectedRule?.configured ? t('Edit Rule') : t('Create Rule')}
            </DialogTitle>
            <DialogDescription>
              {t(
                'Update the agent-facing discount and visibility for this system group.'
              )}
            </DialogDescription>
          </DialogHeader>

          <FieldGroup>
            <Field>
              <FieldLabel>{t('System Group')}</FieldLabel>
              <NativeSelect
                className='w-full'
                value={props.systemGroupName}
                onChange={(event) =>
                  props.onSystemGroupNameChange(event.target.value)
                }
              >
                {systemGroupOptions.map((item) => (
                  <NativeSelectOption
                    key={item.system_group_name}
                    value={item.system_group_name}
                  >
                    {item.system_group_name}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
              <FieldDescription>
                {t('Minimum discount')}: {props.groupRatioFloor}
              </FieldDescription>
            </Field>

            <Field>
              <FieldLabel>{t('Rule description')}</FieldLabel>
              <Input
                value={props.groupDescription}
                onChange={(event) =>
                  props.onGroupDescriptionChange(event.target.value)
                }
                placeholder={t('Optional description')}
              />
            </Field>

            <Field>
              <FieldLabel>{t('Sales Discount')}</FieldLabel>
              <Input
                value={props.groupRatio}
                onChange={(event) =>
                  props.onGroupRatioChange(event.target.value)
                }
                type='number'
                step='0.01'
                min={props.groupRatioFloor}
              />
              <FieldDescription>
                {selectedRule?.configured
                  ? t('This sales discount is configured by the agent.')
                  : t('No sales discount saved yet. Agent discount applies.')}
              </FieldDescription>
            </Field>

            <Field orientation='horizontal'>
              <Switch
                checked={props.groupVisible}
                onCheckedChange={(checked) =>
                  props.onGroupVisibleChange(checked === true)
                }
              />
              <FieldContent>
                <FieldLabel>{t('Visible to users')}</FieldLabel>
                <FieldDescription>
                  {t(
                    'Hidden system groups can still be enabled by user group rules.'
                  )}
                </FieldDescription>
              </FieldContent>
            </Field>
          </FieldGroup>

          <DialogFooter>
            <Button
              variant='outline'
              disabled={props.isPending}
              onClick={props.onResetForm}
            >
              <RotateCcw />
              {t('Reset')}
            </Button>
            <Button
              disabled={!props.canSave || props.isPending}
              onClick={props.onSave}
            >
              <Save />
              {selectedRule?.configured ? t('Update Rule') : t('Save Rule')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function AgentDiscountValue({ ratio }: { ratio: string }) {
  const label = formatDiscountPercentage(ratio)
  if (!label) return ratio

  return (
    <DiscountTooltip label={label}>
      <Badge variant='secondary' className='font-mono'>
        {label}
      </Badge>
    </DiscountTooltip>
  )
}

function MetricPill(props: { label: string; value: number; muted?: boolean }) {
  return (
    <div className='rounded-md border px-3 py-2 text-right'>
      <div className='text-muted-foreground text-[11px] whitespace-nowrap'>
        {props.label}
      </div>
      <div
        className={
          props.muted
            ? 'text-muted-foreground text-sm'
            : 'text-sm font-semibold'
        }
      >
        {props.value}
      </div>
    </div>
  )
}
