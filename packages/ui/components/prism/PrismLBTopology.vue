<script setup lang="ts">
// 负载均衡拓扑：策略组（左侧圆）→ 成员节点（右侧胶囊，含连接数与下载速率），用 SVG 曲线连接。
const props = withDefaults(defineProps<{ group: string, limit?: number }>(), { limit: 8 })

const proxiesStore = useProxiesStore()
const connectionsStore = useConnectionsStore()

const g = computed(() => proxiesStore.proxies.find((p) => p.name === props.group))

// 统计经过该组的连接：chains 中组名之前的那一项就是实际成员
const usage = computed(() => {
  const m = new Map<string, { conns: number, down: number, up: number }>()
  for (const c of connectionsStore.activeConnections) {
    const i = c.chains.indexOf(props.group)
    if (i <= 0)
      continue
    const member = c.chains[i - 1]!
    const u = m.get(member) ?? { conns: 0, down: 0, up: 0 }
    u.conns++
    u.down += c.downloadSpeed || 0
    u.up += c.uploadSpeed || 0
    m.set(member, u)
  }
  return m
})

const members = computed(() => {
  const all = g.value?.all ?? []
  return all
    .map((name) => ({ name, alive: proxiesStore.getLatencyByName(name, g.value?.testUrl ?? null) > 0, ...(usage.value.get(name) ?? { conns: 0, down: 0, up: 0 }) }))
    .sort((a, b) => b.conns - a.conns || b.down - a.down || Number(b.alive) - Number(a.alive))
})
const shown = computed(() => members.value.slice(0, props.limit))
const total = computed(() => {
  let down = 0
  let up = 0
  let conns = 0
  for (const u of usage.value.values()) {
    down += u.down
    up += u.up
    conns += u.conns
  }
  return { down, up, conns }
})
const healthy = computed(() => members.value.filter((m) => m.alive).length)
const inUse = computed(() => members.value.filter((m) => m.conns > 0).length)

const ROW = 36
const svgH = computed(() => Math.max(ROW, shown.value.length * ROW))
const path = (i: number) => {
  const y = i * ROW + ROW / 2
  const cy = svgH.value / 2
  return `M0,${cy} C30,${cy} 20,${y} 50,${y}`
}
defineExpose({ healthy, inUse, total, members })
</script>

<template>
  <div v-if="g" class="flex items-stretch gap-0">
    <div class="flex shrink-0 items-center">
      <div class="flex size-28 flex-col items-center justify-center rounded-full border-2 border-info/70 bg-info/10 text-center text-xs sm:size-32">
        <b class="max-w-[6.5rem] truncate text-sm">{{ prismStripEmoji(g.name) }}</b>
        <span class="text-success">↓ {{ prismSpeed(total.down) }}</span>
        <span class="text-warning">↑ {{ prismSpeed(total.up) }}</span>
      </div>
    </div>
    <svg class="w-10 shrink-0 sm:w-12" :viewBox="`0 0 50 ${svgH}`" preserveAspectRatio="none" :style="{ height: `${svgH}px` }">
      <path v-for="(_, i) in shown" :key="i" :d="path(i)" fill="none" stroke="var(--color-info)" stroke-opacity="0.7" stroke-width="2" vector-effect="non-scaling-stroke" />
    </svg>
    <div class="flex min-w-0 flex-1 flex-col">
      <div v-for="m in shown" :key="m.name" class="flex items-center" :style="{ height: `${ROW}px` }">
        <div class="flex w-full min-w-0 items-center gap-2 rounded-lg border border-info/40 bg-base-100/40 px-2.5 py-1 text-xs sm:text-sm">
          <span class="size-2 shrink-0 rounded-full" :class="m.alive ? 'bg-success' : 'bg-error'" />
          <span class="min-w-0 flex-1 truncate font-semibold">{{ m.name }}</span>
          <span class="prism-muted shrink-0">⇅ {{ m.conns }}</span>
          <span class="shrink-0 text-success">↓ {{ prismSpeed(m.down) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
