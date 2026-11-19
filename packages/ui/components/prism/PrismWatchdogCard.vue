<script setup lang="ts">
import type { PrismWatchdog } from '~/types/prism'
import dayjs from 'dayjs'

const props = withDefaults(defineProps<{ full?: boolean }>(), { full: false })
const api = usePrismApi()
const { data, error } = usePrismPoll<PrismWatchdog>(() => api.watchdog(), 15000)

const STATE: Record<string, { text: string, cls: string }> = {
  ok: { text: '正常', cls: 'text-success' },
  failing: { text: '异常', cls: 'text-warning' },
  bypass: { text: '已直连', cls: 'text-error' },
  disabled: { text: '已关闭', cls: 'prism-muted' },
}
const st = computed(() => STATE[data.value?.state ?? ''] ?? { text: '未知', cls: 'prism-muted' })
const events = computed(() => [...(data.value?.events ?? [])].reverse().slice(0, props.full ? 20 : 4))
const KIND: Record<string, string> = { restart: 'text-warning', bypass: 'text-error', recover: 'text-success', error: 'text-error' }
const fmt = (t: number) => dayjs.unix(t).format('MM/DD HH:mm:ss')
</script>

<template>
  <div class="prism-card flex flex-col gap-2 p-4">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <span class="prism-title">故障自动直连（看门狗）</span>
      <span class="flex items-center gap-2 text-sm">
        <span :class="st.cls">● {{ st.text }}</span>
        <span v-if="data?.last_check" class="prism-muted">上次检查 {{ dayjs.unix(data.last_check).format('HH:mm:ss') }}</span>
      </span>
    </div>
    <p class="prism-muted text-sm">
      每 {{ data?.interval ?? 30 }} 秒检查核心；无响应时自动重启，连续 {{ data?.threshold ?? 2 }} 次失败关闭透明代理让局域网直连，核心恢复后自动重新接管
    </p>
    <p v-if="data" class="prism-muted text-sm">
      最近 {{ data.events?.length ?? 0 }}/20 条记录：自动重启 <b class="text-base-content">{{ data.counts.restart }}</b> 次 · 切换直连 <b class="text-base-content">{{ data.counts.bypass }}</b> 次 · 恢复 <b class="text-base-content">{{ data.counts.recover }}</b> 次（满 20 条自动删除最旧的）
    </p>
    <div v-if="error" class="text-sm text-error">
      {{ error }}
    </div>
    <ul class="flex flex-col gap-1 text-sm">
      <li v-for="(e, i) in events" :key="i" class="flex gap-2">
        <span class="prism-muted prism-num shrink-0 font-normal">{{ fmt(e.t) }}</span>
        <span :class="KIND[e.kind]">{{ e.msg }}</span>
      </li>
    </ul>
  </div>
</template>
