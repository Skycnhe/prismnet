// PrismNet 扩展 API（Go 后端 /mihomo/prism/*）的数据结构。

export interface PrismSite {
  name: string
  url: string
  icon: string
  /** 毫秒；< 0 表示超时 / 失败 */
  ms: number
  hist: number[]
}

export interface PrismIPSide {
  ip: string
  country: string
  region: string
  city: string
  isp: string
  location: string
  error?: string
}

export interface PrismIPInfo {
  local: PrismIPSide
  proxy: PrismIPSide
  /** mihomo 顺序：节点在前，策略组在后 */
  chains: string[] | null
  rule: string
  at: number
}

export interface PrismHourPoint {
  h: string
  req: number
  blk: number
  dns: number | null
}

export interface PrismStatsReport {
  connected: boolean
  today: number
  req24: number
  blk24: number
  hours: PrismHourPoint[]
  top: { domain: string, count: number }[]
  days: { date: string, count: number }[] | null
  dns: { avg: number | null, samples: number, series: (number | null)[] | null }
  dns_result: { total: number, cache: number, upstream: number, other: number }
}

export interface PrismStats {
  stats: PrismStatsReport
  adblock: { enabled: boolean, rules: number, updated: number }
}

export interface PrismAdblockList {
  id: string
  name: string
  url: string
  enabled: boolean
  count: number
  updated: number
  error?: string
}

export interface PrismAdblockState {
  enabled: boolean
  lists: PrismAdblockList[]
  white: string[]
  black: string[]
  rules: number
  updated: number
  injected: boolean
  error?: string
}

export interface PrismWatchdogEvent {
  t: number
  kind: 'restart' | 'bypass' | 'recover' | 'error'
  msg: string
}

export interface PrismWatchdog {
  enabled: boolean
  state: 'ok' | 'failing' | 'bypass' | 'disabled'
  fails: number
  last_check: number
  interval: number
  threshold: number
  version: string
  events: PrismWatchdogEvent[] | null
  counts: { restart: number, bypass: number, recover: number }
}

export interface PrismUpdateCheck {
  current: string
  latest: string
  has_update: boolean
  notes: string
  url: string
  published: string
  asset: string
  error?: string
}

export interface PrismInfo {
  name: string
  version: string
  repo: string
  core: {
    alive: boolean
    version: string
    service: string
    config_path: string
    error?: string
  }
  features: string[]
}
