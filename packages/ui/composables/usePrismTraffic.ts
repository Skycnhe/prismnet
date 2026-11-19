// 概览「实时流量（近 2 分钟）」：在全局流量推送上维护 120 个点的环形缓冲（跨页面保留）。
import { useMockMode } from './useApi'

const POINTS = 120
const down = ref<number[]>([])
const up = ref<number[]>([])
let started = false

function push(d: number, u: number) {
  down.value = [...down.value, d].slice(-POINTS)
  up.value = [...up.value, u].slice(-POINTS)
}

export function usePrismTraffic() {
  const globalStore = useGlobalStore()
  if (!started && import.meta.client) {
    started = true
    if (useMockMode()) {
      for (let i = 0; i < POINTS; i++) {
        const burst = i > 95 && i < 104 ? 46000 * Math.exp(-((i - 98) ** 2) / 4) : 0
        push(1200 + Math.round(Math.random() * 1600 + burst), 300 + Math.round(Math.random() * 500 + burst / 5))
      }
    }
    watch(() => globalStore.latestTraffic, (t) => {
      if (t && !useMockMode())
        push(t.down ?? 0, t.up ?? 0)
    })
  }
  const cur = computed(() => ({ down: down.value.at(-1) ?? 0, up: up.value.at(-1) ?? 0 }))
  return { down, up, cur, points: POINTS }
}
