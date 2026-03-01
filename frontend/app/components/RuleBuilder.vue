<template>
  <div class="p-4 rounded-lg border border-border bg-muted space-y-4">
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-3">
      <!-- ① Service Instance -->
      <div class="space-y-1.5">
        <UiLabel class="text-xs text-muted-foreground">Service</UiLabel>
        <UiSelect v-model="form.integrationId" @update:model-value="onServiceChange">
          <UiSelectTrigger>
            <UiSelectValue placeholder="Select service…" />
          </UiSelectTrigger>
          <UiSelectContent>
            <UiSelectItem
              v-for="svc in arrIntegrations"
              :key="svc.id"
              :value="String(svc.id)"
            >
              {{ svc.name }}
            </UiSelectItem>
          </UiSelectContent>
        </UiSelect>
      </div>

      <!-- ② Action (Field) -->
      <div class="space-y-1.5">
        <UiLabel class="text-xs text-muted-foreground">Action</UiLabel>
        <UiSelect v-model="form.field" :disabled="!form.integrationId" @update:model-value="onFieldChange">
          <UiSelectTrigger>
            <UiSelectValue placeholder="Select field…" />
          </UiSelectTrigger>
          <UiSelectContent>
            <UiSelectItem
              v-for="f in fields"
              :key="f.field"
              :value="f.field"
            >
              {{ f.label }}
            </UiSelectItem>
          </UiSelectContent>
        </UiSelect>
      </div>

      <!-- ③ Operator -->
      <div class="space-y-1.5">
        <UiLabel class="text-xs text-muted-foreground">Operator</UiLabel>
        <UiSelect v-model="form.operator" :disabled="!form.field">
          <UiSelectTrigger>
            <UiSelectValue placeholder="Select…" />
          </UiSelectTrigger>
          <UiSelectContent>
            <UiSelectItem
              v-for="op in availableOperators"
              :key="op.value"
              :value="op.value"
            >
              {{ op.label }}
            </UiSelectItem>
          </UiSelectContent>
        </UiSelect>
      </div>

      <!-- ④ Value -->
      <div class="space-y-1.5">
        <UiLabel class="text-xs text-muted-foreground">Value</UiLabel>
        <UiInput
          v-model="form.value"
          :disabled="!form.operator"
          :type="selectedFieldType === 'number' ? 'number' : 'text'"
          :placeholder="valuePlaceholder"
        />
      </div>

      <!-- ⑤ Effect -->
      <div class="space-y-1.5">
        <UiLabel class="text-xs text-muted-foreground">Effect</UiLabel>
        <UiSelect v-model="form.effect" :disabled="!form.value">
          <UiSelectTrigger>
            <UiSelectValue placeholder="Select effect…" />
          </UiSelectTrigger>
          <UiSelectContent>
            <UiSelectItem
              v-for="eff in effectOptions"
              :key="eff.value"
              :value="eff.value"
            >
              <span class="inline-flex items-center gap-2">
                <span class="w-2 h-2 rounded-full shrink-0" :class="eff.colorClass" />
                {{ eff.label }}
              </span>
            </UiSelectItem>
          </UiSelectContent>
        </UiSelect>
      </div>
    </div>

    <div class="flex items-center gap-3">
      <UiButton size="sm" :disabled="!isFormValid" @click="submitRule">
        Save Rule
      </UiButton>
      <UiButton variant="ghost" size="sm" @click="$emit('cancel')">
        Cancel
      </UiButton>
    </div>
  </div>
</template>

<script setup lang="ts">
interface Integration {
  id: number
  type: string
  name: string
  enabled: boolean
}

interface FieldDef {
  field: string
  label: string
  type: string
  operators: string[]
}

const props = defineProps<{
  integrations: Integration[]
}>()

const emit = defineEmits<{
  (e: 'save', rule: {
    integrationId: number
    field: string
    operator: string
    value: string
    effect: string
  }): void
  (e: 'cancel'): void
}>()

const api = useApi()

