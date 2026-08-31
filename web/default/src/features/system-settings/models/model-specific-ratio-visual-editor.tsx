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
import { memo, useMemo, useState } from 'react'
import { Add01Icon, Delete02Icon, Edit02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
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
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  buildConfiguredBillingGroupNameOptions,
  buildConfiguredUserGroupNameOptions,
} from './group-ratio-visual-editor'
import {
  parseGroupModelRatioRules,
  parseUserGroupModelRatioRules,
  parseUserGroupRatioRules,
  serializeGroupModelRatioRules,
  serializeUserGroupModelRatioRules,
  serializeUserGroupRatioRules,
  type GroupModelRatioRule,
  type UserGroupModelRatioRule,
  type UserGroupRatioRule,
} from './model-specific-ratio-utils'

type RuleKind = 'group-model' | 'user-group' | 'user-group-model'

type EditableRule =
  | { kind: 'group-model'; rule: GroupModelRatioRule }
  | { kind: 'user-group'; rule: UserGroupRatioRule }
  | { kind: 'user-group-model'; rule: UserGroupModelRatioRule }

type RuleDraft = {
  userGroup: string
  group: string
  model: string
  ratio: string
}

type RuleErrors = Partial<
  Record<'userGroup' | 'group' | 'model' | 'ratio' | 'combination', string>
>

type UserGroupPricingRow =
  | { kind: 'user-group'; rule: UserGroupRatioRule }
  | { kind: 'user-group-model'; rule: UserGroupModelRatioRule }

type ModelSpecificRatioVisualEditorProps = {
  groupModelRatio: string
  groupGroupRatio: string
  userGroupModelRatio: string
  groupRatio: string
  userUsableGroups: string
  groupDisplayConfig: string
  modelOptions: string[]
  onChange: (
    field: 'GroupModelRatio' | 'GroupGroupRatio' | 'UserGroupModelRatio',
    value: string
  ) => void
}

function uniqueOptions(...groups: string[][]): string[] {
  return [
    ...new Set(
      groups
        .flat()
        .map((item) => item.trim())
        .filter(Boolean)
    ),
  ].sort((left, right) => left.localeCompare(right))
}

function isSameGroupModelRule(
  left: GroupModelRatioRule,
  right: GroupModelRatioRule
) {
  return left.group === right.group && left.model === right.model
}

function isSameUserGroupRule(
  left: UserGroupRatioRule,
  right: UserGroupRatioRule
) {
  return left.userGroup === right.userGroup && left.group === right.group
}

function isSameUserGroupModelRule(
  left: UserGroupModelRatioRule,
  right: UserGroupModelRatioRule
) {
  return left.userGroup === right.userGroup && isSameGroupModelRule(left, right)
}

function buildUserGroupPricingSections(
  groupRules: UserGroupRatioRule[],
  modelRules: UserGroupModelRatioRule[]
) {
  const sections = new Map<string, UserGroupPricingRow[]>()

  for (const rule of groupRules) {
    const rows = sections.get(rule.userGroup) ?? []
    rows.push({ kind: 'user-group', rule })
    sections.set(rule.userGroup, rows)
  }
  for (const rule of modelRules) {
    const rows = sections.get(rule.userGroup) ?? []
    rows.push({ kind: 'user-group-model', rule })
    sections.set(rule.userGroup, rows)
  }

  return [...sections.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([userGroup, rows]) => ({
      userGroup,
      rows: rows.sort((left, right) => {
        if (left.rule.group !== right.rule.group) {
          return left.rule.group.localeCompare(right.rule.group)
        }
        if (left.kind !== right.kind) {
          return left.kind === 'user-group' ? -1 : 1
        }
        if (left.kind === 'user-group-model') {
          return left.rule.model.localeCompare(
            (right as { rule: UserGroupModelRatioRule }).rule.model
          )
        }
        return 0
      }),
    }))
}

