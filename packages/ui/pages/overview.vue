<script setup lang="ts">
// PrismNet 概览：站点延迟、模式 / 方式 / 连接 / 广告、IP 信息、实时流量、域名过滤、
// 节点选择、负载均衡拓扑、看门狗与 DNS 查询结果。原 metacubexd 概览图表移到「统计」页。
import type { PrismStats } from '~/types/prism'

const { t } = useI18n()
useHead({ title: computed(() => t('overview')) })

const api = usePrismApi()
const { data: stats, refresh: refreshStats } = usePrismPoll<PrismStats>(() => api.stats(), 30000)
</script>

<template>
  <div class="flex h-full flex-col gap-3 overflow-x-hidden overflow-y-auto p-2 pb-24 sm:gap-4 lg:pb-4">
    <OnboardingEmptyState context="overview" />

    <PrismSiteCards />
    <PrismStatusRow :stats="stats" />

    <div class="grid gap-3 sm:gap-4 lg:grid-cols-2">
      <PrismIPCard />
      <PrismTrafficCard />
    </div>

    <div class="grid items-start gap-3 sm:gap-4 lg:grid-cols-2">
      <div class="flex min-w-0 flex-col gap-3 sm:gap-4">
        <PrismDomainFilterCard :stats="stats" @changed="refreshStats(true)" />
        <PrismLBCard />
      </div>
      <PrismNodeSelectCard class="min-w-0" />
    </div>

    <div class="grid items-start gap-3 sm:gap-4 lg:grid-cols-2">
      <PrismWatchdogCard />
      <PrismDNSCard :stats="stats" />
    </div>
  </div>
</template>
