<script setup lang="ts">
import { computed } from 'vue'

const endpointStore = useEndpointStore()
const hasEndpoint = computed(() => !!endpointStore.currentEndpoint)
</script>

<template>
  <div
    class="flex min-w-0 items-center gap-1 text-sm font-bold whitespace-nowrap sm:text-xl"
  >
    <NuxtLink
      class="bg-linear-to-br from-info via-primary to-warning bg-clip-text tracking-wide text-transparent no-underline"
      :to="hasEndpoint ? '/' : '/setup'"
    >
      <span>PrismNet</span>
    </NuxtLink>
    <a
      class="hidden text-[0.6em] font-normal text-base-content/50 no-underline hover:text-base-content min-[400px]:inline"
      href="https://github.com/Skycnhe/prismnet"
      target="_blank"
      title="PrismNet · 基于 metacubexd"
    >
      mihomo
    </a>
  </div>
</template>
