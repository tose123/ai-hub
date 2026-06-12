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
import { useEffect, useRef, useState, type DragEvent } from 'react'
import { useForm, type SubmitErrorHandler } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, GripVertical, KeyRound, Settings2, Trash2, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getUserModels, getUserGroups } from '@/lib/api'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { cn } from '@/lib/utils'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Combobox } from '@/components/ui/combobox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { DateTimePicker } from '@/components/datetime-picker'
import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
  sideDrawerSwitchItemClassName,
} from '@/components/drawer-layout'
import { MultiSelect } from '@/components/multi-select'
import { ModelMappingEditor } from '@/features/channels/components/model-mapping-editor'
import { getPricing } from '@/features/pricing/api'
import { createApiKey, updateApiKey, getApiKey } from '../api'
import { ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import {
  getApiKeyFormSchema,
  type ApiKeyFormValues,
  getApiKeyFormDefaultValues,
  transformFormDataToPayload,
  transformApiKeyToFormDefaults,
} from '../lib'
import type { ApiKey } from '../types'
import {
  GroupRatioBadge,
  ApiKeyGroupCombobox,
  type ApiKeyGroupOption,
} from './api-key-group-combobox'
import { useApiKeys } from './api-keys-provider'

type ApiKeyMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: ApiKey
}

