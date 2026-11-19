<script setup lang="ts">
// 迷你折线图（可带面积填充）；null 表示该点无数据（断开）。
const props = withDefaults(defineProps<{
  values: (number | null)[]
  color?: string
  height?: number
  fill?: boolean
  bars?: boolean
  min?: number
  max?: number
}>(), { color: 'var(--color-info)', height: 56, fill: false, bars: false })

const W = 200

const geo = computed(() => {
  const vals = props.values
  const nums = vals.filter((v): v is number => v != null)
  if (!nums.length)
    return { segs: [] as string[], areas: [] as string[], rects: [] as { x: number, y: number, h: number }[] }
  const max = props.max ?? Math.max(...nums)
  const min = props.min ?? Math.min(...nums)
  const span = Math.max(1, max - min)
  const h = props.height - 4
  const step = vals.length > 1 ? W / (vals.length - 1) : W
  const y = (v: number) => 2 + h - ((v - min) / span) * h
  if (props.bars) {
    const bw = W / Math.max(1, vals.length)
    return {
      segs: [],
      areas: [],
      rects: vals.flatMap((v, i) => v == null ? [] : [{ x: i * bw, y: y(v), h: props.height - y(v) }]),
    }
  }
  const segs: string[] = []
  const areas: string[] = []
  let cur: [number, number][] = []
  const flush = () => {
    if (cur.length) {
      segs.push(cur.map(([x, yy], i) => `${i ? 'L' : 'M'}${x.toFixed(1)},${yy.toFixed(1)}`).join(''))
      areas.push(`M${cur[0]![0].toFixed(1)},${props.height}${cur.map(([x, yy]) => `L${x.toFixed(1)},${yy.toFixed(1)}`).join('')}L${cur.at(-1)![0].toFixed(1)},${props.height}Z`)
    }
    cur = []
  }
  vals.forEach((v, i) => {
    if (v == null)
      flush()
    else cur.push([i * step, y(v)])
  })
  flush()
  return { segs, areas, rects: [] }
})
</script>

<template>
  <svg
    :viewBox="`0 0 ${W} ${height}`"
    preserveAspectRatio="none"
    class="block w-full"
    :style="{ height: `${height}px` }"
    aria-hidden="true"
  >
    <template v-if="bars">
      <rect
        v-for="(r, i) in geo.rects"
        :key="i"
        :x="r.x"
        :y="r.y"
        :width="Math.max(0.6, (W / Math.max(1, values.length)) * 0.7)"
        :height="r.h"
        :fill="color"
      />
    </template>
    <template v-else>
      <path v-for="(a, i) in fill ? geo.areas : []" :key="`a${i}`" :d="a" :fill="color" opacity="0.18" />
      <path
        v-for="(s, i) in geo.segs"
        :key="`s${i}`"
        :d="s"
        fill="none"
        :stroke="color"
        stroke-width="2"
        stroke-linejoin="round"
        stroke-linecap="round"
        vector-effect="non-scaling-stroke"
      />
    </template>
  </svg>
</template>
