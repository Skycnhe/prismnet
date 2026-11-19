<script setup lang="ts">
import type { PrismStats } from '~/types/prism'
import { useConfigQuery, useUpdateConfigMutation, useVersionQuery } from '~/composables/useQueries'

defineProps<{ stats: PrismStats | null }>()

const { data: cfg } = useConfigQuery()
const { data: version } = useVersionQuery()
const update = useUpdateConfigMutation()
const connectionsStore = useConnectionsStore()

const MODES = [
  { key: 'rule', label: '规则' },
  { key: 'global', label: '全局' },
  { key: 'direct', label: '直连' },
]
const mode = computed(() => String(cfg.value?.mode ?? '').toLowerCase())
const setMode = (m: string) => {
  if (m !== mode.value)
    update.mutate({ key: 'mode', value: m })
}

const method = computed(() => {
  const c = cfg.value
  if (!c)
    return { main: '—', sub: '' }
  if (c.tun?.enable)
    return { main: 'TUN', sub: [c.tun.stack, c.tun.device].filter(Boolean).join(' · ') }
  if (c['tproxy-port'])
    return { main: 'TProxy', sub: `端口 ${c['tproxy-port']}` }
  if (c['redir-port'])
    return { main: 'Redir', sub: `端口 ${c['redir-port']}` }
  return { main: 'Mixed', sub: `端口 ${c['mixed-port'] || c.port || '—'}` }
})
const coreName = computed(() => (version.value ? `mihomo ${version.value}` : 'mihomo'))

const conns = computed(() => connectionsStore.activeConnections.length)
const devices = computed(() => new Set(connectionsStore.activeConnections.map((c) => c.metadata.sourceIP).filter(Boolean)).size)
</script>

<template>
  <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
    <div class="prism-card p-3 sm:p-4">
      <div class="prism-muted mb-2 text-sm">
        代理模式
      </div>
      <div class="prism-sub flex p-1">
        <button
          v-for="m in MODES"
          :key="m.key"
          type="button"
          class="flex-1 rounded-lg py-1.5 text-sm font-semibold transition-colors sm:text-base"
          :class="mode === m.key ? 'bg-info/80 text-info-content shadow' : 'hover:bg-base-content/10'"
          :disabled="update.isPending.value"
          @click="setMode(m.key)"
        >
          {{ m.label }}
        </button>
      </div>
    </div>
    <div class="prism-card p-3 sm:p-4">
      <div class="prism-muted mb-2 text-sm">
        代理方式
      </div>
      <div class="flex items-baseline justify-between gap-2">
        <span class="prism-num text-2xl sm:text-3xl">{{ method.main }}</span>
        <span class="prism-muted truncate text-xs sm:text-sm" :title="coreName">{{ method.sub }}</span>
      </div>
      <div class="prism-muted mt-1 truncate text-xs">
        {{ coreName }}
      </div>
    </div>
    <div class="prism-card p-3 sm:p-4">
      <div class="prism-muted mb-2 text-sm">
        活动连接
      </div>
      <div class="flex items-baseline justify-between gap-2">
        <NuxtLink to="/connections" class="prism-num text-2xl sm:text-3xl">
          {{ prismNum(conns) }}
        </NuxtLink>
        <span class="prism-muted text-xs sm:text-sm">{{ devices }} 台设备</span>
      </div>
    </div>
    <div class="prism-card p-3 sm:p-4">
      <div class="prism-muted mb-2 text-sm">
        今日拦截广告
      </div>
      <div class="flex items-baseline justify-between gap-2">
        <span class="prism-num text-2xl sm:text-3xl">{{ stats ? prismNum(stats.stats.today) : '—' }}</span>
        <span class="prism-muted truncate text-xs sm:text-sm">规则 {{ stats ? prismNum(stats.adblock.rules) : '—' }} 条</span>
      </div>
    </div>
  </div>
</template>
