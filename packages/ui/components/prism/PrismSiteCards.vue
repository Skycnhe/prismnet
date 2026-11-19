<script setup lang="ts">
import type { PrismSite } from '~/types/prism'
import {
  IconBrandBaidu,
  IconBrandGithub,
  IconBrandGoogle,
  IconBrandTaobao,
  IconBrandTelegram,
  IconBrandYoutube,
  IconWorld,
} from '@tabler/icons-vue'

const api = usePrismApi()
const { data, refresh, loading } = usePrismPoll<PrismSite[]>((f) => api.sites(f), 60000)

const ICONS: Record<string, { icon: unknown, bg: string, fg: string }> = {
  google: { icon: IconBrandGoogle, bg: '#ffffff', fg: '#4285F4' },
  youtube: { icon: IconBrandYoutube, bg: '#FF0000', fg: '#ffffff' },
  telegram: { icon: IconBrandTelegram, bg: '#2AABEE', fg: '#ffffff' },
  github: { icon: IconBrandGithub, bg: '#24292f', fg: '#ffffff' },
  baidu: { icon: IconBrandBaidu, bg: '#ffffff', fg: '#2932E1' },
  taobao: { icon: IconBrandTaobao, bg: '#FF5000', fg: '#ffffff' },
}
const iconOf = (s: PrismSite) => ICONS[s.icon] ?? { icon: IconWorld, bg: 'var(--color-base-300)', fg: 'var(--color-base-content)' }

defineExpose({ refresh })
</script>

<template>
  <div class="grid grid-cols-2 gap-3 md:grid-cols-3">
    <template v-if="data">
      <button
        v-for="s in data"
        :key="s.name"
        type="button"
        class="prism-card flex items-center gap-3 px-3 py-3 text-left transition-transform active:scale-[0.98] sm:px-4"
        :title="`${s.url}\n点击重新测速`"
        :disabled="loading"
        @click="refresh(true)"
      >
        <span
          class="flex size-9 shrink-0 items-center justify-center rounded-xl shadow sm:size-11"
          :style="{ background: iconOf(s).bg, color: iconOf(s).fg }"
        >
          <component :is="iconOf(s).icon" :size="22" stroke-width="2" />
        </span>
        <div class="hidden min-w-0 flex-1 self-end sm:block">
          <PrismSparkBars :values="s.hist" />
        </div>
        <div class="ml-auto flex shrink-0 flex-col items-end">
          <span class="prism-muted text-xs sm:text-sm">{{ s.name }}</span>
          <span class="prism-num text-xl leading-tight sm:text-2xl" :class="prismLatencyClass(s.ms)">
            <template v-if="s.ms > 0">{{ s.ms }}<small class="prism-muted ml-0.5 text-xs font-normal">ms</small></template>
            <template v-else>超时</template>
          </span>
        </div>
      </button>
    </template>
    <template v-else>
      <div v-for="i in 6" :key="i" class="prism-card skeleton h-[68px] opacity-40" />
    </template>
  </div>
</template>
