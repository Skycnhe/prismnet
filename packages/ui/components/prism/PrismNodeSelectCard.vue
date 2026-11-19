<script setup lang="ts">
import type { Proxy } from '~/types'
import { IconAlertTriangle, IconRocket } from '@tabler/icons-vue'
import { toast } from 'vue-sonner'

const proxiesStore = useProxiesStore()
const testing = ref(false)

const groups = computed(() => proxiesStore.proxies.filter((p) => p.name !== 'GLOBAL'))
const groupMap = computed(() => new Map(proxiesStore.proxies.map((p) => [p.name, p])))

// 主选择组：成员里策略组最多的 Selector（通常是「🚀 节点选择」）
const main = computed<Proxy | undefined>(() => {
  let best: Proxy | undefined
  let bestN = 0
  for (const g of groups.value) {
    if (g.type !== 'Selector')
      continue
    const n = (g.all ?? []).filter((m) => groupMap.value.has(m)).length
    if (n > bestN) {
      best = g
      bestN = n
    }
  }
  return best
})

const tiles = computed(() => {
  const m = main.value
  if (!m)
    return groups.value
  return (m.all ?? []).map((n) => groupMap.value.get(n)).filter((g): g is Proxy => !!g)
})

const latencyOf = (g: Proxy): number => {
  if (g.type === 'LoadBalance') {
    const vals = (g.all ?? []).map((n) => proxiesStore.getLatencyByName(n, g.testUrl ?? null)).filter((v) => v > 0)
    return vals.length ? Math.min(...vals) : 0
  }
  return proxiesStore.getLatencyByName(g.name, g.testUrl ?? null)
}

const currentMember = (g: Proxy) => (g.type === 'LoadBalance' ? '' : g.now || '')
const mainIsLB = computed(() => {
  const now = main.value?.now
  return !!now && groupMap.value.get(now)?.type === 'LoadBalance'
})

async function pick(g: Proxy) {
  if (!main.value || main.value.now === g.name)
    return
  try {
    await proxiesStore.selectProxyInGroup(main.value, g.name)
  }
  catch (e) {
    toast.error(`切换失败：${e instanceof Error ? e.message : e}`)
  }
}

async function testAll() {
  testing.value = true
  try {
    await Promise.allSettled(tiles.value.filter((g) => g.name !== main.value?.name).map((g) => proxiesStore.proxyGroupLatencyTest(g.name)))
  }
  finally {
    testing.value = false
  }
}

onMounted(() => {
  if (!proxiesStore.proxies.length)
    proxiesStore.fetchProxies()
})
</script>

<template>
  <div class="prism-card flex flex-col gap-3 p-4">
    <div class="flex items-center justify-between gap-2">
      <span class="prism-title flex items-center gap-1.5">
        <IconRocket :size="18" class="text-info" />
        {{ main ? prismStripEmoji(main.name) : '节点选择' }}
      </span>
      <button type="button" class="prism-sub btn btn-sm border-0" :disabled="testing" @click="testAll">
        <span v-if="testing" class="loading loading-spinner loading-xs" />
        测速
      </button>
    </div>
    <div v-if="main" class="prism-muted flex flex-wrap items-center gap-x-3 text-xs sm:text-sm">
      <span>当前：{{ main.now }}</span>
      <span v-if="mainIsLB" class="flex items-center gap-1 text-warning/90">
        <IconAlertTriangle :size="14" /> 主出口是负载均衡，建议改用竞技组
      </span>
    </div>
    <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
      <button
        v-for="g in tiles"
        :key="g.name"
        type="button"
        class="prism-sub flex min-w-0 flex-col gap-1 p-2.5 text-left transition-colors hover:border-info/40"
        :class="main?.now === g.name ? '!border-info/70 !bg-info/15' : ''"
        :title="g.name"
        @click="pick(g)"
      >
        <span class="truncate text-sm font-semibold">{{ g.name }}</span>
        <span class="flex items-end justify-between gap-1 text-xs">
          <span class="prism-muted min-w-0 truncate">
            <template v-if="currentMember(g)">→ {{ currentMember(g) }}</template>
            <template v-else>{{ prismGroupKind(g.type) }}</template>
          </span>
          <span class="prism-num shrink-0" :class="prismLatencyClass(latencyOf(g))">
            {{ latencyOf(g) > 0 ? `${latencyOf(g)} ms` : '—' }}
          </span>
        </span>
      </button>
    </div>
    <div v-if="!tiles.length" class="prism-muted py-6 text-center text-sm">
      暂无策略组
    </div>
  </div>
</template>
