<script setup lang="ts">
// PrismNet 设置：后端信息、在线更新、广告拦截管理、看门狗。
import type { PrismAdblockList, PrismAdblockState, PrismInfo, PrismUpdateCheck, PrismWatchdog } from '~/types/prism'
import dayjs from 'dayjs'
import { toast } from 'vue-sonner'

const { t } = useI18n()
useHead({ title: computed(() => t('prism.settings')) })

const api = usePrismApi()
const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e))

// ---- 后端信息 ----
const info = ref<PrismInfo | null>(null)
const infoErr = ref('')
const loadInfo = async () => {
  try {
    info.value = await api.info()
    infoErr.value = ''
  }
  catch (e) {
    infoErr.value = errMsg(e)
  }
}
const restarting = ref(false)
async function restartCore() {
  if (!confirm('确定重启 mihomo 核心？重启期间网络会短暂中断。'))
    return
  restarting.value = true
  try {
    await api.restartCore()
    toast.success('已重启核心')
  }
  catch (e) {
    toast.error(`重启失败：${errMsg(e)}`)
  }
  finally {
    restarting.value = false
    setTimeout(loadInfo, 3000)
  }
}

// ---- 在线更新 ----
const upd = ref<PrismUpdateCheck | null>(null)
const checking = ref(false)
const applying = ref(false)
async function checkUpdate() {
  checking.value = true
  try {
    upd.value = await api.checkUpdate()
    if (upd.value.error)
      toast.error(upd.value.error)
    else if (!upd.value.has_update)
      toast.success('已是最新版本')
  }
  catch (e) {
    toast.error(`检查失败：${errMsg(e)}`)
  }
  finally {
    checking.value = false
  }
}
async function applyUpdate() {
  if (!upd.value?.has_update || !confirm(`更新到 ${upd.value.latest}？后端会自动重启，面板随后刷新。`))
    return
  applying.value = true
  try {
    const r = await api.applyUpdate()
    toast.success(`已更新到 ${r.version}，正在重启…`)
    const deadline = Date.now() + 60000
    const wait = async () => {
      try {
        const i = await api.info()
        if (i.version === r.version)
          return location.reload()
      }
      catch {}
      if (Date.now() < deadline)
        setTimeout(wait, 2000)
      else location.reload()
    }
    setTimeout(wait, 2500)
  }
  catch (e) {
    toast.error(`更新失败：${errMsg(e)}`)
    applying.value = false
  }
}