export const ModelSpecificRatioVisualEditor = memo(
  function ModelSpecificRatioVisualEditor({
    groupModelRatio,
    groupGroupRatio,
    userGroupModelRatio,
    groupRatio,
    userUsableGroups,
    groupDisplayConfig,
    modelOptions,
    onChange,
  }: ModelSpecificRatioVisualEditorProps) {
    const { t } = useTranslation()
    const [dialogKind, setDialogKind] = useState<RuleKind | null>(null)
    const [editingRule, setEditingRule] = useState<EditableRule | null>(null)
    const [draft, setDraft] = useState<RuleDraft>({
      userGroup: '',
      group: '',
      model: '',
      ratio: '1',
    })
    const [errors, setErrors] = useState<RuleErrors>({})

    const groupModelRules = useMemo(
      () => parseGroupModelRatioRules(groupModelRatio),
      [groupModelRatio]
    )
    const userGroupRules = useMemo(
      () => parseUserGroupRatioRules(groupGroupRatio),
      [groupGroupRatio]
    )
    const userGroupModelRules = useMemo(
      () => parseUserGroupModelRatioRules(userGroupModelRatio),
      [userGroupModelRatio]
    )
    const configuredUserGroupOptions = useMemo(
      () =>
        buildConfiguredUserGroupNameOptions(
          groupRatio,
          userUsableGroups,
          groupDisplayConfig
        ),
      [groupDisplayConfig, groupRatio, userUsableGroups]
    )
    const billingGroupOptions = useMemo(() => {
      const userGroupNames = new Set(configuredUserGroupOptions)
      return uniqueOptions(
        buildConfiguredBillingGroupNameOptions(
          groupRatio,
          userUsableGroups,
          groupDisplayConfig
        ),
        groupModelRules.map((rule) => rule.group),
        userGroupRules.map((rule) => rule.group),
        userGroupModelRules.map((rule) => rule.group)
      ).filter((group) => !userGroupNames.has(group))
    }, [
      configuredUserGroupOptions,
      groupDisplayConfig,
      groupModelRules,
      groupRatio,
      userGroupModelRules,
      userGroupRules,
      userUsableGroups,
    ])
    const userGroupOptions = useMemo(
      () =>
        uniqueOptions(
          configuredUserGroupOptions,
          userGroupRules.map((rule) => rule.userGroup),
          userGroupModelRules.map((rule) => rule.userGroup)
        ),
      [configuredUserGroupOptions, userGroupModelRules, userGroupRules]
    )
    const configuredModelOptions = useMemo(
      () =>
        uniqueOptions(
          modelOptions,
          groupModelRules.map((rule) => rule.model),
          userGroupModelRules.map((rule) => rule.model)
        ),
      [groupModelRules, modelOptions, userGroupModelRules]
    )
    const userGroupPricingSections = useMemo(
      () => buildUserGroupPricingSections(userGroupRules, userGroupModelRules),
      [userGroupModelRules, userGroupRules]
    )

    const closeDialog = () => {
      setDialogKind(null)
      setEditingRule(null)
      setErrors({})
    }

    const openRuleDialog = (kind: RuleKind, editableRule?: EditableRule) => {
      const rule = editableRule?.rule
      setDialogKind(kind)
      setEditingRule(editableRule ?? null)
      setDraft({
        userGroup:
          rule && 'userGroup' in rule
            ? rule.userGroup
            : (userGroupOptions[0] ?? ''),
        group: rule?.group ?? billingGroupOptions[0] ?? '',
        model: rule && 'model' in rule ? rule.model : '',
        ratio: String(rule?.ratio ?? 1),
      })
      setErrors({})
    }

    const handleSaveRule = () => {
      if (!dialogKind) return
      const nextErrors: RuleErrors = {}
      const userGroup = draft.userGroup.trim()
      const group = draft.group.trim()
      const model = draft.model.trim()
      const ratio = Number(draft.ratio)
      const isUserGroupKind = dialogKind !== 'group-model'
      const isModelKind = dialogKind !== 'user-group'

      if (isUserGroupKind && !userGroup) {
        nextErrors.userGroup = 'User group is required'
      }
      if (!group) nextErrors.group = 'Billing group is required'
      if (isModelKind && !model) nextErrors.model = 'Model is required'
      if (draft.ratio.trim() === '' || !Number.isFinite(ratio) || ratio < 0) {
        nextErrors.ratio = 'Ratio must be a non-negative number'
      }

      if (Object.keys(nextErrors).length === 0) {
        if (dialogKind === 'group-model') {
          const nextRule = { group, model, ratio }
          const currentRule =
            editingRule?.kind === 'group-model' ? editingRule.rule : null
          const duplicate = groupModelRules.some(
            (rule) =>
              isSameGroupModelRule(rule, nextRule) &&
              (!currentRule || !isSameGroupModelRule(rule, currentRule))
          )
          if (duplicate) {
            nextErrors.combination = 'This combination already exists'
          } else {
            const nextRules = currentRule
              ? groupModelRules.map((rule) =>
                  isSameGroupModelRule(rule, currentRule) ? nextRule : rule
                )
              : [...groupModelRules, nextRule]
            onChange(
              'GroupModelRatio',
              serializeGroupModelRatioRules(nextRules)
            )
          }
        } else if (dialogKind === 'user-group') {
          const nextRule = { userGroup, group, ratio }
          const currentRule =
            editingRule?.kind === 'user-group' ? editingRule.rule : null
          const duplicate = userGroupRules.some(
            (rule) =>
              isSameUserGroupRule(rule, nextRule) &&
              (!currentRule || !isSameUserGroupRule(rule, currentRule))
          )
          if (duplicate) {
            nextErrors.combination = 'This combination already exists'
          } else {
            const nextRules = currentRule
              ? userGroupRules.map((rule) =>
                  isSameUserGroupRule(rule, currentRule) ? nextRule : rule
                )
              : [...userGroupRules, nextRule]
            onChange('GroupGroupRatio', serializeUserGroupRatioRules(nextRules))
          }
        } else {
          const nextRule = { userGroup, group, model, ratio }
          const currentRule =
            editingRule?.kind === 'user-group-model' ? editingRule.rule : null
          const duplicate = userGroupModelRules.some(
            (rule) =>
              isSameUserGroupModelRule(rule, nextRule) &&
              (!currentRule || !isSameUserGroupModelRule(rule, currentRule))
          )
          if (duplicate) {
            nextErrors.combination = 'This combination already exists'
          } else {
            const nextRules = currentRule
              ? userGroupModelRules.map((rule) =>
                  isSameUserGroupModelRule(rule, currentRule) ? nextRule : rule
                )
              : [...userGroupModelRules, nextRule]
            onChange(
              'UserGroupModelRatio',
              serializeUserGroupModelRatioRules(nextRules)
            )
          }
        }
      }

      if (Object.keys(nextErrors).length > 0) {
        setErrors(nextErrors)
        return
      }
      closeDialog()
    }

    const handleDeleteGroupModelRule = (rule: GroupModelRatioRule) => {
      onChange(
        'GroupModelRatio',
        serializeGroupModelRatioRules(
          groupModelRules.filter((item) => !isSameGroupModelRule(item, rule))
        )
      )
    }

    const handleDeleteUserGroupRule = (rule: UserGroupRatioRule) => {
      onChange(
        'GroupGroupRatio',
        serializeUserGroupRatioRules(
          userGroupRules.filter((item) => !isSameUserGroupRule(item, rule))
        )
      )
    }

    const handleDeleteUserGroupModelRule = (rule: UserGroupModelRatioRule) => {
      onChange(
        'UserGroupModelRatio',
        serializeUserGroupModelRatioRules(
          userGroupModelRules.filter(
            (item) => !isSameUserGroupModelRule(item, rule)
          )
        )
      )
    }

    const isUserGroupDialog = dialogKind !== 'group-model'
    const isModelDialog = dialogKind !== 'user-group'
    const isEditing = editingRule !== null
    const dialogTitle =
      dialogKind === 'group-model'
        ? isEditing
          ? 'Edit model-specific ratio'
          : 'Add model-specific ratio'
        : dialogKind === 'user-group'
          ? isEditing
            ? 'Edit group-wide override'
            : 'Add group-wide override'
          : isEditing
            ? 'Edit user-group model override'
            : 'Add user-group model override'
    const dialogDescription =
      dialogKind === 'user-group'
        ? 'This fallback ratio applies only when no user-group model override or model-specific group ratio matches.'
        : dialogKind === 'user-group-model'
          ? 'This is the highest-priority final ratio for the selected user group, billing group, and model.'
          : 'The ratio is the final billing multiplier; it is not multiplied by inherited ratios.'

    return (
      <div className='flex flex-col gap-6'>
        <Card>
          <CardHeader>
            <CardTitle>{t('Model-specific group ratios')}</CardTitle>
            <CardDescription>
              {t(
                'Final ratio overrides by billing group and model. Missing models inherit the existing group ratio.'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className='flex flex-col gap-4'>
            <div>
              <Button
                type='button'
                size='sm'
                onClick={() => openRuleDialog('group-model')}
              >
                <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
                {t('Add model-specific ratio')}
              </Button>
            </div>

            {groupModelRules.length === 0 ? (
              <Empty className='border'>
                <EmptyHeader>
                  <EmptyTitle>
                    {t('No model-specific ratios configured')}
                  </EmptyTitle>
                  <EmptyDescription>
                    {t(
                      'Add a rule to set a final ratio for one model in a billing group.'
                    )}
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : (
              <div className='overflow-hidden rounded-lg border'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Billing group')}</TableHead>
                      <TableHead>{t('Model')}</TableHead>
                      <TableHead>{t('Ratio')}</TableHead>
                      <TableHead className='w-24 text-right'>
                        <span className='sr-only'>{t('Actions')}</span>
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {groupModelRules.map((rule) => (
                      <TableRow key={`${rule.group}:${rule.model}`}>
                        <TableCell className='font-medium'>
                          {rule.group}
                        </TableCell>
                        <TableCell>{rule.model}</TableCell>
                        <TableCell>{rule.ratio}</TableCell>
                        <TableCell>
                          <div className='flex justify-end gap-1'>
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              onClick={() =>
                                openRuleDialog('group-model', {
                                  kind: 'group-model',
                                  rule,
                                })
                              }
                              aria-label={t('Edit')}
                            >
                              <HugeiconsIcon icon={Edit02Icon} />
                            </Button>
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              onClick={() => handleDeleteGroupModelRule(rule)}
                              aria-label={t('Delete')}
                            >
                              <HugeiconsIcon icon={Delete02Icon} />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('User-group pricing overrides')}</CardTitle>
            <CardDescription>
              {t(
                'Configure group-wide and model-specific ratios together. User-group model overrides have highest priority; group-wide overrides apply only when no model-level ratio matches.'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className='flex flex-col gap-4'>
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                size='sm'
                onClick={() => openRuleDialog('user-group')}
              >
                <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
                {t('Add group-wide override')}
              </Button>
              <Button
                type='button'
                size='sm'
                variant='outline'
                onClick={() => openRuleDialog('user-group-model')}
              >
                <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
                {t('Add model override')}
              </Button>
            </div>

            {userGroupPricingSections.length === 0 ? (
              <Empty className='border'>
                <EmptyHeader>
                  <EmptyTitle>
                    {t('No user-group pricing overrides configured')}
                  </EmptyTitle>
                  <EmptyDescription>
                    {t(
                      'Add a group-wide fallback or a model-specific override for a user group.'
                    )}
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : (
              <div className='flex flex-col gap-3'>
                {userGroupPricingSections.map((section) => (
                  <div
                    key={section.userGroup}
                    className='overflow-hidden rounded-lg border'
                  >
                    <div className='bg-muted/20 flex items-center gap-2 px-4 py-3'>
                      <span className='font-semibold'>{section.userGroup}</span>
                      <Badge variant='secondary'>
                        {t('{{count}} pricing rules', {
                          count: section.rows.length,
                        })}
                      </Badge>
                    </div>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Billing group')}</TableHead>
                          <TableHead>{t('Applies to')}</TableHead>
                          <TableHead>{t('Ratio')}</TableHead>
                          <TableHead className='w-24 text-right'>
                            <span className='sr-only'>{t('Actions')}</span>
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {section.rows.map((row) => {
                          const key =
                            row.kind === 'user-group'
                              ? `${row.rule.group}:fallback`
                              : `${row.rule.group}:${row.rule.model}`
                          return (
                            <TableRow key={key}>
                              <TableCell className='font-medium'>
                                {row.rule.group}
                              </TableCell>
                              <TableCell>
                                {row.kind === 'user-group' ? (
                                  <Badge variant='outline'>
                                    {t('Group-wide fallback')}
                                  </Badge>
                                ) : (
                                  row.rule.model
                                )}
                              </TableCell>
                              <TableCell>{row.rule.ratio}</TableCell>
                              <TableCell>
                                <div className='flex justify-end gap-1'>
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='icon-sm'
                                    onClick={() =>
                                      row.kind === 'user-group'
                                        ? openRuleDialog('user-group', row)
                                        : openRuleDialog(
                                            'user-group-model',
                                            row
                                          )
                                    }
                                    aria-label={t('Edit')}
                                  >
                                    <HugeiconsIcon icon={Edit02Icon} />
                                  </Button>
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='icon-sm'
                                    onClick={() =>
                                      row.kind === 'user-group'
                                        ? handleDeleteUserGroupRule(row.rule)
                                        : handleDeleteUserGroupModelRule(
                                            row.rule
                                          )
                                    }
                                    aria-label={t('Delete')}
                                  >
                                    <HugeiconsIcon icon={Delete02Icon} />
                                  </Button>
                                </div>
                              </TableCell>
                            </TableRow>
                          )
                        })}
                      </TableBody>
                    </Table>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        <Dialog
          open={dialogKind !== null}
          onOpenChange={(open) => {
            if (!open) closeDialog()
          }}
        >
          <DialogContent className='sm:max-w-lg'>
            <DialogHeader>
              <DialogTitle>{t(dialogTitle)}</DialogTitle>
              <DialogDescription>{t(dialogDescription)}</DialogDescription>
            </DialogHeader>

            <FieldGroup>
              {isUserGroupDialog && (
                <Field data-invalid={Boolean(errors.userGroup)}>
                  <FieldLabel htmlFor='model-ratio-user-group'>
                    {t('User group')}
                  </FieldLabel>
                  <NativeSelect
                    id='model-ratio-user-group'
                    value={draft.userGroup}
                    onChange={(event) =>
                      setDraft((current) => ({
                        ...current,
                        userGroup: event.target.value,
                      }))
                    }
                    aria-invalid={Boolean(errors.userGroup)}
                    disabled={userGroupOptions.length === 0}
                  >
                    <NativeSelectOption value=''>
                      {userGroupOptions.length === 0
                        ? t('No groups available')
                        : t('Select a group')}
                    </NativeSelectOption>
                    {userGroupOptions.map((option) => (
                      <NativeSelectOption key={option} value={option}>
                        {option}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                  <FieldError>
                    {errors.userGroup ? t(errors.userGroup) : null}
                  </FieldError>
                </Field>
              )}

              <Field data-invalid={Boolean(errors.group)}>
                <FieldLabel htmlFor='model-ratio-billing-group'>
                  {t('Billing group')}
                </FieldLabel>
                <NativeSelect
                  id='model-ratio-billing-group'
                  value={draft.group}
                  onChange={(event) =>
                    setDraft((current) => ({
                      ...current,
                      group: event.target.value,
                    }))
                  }
                  aria-invalid={Boolean(errors.group)}
                  disabled={billingGroupOptions.length === 0}
                >
                  <NativeSelectOption value=''>
                    {billingGroupOptions.length === 0
                      ? t('No groups available')
                      : t('Select a group')}
                  </NativeSelectOption>
                  {billingGroupOptions.map((option) => (
                    <NativeSelectOption key={option} value={option}>
                      {option}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
                <FieldError>{errors.group ? t(errors.group) : null}</FieldError>
              </Field>

              {isModelDialog && (
                <Field data-invalid={Boolean(errors.model)}>
                  <FieldLabel htmlFor='model-ratio-model'>
                    {t('Model')}
                  </FieldLabel>
                  <Input
                    id='model-ratio-model'
                    list='model-ratio-model-options'
                    value={draft.model}
                    onChange={(event) =>
                      setDraft((current) => ({
                        ...current,
                        model: event.target.value,
                      }))
                    }
                    aria-invalid={Boolean(errors.model)}
                    placeholder='glm-5.2'
                  />
                  <datalist id='model-ratio-model-options'>
                    {configuredModelOptions.map((option) => (
                      <option key={option} value={option} />
                    ))}
                  </datalist>
                  <FieldError>
                    {errors.model ? t(errors.model) : null}
                  </FieldError>
                </Field>
              )}

              <Field data-invalid={Boolean(errors.ratio)}>
                <FieldLabel htmlFor='model-ratio-value'>
                  {t('Ratio')}
                </FieldLabel>
                <Input
                  id='model-ratio-value'
                  type='number'
                  min='0'
                  step='any'
                  value={draft.ratio}
                  onChange={(event) =>
                    setDraft((current) => ({
                      ...current,
                      ratio: event.target.value,
                    }))
                  }
                  aria-invalid={Boolean(errors.ratio)}
                  placeholder='0.7'
                />
                <FieldDescription>
                  {t(
                    dialogKind === 'user-group'
                      ? 'Used as the user-group fallback for this billing group.'
                      : 'Enter the final billing multiplier, for example 0.7 for 70%.'
                  )}
                </FieldDescription>
                <FieldError>{errors.ratio ? t(errors.ratio) : null}</FieldError>
              </Field>

              {errors.combination && (
                <FieldError>{t(errors.combination)}</FieldError>
              )}
            </FieldGroup>

            <DialogFooter>
              <Button type='button' variant='outline' onClick={closeDialog}>
                {t('Cancel')}
              </Button>
              <Button type='button' onClick={handleSaveRule}>
                {t(isEditing ? 'Update' : 'Add')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    )
  }
)
