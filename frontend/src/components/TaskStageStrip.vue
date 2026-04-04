<script setup>
import { computed } from 'vue'

const props = defineProps({
  task: {
    type: Object,
    required: true,
  },
  stageDefinitions: {
    type: Array,
    default: () => [],
  },
  currentView: {
    type: String,
    default: '',
  },
  locale: {
    type: String,
    default: 'zh',
  },
  t: {
    type: Function,
    required: true,
  },
})

const emit = defineEmits(['select-stage'])

const stageCards = computed(() => {
  return props.stageDefinitions.map(definition => {
    const stage = props.task?.stages?.find(item => item.name === definition.key) || null
    const parsed = parseResultArray(stage?.output_json || stage?.result)
    const hasRawPayload = Boolean(
      typeof stage?.result === 'string' && stage.result.trim() ||
      Array.isArray(stage?.output_json) ||
      typeof stage?.output_json === 'string' && stage.output_json.trim() && stage.output_json !== '{}' && stage.output_json !== 'null' ||
      stage?.output_json && typeof stage.output_json === 'object' && Object.keys(stage.output_json).length > 0
    )
    const findingCount = Array.isArray(parsed) ? parsed.length : null
    const status = stage?.status || 'pending'

    let detail = props.t('taskStrip.waitingToRun')
    if (status === 'running') {
      detail = props.t('taskStrip.auditInProgress')
    } else if (status === 'failed') {
      detail = props.t('taskStrip.lastRunFailed')
    } else if (status === 'completed' && findingCount === 0) {
      detail = props.t('taskStrip.cleanResult')
    } else if (status === 'completed' && findingCount !== null) {
      detail = props.t('taskStrip.findingsCount', { count: findingCount })
    } else if (status === 'completed' && hasRawPayload) {
      detail = props.t('taskStrip.rawExportReady')
    }

    return {
      ...definition,
      stage,
      status,
      findingCount,
      detail,
      updatedAt: stage?.updated_at ? new Date(stage.updated_at).toLocaleString(props.locale === 'en' ? 'en-US' : 'zh-CN') : '',
      active: props.currentView === definition.view,
    }
  })
})

function displayStatus(status) {
  return props.t(`status.${String(status || '').trim().toLowerCase() || 'pending'}`)
}

function parseResultArray(raw) {
  if (!raw) return null

  try {
    if (Array.isArray(raw)) return raw
    if (raw && typeof raw === 'object') return null

    let text = String(raw).trim()
    if (!text) return null

    if (text.startsWith('```json')) {
      text = text.replace(/^```json\s*/, '').replace(/\s*```$/, '')
    } else if (text.startsWith('```')) {
      text = text.replace(/^```\s*/, '').replace(/\s*```$/, '')
    }

    const start = text.indexOf('[')
    const end = text.lastIndexOf(']')
    if (start !== -1 && end !== -1 && end > start) {
      text = text.slice(start, end + 1)
    }

    const parsed = JSON.parse(text)
    return Array.isArray(parsed) ? parsed : null
  } catch {
    return null
  }
}

function statusBadgeClass(status) {
  switch (status) {
    case 'running':
      return 'bg-amber-500/10 text-amber-300 border-amber-500/30'
    case 'completed':
      return 'bg-emerald-500/10 text-emerald-300 border-emerald-500/30'
    case 'failed':
      return 'bg-rose-500/10 text-rose-300 border-rose-500/30'
    default:
      return 'bg-slate-500/10 text-slate-300 border-slate-500/30'
  }
}
</script>

<template>
  <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
    <button
      v-for="card in stageCards"
      :key="card.key"
      type="button"
      @click="emit('select-stage', card.view)"
      :class="[
        'group relative overflow-hidden rounded-2xl border bg-black/25 p-5 text-left transition-all duration-300 hover:-translate-y-0.5 hover:bg-white/5',
        card.cardClass,
        card.active ? 'ring-1 ring-white/20 border-white/20' : 'border-white/10'
      ]"
    >
      <div :class="['absolute inset-0 bg-gradient-to-br opacity-80', card.gradientClass]"></div>
      <div class="relative z-10">
        <div class="flex items-start justify-between gap-3">
          <div :class="['rounded-2xl border p-3', card.iconClass]">
            <component :is="card.icon" class="w-5 h-5" />
          </div>
          <span :class="['rounded-full border px-2.5 py-1 text-[11px] font-semibold uppercase tracking-wide', statusBadgeClass(card.status)]">
            {{ displayStatus(card.status) }}
          </span>
        </div>

        <div class="mt-4">
          <div class="text-sm font-semibold text-white">{{ card.label }}</div>
          <div class="mt-2 text-sm text-slate-300">{{ card.detail }}</div>
          <div class="mt-3 text-xs text-slate-500 min-h-4">{{ card.updatedAt || t('taskStrip.noCompletedRunYet') }}</div>
        </div>
      </div>
    </button>
  </div>
</template>

