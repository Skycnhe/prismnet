// PrismNet 扩展 API 客户端：地址为「当前后端地址 + /prism」，与核心 API 共用同一个密钥。
// 演示模式（MOCK_MODE）下返回 prismMock 里的数据。
import type {
  PrismAdblockState,
  PrismInfo,
  PrismIPInfo,
  PrismSite,
  PrismStats,
  PrismUpdateCheck,
  PrismWatchdog,
} from '~/types/prism'
import ky from 'ky'
import { prismMockResponse } from './prismMock'
import { useMockMode } from './useApi'

export function usePrismApi() {
  const endpointStore = useEndpointStore()
  const mock = useMockMode()

  const client = () => {
    const ep = endpointStore.currentEndpoint
    const headers: Record<string, string> = {}
    if (ep?.secret)
      headers.Authorization = `Bearer ${ep.secret}`
    return ky.create({
      prefix: `${(ep?.url ?? '').replace(/\/+$/, '')}/prism`,
      headers,
      timeout: 20000,
      retry: 0,
    })
  }

  const get = async <T>(path: string, timeout?: number): Promise<T> => {
    if (mock)
      return prismMockResponse(path) as T
    return client().get(path, timeout ? { timeout } : undefined).json<T>()
  }
  const send = async <T>(method: 'post' | 'put', path: string, json?: unknown, timeout?: number): Promise<T> => {
    if (mock)
      return prismMockResponse(path) as T
    return client()[method](path, { json, timeout: timeout ?? 20000 }).json<T>()
  }

  return {
    info: () => get<PrismInfo>('info'),
    sites: (force = false) => get<PrismSite[]>(force ? 'sites?force=1' : 'sites', 30000),
    ipinfo: (force = false) => get<PrismIPInfo>(force ? 'ipinfo?force=1' : 'ipinfo', 30000),
    stats: () => get<PrismStats>('stats'),
    adblock: () => get<PrismAdblockState>('adblock'),
    saveAdblock: (body: Pick<PrismAdblockState, 'lists' | 'white' | 'black'>) =>
      send<PrismAdblockState>('put', 'adblock', body),
    updateAdblock: () => send<PrismAdblockState>('post', 'adblock/update', {}, 200000),
    toggleAdblock: (enabled: boolean) =>
      send<PrismAdblockState>('post', 'adblock/toggle', { enabled }, 200000),
    watchdog: () => get<PrismWatchdog>('watchdog'),
    setWatchdog: (enabled: boolean) => send<PrismWatchdog>('post', 'watchdog', { enabled }),
    restartCore: () => send<{ ok: boolean }>('post', 'core/restart', {}, 70000),
    checkUpdate: () => get<PrismUpdateCheck>('update/check', 40000),
    applyUpdate: () =>
      send<{ ok: boolean, version: string }>('post', 'update/apply', {}, 600000),
  }
}

/** 轮询一个扩展 API；页面隐藏时暂停。返回数据、错误与手动刷新函数。 */
export function usePrismPoll<T>(fetcher: (force: boolean) => Promise<T>, intervalMs: number) {
  const data = shallowRef<T | null>(null)
  const error = ref<string>('')
  const loading = ref(false)
  const visibility = useDocumentVisibility()

  const refresh = async (force = false) => {
    if (loading.value && !force)
      return
    loading.value = true
    try {
      data.value = await fetcher(force)
      error.value = ''
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
    finally {
      loading.value = false
    }
  }

  const { pause, resume } = useIntervalFn(() => refresh(false), intervalMs, { immediate: false })
  onMounted(() => {
    refresh(false)
    resume()
  })
  watch(visibility, (v) => {
    if (v === 'visible') {
      refresh(false)
      resume()
    }
    else {
      pause()
    }
  })
  onBeforeUnmount(pause)
  return { data, error, loading, refresh }
}