// Only show *arr integrations (not enrichment services like Plex/Tautulli/Overseerr)
const arrTypes = ['sonarr', 'radarr', 'lidarr', 'readarr']
const arrIntegrations = computed(() =>
  props.integrations.filter(i => i.enabled && arrTypes.includes(i.type))
)

// Operator labels mapping
const operatorLabels: Record<string, string> = {
  '==': 'is',
  '!=': 'is not',
  'contains': 'contains',
  '!contains': 'does not contain',
  '>': 'more than',
  '>=': 'at least',
  '<': 'less than',
  '<=': 'at most',
}

// Effect options with color coding
const effectOptions = [
  { value: 'always_keep', label: 'Always keep', colorClass: 'bg-emerald-500' },
  { value: 'prefer_keep', label: 'Prefer to keep', colorClass: 'bg-emerald-400' },
  { value: 'lean_keep', label: 'Lean toward keeping', colorClass: 'bg-emerald-300' },
  { value: 'lean_remove', label: 'Lean toward removing', colorClass: 'bg-amber-400' },
  { value: 'prefer_remove', label: 'Prefer to remove', colorClass: 'bg-red-400' },
  { value: 'always_remove', label: 'Always remove', colorClass: 'bg-red-500' },
]

// Form state
const form = reactive({
  integrationId: '',
  field: '',
  operator: '',
  value: '',
  effect: '',
})

// Dynamic fields fetched based on selected service type
const fields = ref<FieldDef[]>([])

// Get the service type from the selected integration
const selectedServiceType = computed(() => {
  if (!form.integrationId) return ''
  const svc = arrIntegrations.value.find(i => String(i.id) === form.integrationId)
  return svc?.type ?? ''
})

// Get the selected field definition
const selectedField = computed(() =>
  fields.value.find(f => f.field === form.field)
)

const selectedFieldType = computed(() => selectedField.value?.type ?? 'string')

// Available operators for the selected field with friendly labels
const availableOperators = computed(() => {
  if (!selectedField.value) return []
  return selectedField.value.operators.map(op => ({
    value: op,
    label: operatorLabels[op] ?? op,
  }))
})

// Placeholder for value input
const valuePlaceholder = computed(() => {
  if (!form.field) return 'Value'
  switch (form.field) {
    case 'title': return 'e.g., Breaking Bad'
    case 'quality': return 'e.g., HD-1080p'
    case 'tag': return 'e.g., anime'
    case 'genre': return 'e.g., Action'
    case 'rating': return 'e.g., 7.5'
    case 'sizebytes': return 'Bytes'
    case 'timeinlibrary': return 'Days'
    case 'year': return 'e.g., 2020'
    case 'language': return 'e.g., English'
    case 'monitored': return 'true / false'
    case 'availability': return 'e.g., ended'
    case 'seasoncount': return 'e.g., 5'
    case 'episodecount': return 'e.g., 100'
    case 'playcount': return 'e.g., 0'
    case 'requestcount': return 'e.g., 3'
    case 'requested': return 'true / false'
    default: return 'Value'
  }
})

const isFormValid = computed(() =>
  form.integrationId !== '' &&
  form.field !== '' &&
  form.operator !== '' &&
  form.value !== '' &&
  form.effect !== ''
)

// Cascade: when service changes, reset downstream fields and fetch field definitions
async function onServiceChange() {
  form.field = ''
  form.operator = ''
  form.value = ''
  form.effect = ''

  if (!form.integrationId) {
    fields.value = []
    return
  }

  try {
    const serviceType = selectedServiceType.value
    fields.value = await api(`/api/v1/rule-fields?service_type=${serviceType}`) as FieldDef[]
  } catch {
    fields.value = []
  }
}

// Cascade: when action (field) changes, reset operator and value
function onFieldChange() {
  form.operator = ''
  form.value = ''
  form.effect = ''
}

function submitRule() {
  if (!isFormValid.value) return
  emit('save', {
    integrationId: Number(form.integrationId),
    field: form.field,
    operator: form.operator,
    value: form.value,
    effect: form.effect,
  })
  // Reset form
  form.integrationId = ''
  form.field = ''
  form.operator = ''
  form.value = ''
  form.effect = ''
  fields.value = []
}
</script>
