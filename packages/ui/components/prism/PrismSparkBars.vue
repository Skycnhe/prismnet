<script setup lang="ts">
// 小柱状图：values 中 ≤0 视为失败（画红色短柱）；不足 slots 个时左侧以虚点补齐。
const props = withDefaults(defineProps<{
  values: number[]
  slots?: number
  height?: number
}>(), { slots: 30, height: 26 })

const bars = computed(() => {
  const vals = props.values.slice(-props.slots)
  const pad = props.slots - vals.length
  const ok = vals.filter((v) => v > 0)
  const maxV = Math.max(1, ...ok)
  const minV = ok.length ? Math.min(...ok) : 0
  return [
    ...Array.from({ length: pad }, () => ({ h: 0, cls: 'empty' })),
    ...vals.map((v) => {
      if (v <= 0)
        return { h: 0.35, cls: 'fail' }
      // 以最小值为基线放大差异，保证最矮的柱子也可见
      const r = maxV === minV ? 0.6 : 0.3 + 0.7 * (v - minV) / (maxV - minV)
      return { h: r, cls: v <= 200 ? 'good' : v <= 400 ? 'mid' : 'bad' }
    }),
  ]
})
</script>

<template>
  <svg
    class="prism-bars"
    :viewBox="`0 0 ${slots * 4} ${height}`"
    preserveAspectRatio="none"
    :style="{ height: `${height}px` }"
    aria-hidden="true"
  >
    <template v-for="(b, i) in bars" :key="i">
      <circle
        v-if="b.cls === 'empty'"
        :cx="i * 4 + 1.5"
        :cy="height - 1.5"
        r="0.7"
        class="dot"
      />
      <rect
        v-else
        :x="i * 4 + 0.5"
        :y="height - b.h * height"
        width="2.2"
        :height="b.h * height"
        rx="0.6"
        :class="b.cls"
      />
    </template>
  </svg>
</template>

<style scoped>
.prism-bars {
  width: 100%;
  display: block;
}
.dot {
  fill: color-mix(in oklab, var(--color-base-content) 30%, transparent);
}
.good {
  fill: var(--color-success);
}
.mid {
  fill: var(--color-warning);
}
.bad,
.fail {
  fill: var(--color-error);
}
</style>
