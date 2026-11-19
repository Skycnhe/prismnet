<script setup lang="ts">
import type { PrismIPInfo } from '~/types/prism'
import { IconRefresh, IconScale } from '@tabler/icons-vue'

const api = usePrismApi()
const proxiesStore = useProxiesStore()
const { data, error, loading, refresh } = usePrismPoll<PrismIPInfo>((f) => api.ipinfo(f), 300000)
const hidden = useLocalStorage('prism-hide-ip', false)

const show = (ip: string) => (hidden.value ? prismMaskIP(ip) : ip)

// 出口节点：节点链里最靠近节点的「策略组」；负载均衡组本身就是出口（按连接分到多个节点）
const exit = computed(() => {
  const chains = data.value?.chains ?? []
  if (!chains.length)
    return null
  for (let i = chains.length - 1; i >= 0; i--) {
    const name = chains[i]!
    const g = proxiesStore.proxies.find((p) => p.name === name)
    if (g?.type === 'LoadBalance')
      return { name, lb: true, desc: '负载均衡，按连接分到多个节点' }
  }
  return { name: chains[0]!, lb: false, desc: chains.length > 1 ? `经 ${chains.slice(1).reverse().join(' → ')}` : '' }
})
</script>

<template>
  <div class="prism-card flex flex-col gap-3 p-4">
    <div class="flex items-center justify-between">
      <span class="prism-title">IP 信息</span>
      <div class="flex gap-2">
        <button type="button" class="btn btn-ghost btn-sm btn-square" title="重新查询" :disabled="loading" @click="refresh(true)">
          <IconRefresh :size="16" :class="loading ? 'animate-spin' : ''" />
        </button>
        <button type="button" class="prism-sub btn btn-sm border-0" @click="hidden = !hidden">
          {{ hidden ? '显示 IP' : '隐藏 IP' }}
        </button>
      </div>
    </div>
    <div v-if="data" class="grid grid-cols-[auto_1fr] items-baseline gap-x-4 gap-y-2.5">
      <span class="whitespace-nowrap">本地公网</span>
      <span class="min-w-0 truncate">
        <b class="prism-num mr-2">{{ show(data.local.ip) || '—' }}</b>
        <span class="prism-muted text-sm">{{ data.local.location || data.local.error }}</span>
      </span>
      <span class="whitespace-nowrap">代理出口</span>
      <span class="min-w-0 truncate">
        <b class="prism-num mr-2">{{ show(data.proxy.ip) || '—' }}</b>
        <span class="prism-muted text-sm">{{ data.proxy.location || data.proxy.error }}</span>
      </span>
      <span class="whitespace-nowrap">出口节点</span>
      <span class="min-w-0 truncate">
        <template v-if="exit">
          <IconScale v-if="exit.lb" :size="15" class="prism-muted mr-1 inline align-[-2px]" />
          <b class="mr-2">{{ exit.lb ? prismStripEmoji(exit.name) : exit.name }}</b>
          <span class="prism-muted text-sm">{{ exit.desc }}</span>
        </template>
        <span v-else class="prism-muted text-sm">—</span>
      </span>
    </div>
    <div v-else-if="error" class="text-error text-sm">
      {{ error }}
    </div>
    <div v-else class="skeleton h-20 opacity-30" />
  </div>
</template>
