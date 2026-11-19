<script setup lang="ts">
// 负载均衡拓扑页：列出全部负载均衡策略组及其成员节点（连接数、速率、健康）。
import { IconScale } from '@tabler/icons-vue'

const { t } = useI18n()
useHead({ title: computed(() => t('prism.topology')) })

const proxiesStore = useProxiesStore()
const lbs = computed(() => proxiesStore.proxies.filter((p) => p.type === 'LoadBalance'))
const active = computed(() => {
  const set = new Set<string>()
  for (const c of useConnectionsStore().activeConnections) c.chains.forEach((n) => set.add(n))
  return set
})
const sorted = computed(() => [...lbs.value].sort((a, b) => Number(active.value.has(b.name)) - Number(active.value.has(a.name))))
const showAll = ref<Record<string, boolean>>({})

onMounted(() => {
  if (!proxiesStore.proxies.length)
    proxiesStore.fetchProxies()
})
</script>

<template>
  <div class="flex h-full flex-col gap-3 overflow-y-auto p-2 pb-24 lg:pb-4">
    <div v-if="!sorted.length" class="prism-card prism-muted p-8 text-center">
      当前配置没有负载均衡策略组
    </div>
    <div v-for="g in sorted" :key="g.name" class="prism-card flex flex-col gap-3 p-4">
      <div class="flex items-center justify-between gap-2">
        <span class="prism-title flex items-center gap-1.5">
          <IconScale :size="18" class="prism-muted" /> {{ g.name }}
          <span v-if="active.has(g.name)" class="badge badge-info badge-sm">在用</span>
        </span>
        <span class="prism-muted text-sm">{{ g.all?.length ?? 0 }} 个节点</span>
      </div>
      <PrismLBTopology :group="g.name" :limit="showAll[g.name] ? 999 : 12" />
      <button
        v-if="(g.all?.length ?? 0) > 12"
        type="button"
        class="text-center text-xs text-info"
        @click="showAll[g.name] = !showAll[g.name]"
      >
        {{ showAll[g.name] ? '收起' : `展开全部 ${g.all?.length} 个节点` }}
      </button>
    </div>
  </div>
</template>
