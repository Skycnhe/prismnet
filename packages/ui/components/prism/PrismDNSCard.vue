<script setup lang="ts">
import type { PrismStats } from '~/types/prism'

const props = defineProps<{ stats: PrismStats | null }>()
const r = computed(() => props.stats?.stats.dns_result ?? { total: 0, cache: 0, upstream: 0, other: 0 })

const parts = computed(() => [
  { key: 'cache', label: '缓存命中', n: r.value.cache, color: 'var(--color-success)' },
  { key: 'upstream', label: '上游应答', n: r.value.upstream, color: 'var(--color-info)' },
  { key: 'other', label: 'Hosts / Fake-IP', n: r.value.other, color: 'var(--color-warning)' },
])

const R = 42
const C = 2 * Math.PI * R
const arcs = computed(() => {
  const total = Math.max(1, r.value.total)
  let off = 0
  return parts.value.map((p) => {
    const len = (p.n / total) * C
    const a = { ...p, dash: `${len} ${C - len}`, offset: -off }
    off += len
    return a
  })
})
const pct = (n: number) => (r.value.total ? `${((100 * n) / r.value.total).toFixed(1)}%` : '0.0%')
</script>

<template>
  <div class="prism-card flex flex-col gap-3 p-4">
    <div class="flex items-start justify-between gap-2">
      <div>
        <div class="prism-title">
          DNS 查询结果
        </div>
        <div class="prism-muted text-xs sm:text-sm">
          最近 100 条查询
        </div>
      </div>
      <NuxtLink to="/config" class="prism-sub btn btn-sm border-0">
        DNS
      </NuxtLink>
    </div>
    <div class="flex items-center gap-5">
      <svg viewBox="0 0 100 100" class="size-32 shrink-0 -rotate-90">
        <circle cx="50" cy="50" :r="R" fill="none" stroke="color-mix(in oklab, var(--color-base-content) 10%, transparent)" stroke-width="12" />
        <circle
          v-for="a in arcs"
          :key="a.key"
          cx="50"
          cy="50"
          :r="R"
          fill="none"
          :stroke="a.color"
          stroke-width="12"
          :stroke-dasharray="a.dash"
          :stroke-dashoffset="a.offset"
        />
        <text x="50" y="50" class="rotate-90" transform-origin="50 50" text-anchor="middle" dominant-baseline="central" fill="currentColor" font-size="20" font-weight="700">{{ r.total }}</text>
      </svg>
      <ul class="flex min-w-0 flex-1 flex-col gap-2 text-sm">
        <li v-for="p in parts" :key="p.key" class="flex items-center gap-2">
          <span class="size-3 shrink-0 rounded" :style="{ background: p.color }" />
          <span class="min-w-0 flex-1 truncate">{{ p.label }}</span>
          <b class="prism-num">{{ p.n }}</b>
          <span class="prism-muted w-12 text-right">{{ pct(p.n) }}</span>
        </li>
      </ul>
    </div>
    <p v-if="stats && !stats.stats.connected" class="text-xs text-warning">
      未连上核心日志流，统计暂停
    </p>
  </div>
</template>