export function ApiKeysMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: ApiKeyMutateDrawerProps) {
  const { t } = useTranslation()
  const isUpdate = !!currentRow
  const { triggerRefresh } = useApiKeys()
  const { status } = useStatus()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [draggedAutoGroup, setDraggedAutoGroup] = useState<string | null>(null)
  const [autoGroupError, setAutoGroupError] = useState('')
  const defaultUseAutoGroup = status?.default_use_auto_group === true
  const userEditedOverrideRef = useRef(false)
  const prevModeRef = useRef<string>('')
  const [autoGroupsOverrideEdited, setAutoGroupsOverrideEdited] = useState(false)

  // Fetch models
  const { data: modelsData } = useQuery({
    queryKey: ['user-models'],
    queryFn: getUserModels,
    staleTime: 5 * 60 * 1000, // Cache for 5 minutes
  })

  // Fetch groups
  const { data: groupsData } = useQuery({
    queryKey: ['user-groups'],
    queryFn: getUserGroups,
    staleTime: 5 * 60 * 1000,
  })

  const { data: pricingData } = useQuery({
    queryKey: ['pricing-auto-groups'],
    queryFn: getPricing,
    staleTime: 5 * 60 * 1000,
  })

  const models = modelsData?.data || []
  const groupsRaw = groupsData?.data || {}
  const groups: ApiKeyGroupOption[] = Object.entries(groupsRaw).map(
    ([key, info]) => ({
      value: key,
      label: key,
      desc: info.desc || key,
      ratio: info.ratio,
    })
  )
  const backendHasAuto = groups.some((g) => g.value === 'auto')
  const pricingAutoGroups = (pricingData?.auto_groups || []).filter(Boolean)
  const schema = getApiKeyFormSchema(t)

  const form = useForm<ApiKeyFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getApiKeyFormDefaultValues(defaultUseAutoGroup),
  })

  // Load existing data when updating
  useEffect(() => {
    if (open && isUpdate && currentRow) {
      getApiKey(currentRow.id).then((result) => {
        if (result.success && result.data) {
          setAutoGroupsOverrideEdited(result.data.auto_groups_override?.length > 0)
          userEditedOverrideRef.current = false
          form.reset(transformApiKeyToFormDefaults(result.data, pricingAutoGroups))
        }
      })
    } else if (open && !isUpdate) {
      userEditedOverrideRef.current = false
      prevModeRef.current = ''
      form.reset(
        getApiKeyFormDefaultValues(defaultUseAutoGroup && backendHasAuto)
      )
    }
  }, [open, isUpdate, currentRow, form, defaultUseAutoGroup, backendHasAuto])

  useEffect(() => {
    if (!open) return
    const group = form.watch('group')
    if (prevModeRef.current !== 'auto' && group === 'auto') {
      const current = form.getValues('auto_groups_override') || []
      const hasCurrent = current.length > 0
      if (
        !userEditedOverrideRef.current &&
        !hasCurrent &&
        pricingAutoGroups.length > 0
      ) {
        setAutoGroupError('')
        form.setValue('auto_groups_override', pricingAutoGroups, {
          shouldDirty: true,
          shouldValidate: true,
        })
      }
    }
    prevModeRef.current = group || ''
  }, [open, form])

  const availableOverrideGroups = groups.filter((g) => g.value !== 'auto')
  const availableOverrideGroupNames = availableOverrideGroups.map(
    (g) => g.value
  )

  const autoGroupsOverride = form.watch('auto_groups_override') || []

  const appendAutoGroup = (value: string) => {
    const next = value.trim()
    if (!next) {
      setAutoGroupError(t('Please select a group'))
      return
    }
    if (!availableOverrideGroupNames.includes(next)) {
      setAutoGroupError(t('Invalid group in auto groups override'))
      return
    }
    if (autoGroupsOverride.includes(next)) {
      setAutoGroupError(t('Group already exists in auto groups override'))
      return
    }
    userEditedOverrideRef.current = true
    setAutoGroupError('')
    form.clearErrors('auto_groups_override')
    form.setValue('auto_groups_override', [...autoGroupsOverride, next], {
      shouldDirty: true,
      shouldValidate: true,
    })
    setAutoGroupsOverrideEdited(true)
  }

  const deleteAutoGroup = (index: number) => {
    userEditedOverrideRef.current = true
    setAutoGroupError('')
    form.clearErrors('auto_groups_override')
    form.setValue(
      'auto_groups_override',
      autoGroupsOverride.filter((_, i) => i !== index),
      {
        shouldDirty: true,
        shouldValidate: true,
      }
    )
    setAutoGroupsOverrideEdited(true)
  }

  const handleAutoGroupDrop = (targetValue: string) => {
    if (!draggedAutoGroup || draggedAutoGroup === targetValue) {
      return
    }
    const sourceIndex = autoGroupsOverride.indexOf(draggedAutoGroup)
    const targetIndex = autoGroupsOverride.indexOf(targetValue)
    if (sourceIndex === -1 || targetIndex === -1) {
      return
    }
    const reordered = [...autoGroupsOverride]
    const [moved] = reordered.splice(sourceIndex, 1)
    reordered.splice(targetIndex, 0, moved)
    userEditedOverrideRef.current = true
    setAutoGroupError('')
    form.clearErrors('auto_groups_override')
    form.setValue('auto_groups_override', reordered, {
      shouldDirty: true,
      shouldValidate: true,
    })
    setAutoGroupsOverrideEdited(true)
  }

  // Correct group after groups load: if the form value is not in available groups, fall back
  useEffect(() => {
    if (groups.length === 0) return
    const currentGroup = form.getValues('group')
    if (currentGroup && !groups.some((g) => g.value === currentGroup)) {
      const fallback =
        groups.find((g) => g.value === 'default')?.value ??
        groups[0]?.value ??
        ''
      form.setValue('group', fallback)
      if (currentGroup === 'auto') {
        form.setValue('cross_group_retry', true)
      }
    }
  }, [groups, form])

  const onSubmit = async (data: ApiKeyFormValues) => {
    setIsSubmitting(true)
    try {
      const basePayload = transformFormDataToPayload(data)

      if (data.group === 'auto') {
        if (!autoGroupsOverrideEdited) basePayload.auto_groups_override = [];
        const normalized = (data.auto_groups_override || []).filter(Boolean)
        const hasDuplicate = new Set(normalized).size !== normalized.length
        const hasInvalid = normalized.some(
          (groupValue) =>
            groupValue === 'auto' ||
            !availableOverrideGroupNames.includes(groupValue)
        )
        if (hasDuplicate || hasInvalid) {
          form.setError('auto_groups_override', {
            type: 'manual',
            message: t(
              'Auto groups override must be non-empty, unique, and only contain valid non-auto groups'
            ),
          })
          setAutoGroupError(
            t(
              'Auto groups override must be non-empty, unique, and only contain valid non-auto groups'
            )
          )
          setIsSubmitting(false)
          return
        }
      }

      if (isUpdate && currentRow) {
        const result = await updateApiKey({
          ...basePayload,
          id: currentRow.id,
        })
        if (result.success) {
          toast.success(t(SUCCESS_MESSAGES.API_KEY_UPDATED))
          onOpenChange(false)
          triggerRefresh()
        } else {
          toast.error(result.message || t(ERROR_MESSAGES.UPDATE_FAILED))
        }
      } else {
        // Create mode - handle batch creation
        const count = data.tokenCount || 1
        let successCount = 0

        for (let i = 0; i < count; i++) {
          const result = await createApiKey({
            ...basePayload,
            name:
              i === 0 && data.name
                ? data.name
                : `${data.name || 'default'}-${Math.random().toString(36).slice(2, 8)}`,
          })
          if (result.success) {
            successCount++
          } else {
            toast.error(result.message || t(ERROR_MESSAGES.CREATE_FAILED))
            break
          }
        }

        if (successCount > 0) {
          toast.success(
            t('Successfully created {{count}} API Key(s)', {
              count: successCount,
            })
          )
          onOpenChange(false)
          triggerRefresh()
        }
      }
    } catch (_error) {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsSubmitting(false)
    }
  }

  const onInvalid: SubmitErrorHandler<ApiKeyFormValues> = () => {
    toast.error(t('Please fix the highlighted fields before saving'))
  }

  const handleSetExpiry = (months: number, days: number, hours: number) => {
    if (months === 0 && days === 0 && hours === 0) {
      form.setValue('expired_time', undefined)
      return
    }

    const now = new Date()
    now.setMonth(now.getMonth() + months)
    now.setDate(now.getDate() + days)
    now.setHours(now.getHours() + hours)

    form.setValue('expired_time', now)
  }

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const quotaLabel = t('Quota ({{currency}})', { currency: currencyLabel })
  const quotaPlaceholder = tokensOnly
    ? t('Enter quota in tokens')
    : t('Enter quota in {{currency}}', { currency: currencyLabel })
  const selectedGroup = form.watch('group')
  const unlimitedQuota = form.watch('unlimited_quota')

  const getGroup = (name: string) => groups.find((g) => g.value === name)

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) {
          form.reset()
        }
      }}
    >
      <SheetContent
        className={sideDrawerContentClassName('max-w-none sm:!max-w-[620px]')}
      >
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isUpdate ? t('Update API Key') : t('Create API Key')}
          </SheetTitle>
          <SheetDescription>
            {isUpdate
              ? t('Update the API key by providing necessary info.')
              : t('Add a new API key by providing necessary info.')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='api-key-form'
            onSubmit={form.handleSubmit(onSubmit, onInvalid)}
            className={sideDrawerFormClassName('gap-5')}
          >
            <SideDrawerSection>
              <SideDrawerSectionHeader
                title={t('Basic Information')}
                description={t('Set API key basic information')}
                icon={<KeyRound className='size-4' />}
              />
              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Name')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder={t('Enter a name')} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='group'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Group')}</FormLabel>
                    <FormControl>
                      <ApiKeyGroupCombobox
                        options={groups}
                        value={field.value}
                        onValueChange={field.onChange}
                        placeholder={t('Select a group')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {selectedGroup === 'auto' && (
                <>
                  <FormField
                    control={form.control}
                    name='auto_groups_override'
                    render={({ field }) => {
                      const options = availableOverrideGroups
                        .filter((g) => !autoGroupsOverride.includes(g.value))
                        .map((g) => ({
                          value: g.value,
                          label: <div className='my-2'>
                            <div className='flex'>
                              <GroupRatioBadge ratio={g.ratio} />
                              <div className='ml-2'>{g.value}</div>
                            </div>
                            <span className='mt-1 text-muted-foreground block truncate text-[11px] sm:text-xs'>{g.desc}</span>
                          </div>,
                        }))

                      return (
                        <FormItem>
                          <div className='mb-2'>
                            <FormLabel>{t('Auto groups override')}</FormLabel>
                            <FormDescription>
                              {t(
                                'Configure ordered fallback groups for this key when using auto mode.'
                              )}
                            </FormDescription>
                          </div>
                          <FormControl>
                            <Combobox
                              options={options}
                              value=''
                              onValueChange={(value) => {
                                if (!value) return
                                appendAutoGroup(value)
                              }}
                              placeholder={t('Select a group')}
                              searchPlaceholder={t('Select a group')}
                              emptyText={t('No available groups can be added')}
                            />
                          </FormControl>

                          <ul className='mt-3 space-y-2'>
                            {field.value?.map((groupValue, index) => {
                              const group = getGroup(groupValue)
                              return (
                                <li
                                  key={groupValue}
                                  draggable={autoGroupsOverride.length > 1}
                                  onDragStart={(
                                    e: DragEvent<HTMLLIElement>
                                  ) => {
                                    e.dataTransfer.effectAllowed = 'move'
                                    setDraggedAutoGroup(groupValue)
                                  }}
                                  onDragOver={(e: DragEvent<HTMLLIElement>) => {
                                    e.preventDefault()
                                    e.dataTransfer.dropEffect = 'move'
                                  }}
                                  onDrop={(e: DragEvent<HTMLLIElement>) => {
                                    e.preventDefault()
                                    handleAutoGroupDrop(groupValue)
                                    setDraggedAutoGroup(null)
                                  }}
                                  onDragEnd={() => setDraggedAutoGroup(null)}
                                  className='bg-muted/40 flex w-full items-center justify-between rounded-md border px-2 py-1.5 text-left'
                                >
                                  <div className='flex items-center gap-2'>
                                    <GripVertical className='text-muted-foreground size-4' />
                                    <GroupRatioBadge ratio={group?.ratio || 0} />
                                    <span className='text-sm'>
                                      {groupValue}
                                    </span>
                                  </div>
                                  <Button
                                    type='button'
                                    size='icon'
                                    variant='ghost'
                                    onMouseDown={(e) => e.stopPropagation()}
                                    onClick={() => deleteAutoGroup(index)}
                                  >
                                    <Trash2 className='size-4' />
                                  </Button>
                                </li>
                              )
                            })}
                          </ul>
                          <FormMessage />
                          {autoGroupError ? (
                            <p className='text-destructive mt-2 text-xs'>
                              {autoGroupError}
                            </p>
                          ) : null}
                        </FormItem>
                      )
                    }}
                  />

                  <FormField
                    control={form.control}
                    name='cross_group_retry'
                    render={({ field }) => (
                      <FormItem className={sideDrawerSwitchItemClassName()}>
                        <div className='flex flex-col gap-0.5'>
                          <FormLabel className='text-sm'>
                            {t('Cross-group retry')}
                          </FormLabel>
                          <FormDescription className='line-clamp-2 text-xs sm:line-clamp-none'>
                            {t(
                              'When enabled, if channels in the current group fail, it will try channels in the next group in order.'
                            )}
                          </FormDescription>
                        </div>
                        <FormControl>
                          <Switch
                            checked={!!field.value}
                            onCheckedChange={field.onChange}
                          />
                        </FormControl>
                      </FormItem>
                    )}
                  />
                </>
              )}

              <FormField
                control={form.control}
                name='expired_time'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Expiration Time')}</FormLabel>
                    <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center'>
                      <FormControl>
                        <DateTimePicker
                          value={field.value}
                          onChange={field.onChange}
                          placeholder={t('Never expires')}
                          className='min-w-0 [&_input[type=time]]:w-24 sm:[&_input[type=time]]:w-32'
                        />
                      </FormControl>
                      <div className='grid grid-cols-4 gap-2 sm:flex'>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 0, 0)}
                        >
                          {t('Never')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(1, 0, 0)}
                        >
                          {t('1 Month')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 1, 0)}
                        >
                          {t('1 Day')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 0, 1)}
                        >
                          {t('1 Hour')}
                        </Button>
                      </div>
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {!isUpdate && (
                <FormField
                  control={form.control}
                  name='tokenCount'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Quantity')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min='1'
                          placeholder={t('Number of keys to create')}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10) || 1)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Create multiple API keys at once (random suffix will be added to names)'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </SideDrawerSection>

            <SideDrawerSection>
              <SideDrawerSectionHeader
                title={t('Quota Settings')}
                description={t('Set quota amount and limits')}
                icon={<WalletCards className='size-4' />}
              />
              {!unlimitedQuota && (
                <FormField
                  control={form.control}
                  name='remain_quota_dollars'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{quotaLabel}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          step={tokensOnly ? 1 : 0.01}
                          placeholder={quotaPlaceholder}
                          onChange={(e) =>
                            field.onChange(parseFloat(e.target.value) || 0)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {tokensOnly
                          ? t('Enter the quota amount in tokens')
                          : t('Enter the quota amount in {{currency}}', {
                              currency: currencyLabel,
                            })}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              <FormField
                control={form.control}
                name='unlimited_quota'
                render={({ field }) => (
                  <FormItem className={sideDrawerSwitchItemClassName()}>
                    <div className='flex flex-col gap-0.5'>
                      <FormLabel className='text-sm'>
                        {t('Unlimited Quota')}
                      </FormLabel>
                      <FormDescription className='text-xs'>
                        {t('Enable unlimited quota for this API key')}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </SideDrawerSection>

            <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
              <SideDrawerSection>
                <CollapsibleTrigger
                  render={
                    <button
                      type='button'
                      className='hover:bg-muted/40 flex w-full items-center gap-3 rounded-md py-1.5 text-left transition-colors'
                    />
                  }
                >
                  <SideDrawerSectionHeader
                    className='flex-1'
                    title={t('Advanced Settings')}
                    description={t('Set API key access restrictions')}
                    icon={<Settings2 className='size-4' />}
                  />
                  <ChevronDown
                    className={cn(
                      'text-muted-foreground size-4 shrink-0 transition-transform',
                      advancedOpen && 'rotate-180'
                    )}
                  />
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <div className='flex flex-col gap-4 pt-2'>
                    <FormField
                      control={form.control}
                      name='model_limits'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Model Limits')}</FormLabel>
                          <FormControl>
                            <MultiSelect
                              options={models.map((m) => ({
                                label: m,
                                value: m,
                              }))}
                              selected={field.value}
                              onChange={field.onChange}
                              placeholder={t(
                                'Select models (empty for allow all)'
                              )}
                            />
                          </FormControl>
                          <FormDescription>
                            {t('Limit which models can be used with this key')}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <FormField
                      control={form.control}
                      name='model_mapping'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Model Mapping')}</FormLabel>
                          <FormControl>
                            <ModelMappingEditor
                              value={field.value || ''}
                              onChange={field.onChange}
                              sourceModelOptions={models}
                              targetModelOptions={models}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <FormField
                      control={form.control}
                      name='allow_ips'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>
                            {t('IP Whitelist (supports CIDR)')}
                          </FormLabel>
                          <FormControl>
                            <Textarea
                              {...field}
                              className='min-h-20 resize-none'
                              placeholder={t(
                                'One IP per line (empty for no restriction)'
                              )}
                              rows={3}
                            />
                          </FormControl>
                          <FormDescription>
                            {t(
                              'Do not over-trust this feature. IP may be spoofed. Please use with nginx, CDN and other gateways.'
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>
                </CollapsibleContent>
              </SideDrawerSection>
            </Collapsible>
          </form>
        </Form>
        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose
            render={<Button variant='outline' className='w-full sm:w-auto' />}
          >
            {t('Close')}
          </SheetClose>
          <Button
            type='button'
            onClick={form.handleSubmit(onSubmit, onInvalid)}
            disabled={isSubmitting}
            className='w-full sm:w-auto'
          >
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
