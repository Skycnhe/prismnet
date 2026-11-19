<script setup lang="ts">
import type { PrismStats } from '~/types/prism'
import { toast } from 'vue-sonner'

const props = defineProps<{ stats: PrismStats | null }>()
const emit = defineEmits<{ changed: [] }>()
const api = usePrismApi()
const toggling = ref(false)

const s = computed(() => props.stats?.stats)
const enabled = computed(() => !!props.stats?.adblock.enabled)
const pct = computed(() => {
  const r = s.value?.req24 ?? 0
  return r ? `${((100 * (s.value?.blk24 ?? 0)) / r).toFixed(1)}%` : '—'
})
const top5 = computed(() => (s.value?.top ?? []).slice(0, 5))
const topMax = computed(() => Math.max(1, ...top5.value.map((t) => t.count)))

async function toggle() {
  toggling.value = true
  try {
    await api.toggleAdblock(!enabled.value)
    toast.success(enabled.value ? '已关闭广告拦截' : '已开启广告拦截')
    emit('changed')
  }
  catch (e) {
    toast.error(`操作失败：${e instanceof Error ? e.message : e}`)
  }
  finally {
    toggling.value = false
  }
}
</script>

<template>
  <div class="prism-card flex flex-col gap-3 p-4">
    <div class="flex items-start justify-between gap-2">
      <div class="min-w-0">
        <div class="prism-title">
          域名过滤
        </div>
        <div class="prism-muted truncate text-xs sm:text-sm">
          最近 24 个小时段 · 核心日志统计的连接请求与广告拦截
        </div>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        <NuxtLink to="/prism" class="prism-sub btn btn-sm border-0">
          管理
        </NuxtLink>
        <input
          type="checkbox"
          class="toggle toggle-success"
          :checked="enabled"
          :disabled="toggling || !stats"
          title="广告拦截开关"
          @change.prevent="toggle"
        >
      </div>
    </div>
    <div class="grid grid-cols-3 gap-2">
      <div class="prism-sub flex min-w-0 flex-col p-2.5">
        <span class="prism-muted truncate text-xs sm:text-sm">平均解析耗时</span>
        <span class="prism-muted truncate text-[11px]">24 小时 · {{ prismNum(s?.dns.samples) }} 次采样</span>
        <span class="prism-num mt-1 text-xl sm:text-2xl">{{ s?.dns.avg ?? '—' }}<small class="prism-muted ml-0.5 text-xs font-normal">ms</small></span>
        <div class="mt-auto pt-2">
          <PrismSparkLine :values="s?.dns.series ?? []" color="var(--color-success)" :height="40" bars :min="0" />
        </div>
      </div>
      <div class="prism-sub flex min-w-0 flex-col p-2.5">
        <span class="prism-muted truncate text-xs sm:text-sm">连接请求</span>
        <span class="prism-num mt-1 text-xl sm:text-2xl">{{ prismNum(s?.req24) }}</span>
        <div class="mt-auto pt-2">
          <PrismSparkLine :values="(s?.hours ?? []).map(h => h.req)" color="var(--color-info)" :height="40" />
        </div>
      </div>
      <div class="prism-sub flex min-w-0 flex-col p-2.5">
        <span class="flex justify-between gap-1 text-xs sm:text-sm">
          <span class="prism-muted truncate">已拦截</span>
          <span class="text-warning">{{ pct }}</span>
        </span>
        <span class="prism-num mt-1 text-xl text-warning sm:text-2xl">{{ prismNum(s?.blk24) }}</span>
        <div class="mt-auto pt-2">
          <PrismSparkLine :values="(s?.hours ?? []).map(h => h.blk)" color="var(--color-warning)" :height="40" />
        </div>
      </div>
    </div>
    <div class="prism-sub p-3">
      <div class="prism-muted mb-1 text-sm">
        被拦截域名排行
      </div>
      <div v-if="!top5.length" class="prism-muted py-3 text-center text-sm">
        今天还没有拦截记录
      </div>
      <div v-for="t in top5" :key="t.domain" class="py-1.5">
        <div class="flex justify-between gap-2 text-sm">
          <span class="truncate">{{ t.domain }}</span>
          <span class="prism-num">{{ prismNum(t.count) }}</span>
        </div>
        <div class="mt-1 h-1 rounded-full bg-base-content/10">
          <div class="h-1 rounded-full bg-warning" :style="{ width: `${(100 * t.count) / topMax}%` }" />
        </div>
      </div>
    </div>
  </div>
</template>
