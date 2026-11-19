<script setup lang="ts">
import { IconScale } from '@tabler/icons-vue'

const proxiesStore = useProxiesStore()
const groupMap = computed(() => new Map(proxiesStore.proxies.map((p) => [p.name, p])))

// 当前在用的负载均衡组：沿主出口（GLOBAL 之外最上层 Selector）的选择链找第一个 LoadBalance
const lb = computed(() => {
  const start = proxiesStore.proxies.find((p) => p.type === 'Selector' && p.name !== 'GLOBAL')
  let cur = start
  const seen = new Set<string>()
  while (cur && !seen.has(cur.name)) {
    seen.add(cur.name)
    if (cur.type === 'LoadBalance')
      return cur
    cur = cur.now ? groupMap.value.get(cur.now) : undefined
  }
  return proxiesStore.proxies.find((p) => p.type === 'LoadBalance')
})

const topo = ref<{ healthy: number, inUse: number, total: { conns: number }, members: unknown[] } | null>(null)
const strategy = computed(() => {
  const n = lb.value?.name ?? ''
  if (n.includes('轮询'))
    return '轮询'
  if (n.includes('哈希'))
    return '一致性哈希'
  return '负载均衡'
})
</script>

<template>
  <div class="prism-card flex flex-col gap-3 p-4">
    <div class="flex items-start justify-between gap-2">
      <div class="min-w-0">
        <div class="prism-title flex items-center gap-1.5">
          <IconScale :size="18" class="prism-muted" /> 当前负载均衡
        </div>
        <div v-if="lb" class="prism-muted truncate text-xs sm:text-sm">
          {{ prismStripEmoji(lb.name) }} · {{ strategy }} · {{ topo?.healthy ?? 0 }}/{{ lb.all?.length ?? 0 }} 健康 · 在用 {{ topo?.inUse ?? 0 }} · ⇅ {{ topo?.total.conns ?? 0 }}
        </div>
      </div>
      <NuxtLink to="/topology" class="prism-sub btn btn-sm shrink-0 border-0">
        拓扑
      </NuxtLink>
    </div>
    <template v-if="lb">
      <PrismLBTopology ref="topo" :group="lb.name" :limit="8" />
      <NuxtLink v-if="(lb.all?.length ?? 0) > 8" to="/topology" class="text-center text-xs text-info">
        还有 {{ (lb.all?.length ?? 0) - 8 }} 个节点 · 打开拓扑页
      </NuxtLink>
    </template>
    <div v-else class="prism-muted py-6 text-center text-sm">
      当前配置没有负载均衡策略组
    </div>
  </div>
</template>