// ---- 广告拦截 ----
const ab = ref<PrismAdblockState | null>(null)
const lists = ref<PrismAdblockList[]>([])
const white = ref('')
const black = ref('')
const newName = ref('')
const newUrl = ref('')
const abBusy = ref('')
const setAb = (s: PrismAdblockState) => {
  ab.value = s
  lists.value = s.lists.map((l) => ({ ...l }))
  white.value = s.white.join('\n')
  black.value = s.black.join('\n')
}
const loadAb = async () => {
  try {
    setAb(await api.adblock())
  }
  catch (e) {
    toast.error(`读取广告拦截设置失败：${errMsg(e)}`)
  }
}
const splitLines = (s: string) => s.split(/[\s,]+/).map((x) => x.trim()).filter(Boolean)
async function saveAb() {
  abBusy.value = 'save'
  try {
    setAb(await api.saveAdblock({ lists: lists.value, white: splitLines(white.value), black: splitLines(black.value) }))
    toast.success('已保存并重新合成规则')
  }
  catch (e) {
    toast.error(`保存失败：${errMsg(e)}`)
  }
  finally {
    abBusy.value = ''
  }
}
async function updateLists() {
  abBusy.value = 'update'
  try {
    await api.saveAdblock({ lists: lists.value, white: splitLines(white.value), black: splitLines(black.value) })
    setAb(await api.updateAdblock())
    toast.success('过滤列表已更新')
  }
  catch (e) {
    toast.error(`更新失败：${errMsg(e)}`)
  }
  finally {
    abBusy.value = ''
  }
}
async function toggleAb() {
  if (!ab.value)
    return
  abBusy.value = 'toggle'
  try {
    setAb(await api.toggleAdblock(!ab.value.enabled))
  }
  catch (e) {
    toast.error(`操作失败：${errMsg(e)}`)
  }
  finally {
    abBusy.value = ''
  }
}
function addList() {
  const url = newUrl.value.trim()
  if (!/^https?:\/\//.test(url))
    return toast.error('请输入 http(s) 开头的列表地址')
  lists.value.push({ id: '', name: newName.value.trim() || url, url, enabled: true, count: 0, updated: 0 })
  newName.value = ''
  newUrl.value = ''
}

// ---- 看门狗 ----
const wd = ref<PrismWatchdog | null>(null)
const loadWd = async () => {
  try {
    wd.value = await api.watchdog()
  }
  catch {}
}
async function toggleWd() {
  if (!wd.value)
    return
  try {
    wd.value = await api.setWatchdog(!wd.value.enabled)
  }
  catch (e) {
    toast.error(`操作失败：${errMsg(e)}`)
  }
}

const fmtTime = (t: number) => (t ? dayjs.unix(t).format('YYYY-MM-DD HH:mm') : '—')

onMounted(() => {
  loadInfo()
  loadAb()
  loadWd()
})
</script>

<template>
  <div class="flex h-full flex-col gap-3 overflow-y-auto p-2 pb-24 sm:gap-4 lg:pb-4">
    <!-- 后端 -->
    <section class="prism-card flex flex-col gap-3 p-4">
      <div class="flex items-center justify-between gap-2">
        <span class="prism-title">PrismNet 后端</span>
        <button type="button" class="btn btn-sm btn-warning" :disabled="restarting || !info" @click="restartCore">
          <span v-if="restarting" class="loading loading-spinner loading-xs" />
          重启核心
        </button>
      </div>
      <div v-if="infoErr" class="text-sm text-error">
        无法连接 PrismNet 后端（{{ infoErr }}）。直接连接原版 mihomo 时，扩展功能不可用。
      </div>
      <div v-if="info" class="grid grid-cols-[auto_1fr] gap-x-6 gap-y-1.5 text-sm">
        <span class="prism-muted">面板版本</span><span>{{ info.version }}</span>
        <span class="prism-muted">核心</span>
        <span :class="info.core.alive ? 'text-success' : 'text-error'">
          {{ info.core.alive ? `运行中 · mihomo ${info.core.version}` : `无响应 · ${info.core.error ?? ''}` }}
        </span>
        <span class="prism-muted">服务</span><span>{{ info.core.service }}</span>
        <span class="prism-muted">核心配置</span><span class="break-all">{{ info.core.config_path }}</span>
        <span class="prism-muted">项目</span>
        <a :href="`https://github.com/${info.repo}`" target="_blank" rel="noopener" class="link link-info">{{ info.repo }}</a>
      </div>
    </section>

    <!-- 在线更新 -->
    <section class="prism-card flex flex-col gap-3 p-4">
      <div class="flex items-center justify-between gap-2">
        <span class="prism-title">在线更新</span>
        <div class="flex gap-2">
          <button type="button" class="btn btn-sm" :disabled="checking" @click="checkUpdate">
            <span v-if="checking" class="loading loading-spinner loading-xs" />
            检查更新
          </button>
          <button v-if="upd?.has_update" type="button" class="btn btn-sm btn-info" :disabled="applying" @click="applyUpdate">
            <span v-if="applying" class="loading loading-spinner loading-xs" />
            更新到 {{ upd.latest }}
          </button>
        </div>
      </div>
      <p class="prism-muted text-sm">
        从 GitHub Releases 下载对应架构（{{ upd?.asset ?? 'prismnet-linux-<arch>.tar.gz' }}）的发布包，校验 SHA-256 后替换面板与后端程序并自动重启；旧版本保留为 .bak。
      </p>
      <div v-if="upd && !upd.error" class="text-sm">
        当前 <b>{{ upd.current }}</b> · 最新 <b>{{ upd.latest }}</b>
        <span v-if="upd.published" class="prism-muted">（{{ dayjs(upd.published).format('YYYY-MM-DD') }}）</span>
        <pre v-if="upd.notes && upd.has_update" class="prism-sub mt-2 max-h-48 overflow-auto p-3 text-xs whitespace-pre-wrap">{{ upd.notes }}</pre>
      </div>
    </section>

    <!-- 广告拦截 -->
    <section id="adblock" class="prism-card flex flex-col gap-3 p-4">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="prism-title">广告拦截（域名过滤）</span>
        <label class="flex items-center gap-2 text-sm">
          {{ ab?.enabled ? '已开启' : '已关闭' }}
          <input type="checkbox" class="toggle toggle-success" :checked="ab?.enabled" :disabled="!!abBusy || !ab" @change.prevent="toggleAb">
        </label>
      </div>
      <p class="prism-muted text-sm">
        把过滤列表合成为 mihomo 规则集（{{ prismNum(ab?.rules) }} 条，更新于 {{ fmtTime(ab?.updated ?? 0) }}）。开启时在核心配置最前面加入一条 REJECT 规则并热重载，关闭时移除；白名单优先于拦截。列表每天自动更新。
      </p>
      <div class="overflow-x-auto">
        <table class="table table-sm">
          <thead>
            <tr><th>启用</th><th>名称</th><th class="hidden sm:table-cell">地址</th><th>域名数</th><th>更新</th><th /></tr>
          </thead>
          <tbody>
            <tr v-for="(l, i) in lists" :key="l.id || l.url">
              <td><input v-model="l.enabled" type="checkbox" class="checkbox checkbox-sm"></td>
              <td class="max-w-40 truncate">
                {{ l.name }}
                <div v-if="l.error" class="text-xs text-error">
                  {{ l.error }}
                </div>
              </td>
              <td class="hidden max-w-72 truncate sm:table-cell">
                {{ l.url }}
              </td>
              <td class="prism-num">
                {{ prismNum(l.count) }}
              </td>
              <td class="whitespace-nowrap">
                {{ fmtTime(l.updated) }}
              </td>
              <td><button type="button" class="btn btn-ghost btn-xs" @click="lists.splice(i, 1)">删除</button></td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="flex flex-col gap-2 sm:flex-row">
        <input v-model="newName" class="input input-sm sm:w-40" placeholder="名称（可选）">
        <input v-model="newUrl" class="input input-sm flex-1" placeholder="列表地址：hosts / AdGuard / 纯域名 / Clash 规则集">
        <button type="button" class="btn btn-sm" @click="addList">
          添加列表
        </button>
      </div>
      <div class="grid gap-3 md:grid-cols-2">
        <label class="flex flex-col gap-1 text-sm">
          白名单（一行一个域名，含子域名）
          <textarea v-model="white" class="textarea h-28 font-mono text-xs" placeholder="s.click.taobao.com" />
        </label>
        <label class="flex flex-col gap-1 text-sm">
          黑名单（一行一个域名，含子域名）
          <textarea v-model="black" class="textarea h-28 font-mono text-xs" placeholder="ads.example.com" />
        </label>
      </div>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-sm" :disabled="!!abBusy" @click="updateLists">
          <span v-if="abBusy === 'update'" class="loading loading-spinner loading-xs" />
          立即更新列表
        </button>
        <button type="button" class="btn btn-sm btn-info" :disabled="!!abBusy" @click="saveAb">
          <span v-if="abBusy === 'save'" class="loading loading-spinner loading-xs" />
          保存
        </button>
      </div>
    </section>

    <!-- 看门狗 -->
    <section class="flex flex-col gap-2">
      <div class="prism-card flex items-center justify-between gap-2 p-4">
        <span class="prism-title">看门狗开关</span>
        <input type="checkbox" class="toggle toggle-success" :checked="wd?.enabled" :disabled="!wd" @change.prevent="toggleWd">
      </div>
      <PrismWatchdogCard full />
    </section>
  </div>
</template>
