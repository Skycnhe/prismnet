<script setup lang="ts">
const { down, up, cur } = usePrismTraffic()

const peak = computed(() => Math.max(1, ...down.value, ...up.value))
const pad = (arr: number[]) => [...Array.from<number | null>({ length: Math.max(0, 120 - arr.length) }).fill(null), ...arr]
</script>

<template>
  <div class="prism-card flex flex-col gap-2 p-4">
    <div class="flex items-center justify-between">
      <span class="prism-title">实时流量（近 2 分钟）</span>
      <span class="flex gap-3 text-xs">
        <span class="text-info">● 下载</span>
        <span class="text-success">● 上传</span>
      </span>
    </div>
    <div class="flex items-baseline gap-6 text-2xl">
      <span class="prism-num text-info"><small class="prism-muted text-base">↓</small> {{ prismSpeed(cur.down) }}</span>
      <span class="prism-num text-success"><small class="prism-muted text-base">↑</small> {{ prismSpeed(cur.up) }}</span>
    </div>
    <div class="prism-muted text-xs">
      {{ prismSpeed(peak) }}
    </div>
    <div class="relative mt-auto h-[96px]">
      <div class="absolute inset-0">
        <PrismSparkLine :values="pad(down)" color="var(--color-info)" :height="96" :min="0" :max="peak" fill />
      </div>
      <div class="absolute inset-0">
        <PrismSparkLine :values="pad(up)" color="var(--color-success)" :height="96" :min="0" :max="peak" />
      </div>
    </div>
  </div>
</template>
