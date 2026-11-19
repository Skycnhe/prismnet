// PrismNet 演示数据（MOCK_MODE=true 时使用，用于截图与离线预览）。
import type {
  PrismAdblockState,
  PrismInfo,
  PrismIPInfo,
  PrismSite,
  PrismStats,
  PrismUpdateCheck,
  PrismWatchdog,
} from '~/types/prism'

interface MockNode {
  name: string
  type: string
  alive: boolean
  udp: boolean
  xudp: boolean
  tfo: boolean
  id: string
  extra: Record<string, unknown>
  history: { time: string, delay: number }[]
  all?: string[]
  now?: string
  hidden?: boolean
  icon?: string
}

const now = () => new Date().toISOString()

function node(name: string, delay: number, type = 'Vless'): MockNode {
  return {
    name,
    type,
    alive: delay > 0,
    udp: true,
    xudp: true,
    tfo: false,
    id: name,
    extra: {},
    history: delay > 0 ? [{ time: now(), delay }] : [],
  }
}

function group(name: string, type: string, all: string[], cur: string): MockNode {
  return { ...node(name, 0, type), all, now: cur, alive: true, history: [] }
}

const REGIONS: { key: string, flag: string, label: string, code: string, count: number, base: number }[] = [
  { key: 'jp', flag: '🇯🇵', label: '日本', code: 'JP', count: 36, base: 68 },
  { key: 'sg', flag: '🇸🇬', label: '新加坡', code: 'SG', count: 12, base: 96 },
  { key: 'hk', flag: '🇭🇰', label: '香港', code: 'HK', count: 18, base: 52 },
  { key: 'us', flag: '🇺🇸', label: '美国', code: 'US', count: 14, base: 188 },
  { key: 'kr', flag: '🇰🇷', label: '韩国', code: 'KR', count: 6, base: 121 },
  { key: 'other', flag: '🌐', label: '其他', code: 'XX', count: 5, base: 260 },
]

function seeded(i: number) {
  const x = Math.sin(i * 9301 + 49297) * 233280
  return x - Math.floor(x)
}

export function buildPrismMockProxies() {
  const proxies: Record<string, MockNode> = {
    DIRECT: node('DIRECT', 32, 'Direct'),
    REJECT: node('REJECT', 0, 'Reject'),
  }
  const regionGroups: string[] = []
  const allNodes: string[] = []
  let k = 0
  for (const r of REGIONS) {
    const names: string[] = []
    for (let i = 1; i <= r.count; i++) {
      const n = r.key === 'other'
        ? `${['三网通', '土耳其', '英国', '德国', '澳洲'][i - 1]} ${r.code}${i}`
        : `${r.key}【${r.label}】${r.code} (${i})`
      const d = Math.round(r.base + seeded(++k) * r.base * 0.9)
      proxies[n] = node(n, seeded(k + 7) > 0.06 ? d : 0)
      names.push(n)
    }
    allNodes.push(...names)
    const best = names.reduce((a, b) =>
      (proxies[b]!.history[0]?.delay ?? 9e9) < (proxies[a]!.history[0]?.delay ?? 9e9) ? b : a)
    const race = `${r.flag} ${r.label}竞技`
    const hash = `⚖️ ${r.label}哈希`
    const rr = `⚖️ ${r.label}轮询`
    proxies[race] = group(race, 'URLTest', names, best)
    proxies[hash] = group(hash, 'LoadBalance', names, '')
    proxies[rr] = group(rr, 'LoadBalance', names, '')
    regionGroups.push(race, hash, rr)
  }
  const races = regionGroups.filter((g) => g.includes('竞技'))
  const hashes = regionGroups.filter((g) => g.includes('哈希'))
  const rrs = regionGroups.filter((g) => g.includes('轮询'))
  proxies['🖐️ 手动选择'] = group('🖐️ 手动选择', 'Selector', allNodes, 'jp【日本】JP (5)')
  proxies['⚡ 全局自动选择'] = group('⚡ 全局自动选择', 'URLTest', allNodes, '三网通 XX1')
  proxies['🏠 直连'] = group('🏠 直连', 'Selector', ['DIRECT'], 'DIRECT')
  proxies['🚀 节点选择'] = group('🚀 节点选择', 'Selector', [
    ...races, ...hashes, ...rrs, '🖐️ 手动选择', '⚡ 全局自动选择', '🏠 直连',
  ], '⚖️ 日本轮询')
  proxies.GLOBAL = group('GLOBAL', 'Selector', ['🚀 节点选择', ...races, ...hashes, ...rrs, '🖐️ 手动选择', '⚡ 全局自动选择', '🏠 直连', 'DIRECT'], '🚀 节点选择')
  return proxies
}

export const prismMockProxies = buildPrismMockProxies()

export const prismMockConfig = {
  'port': 0,
  'socks-port': 0,
  'redir-port': 0,
  'tproxy-port': 7894,
  'mixed-port': 7890,
  'mode': 'rule',
  'log-level': 'info',
  'allow-lan': true,
  'unified-delay': true,
  'ipv6': false,
  'tun': {
    'enable': true,
    'device': 'Meta',
    'stack': 'mixed',
    'dns-hijack': null,
    'auto-route': true,
    'auto-detect-interface': true,
    'file-descriptor': 0,
  },
}

export const prismMockVersion = { meta: true, version: 'v1.19.17' }

const HOSTS = [
  'www.google.com', 'i.ytimg.com', 'rr3---sn-oguelnzl.googlevideo.com', 'api.github.com',
  'web.telegram.org', 'x.com', 'abs.twimg.com', 'chatgpt.com', 'www.netflix.com',
  'open.spotify.com', 'discord.com', 'gateway.icloud.com', 'www.baidu.com', 'g.alicdn.com',
]

export function buildPrismMockConnections() {
  const jp = Object.keys(prismMockProxies).filter((n) => n.startsWith('jp【'))
  const out = []
  for (let i = 0; i < 40; i++) {
    const host = HOSTS[i % HOSTS.length]!
    const direct = host.endsWith('baidu.com') || host.endsWith('alicdn.com')
    const nodeName = jp[[4, 5, 7, 25, 26, 1, 2, 6][i % 8]!]!
    const dev = i % 9 === 0 ? '192.168.1.23' : '192.168.1.105'
    out.push({
      id: `prism-conn-${i}`,
      metadata: {
        network: i % 7 === 0 ? 'udp' : 'tcp',
        type: 'Tun',
        destinationIP: `203.0.113.${i + 10}`,
        destinationPort: '443',
        dnsMode: 'fake-ip',
        host,
        inboundIP: '',
        inboundName: 'DEFAULT-TUN',
        inboundPort: '',
        inboundUser: '',
        process: '',
        processPath: '',
        remoteDestination: '',
        sniffHost: host,
        sourceIP: dev,
        sourcePort: String(50000 + i),
        specialProxy: '',
        specialRules: '',
        uid: 0,
      },
      upload: Math.round(seeded(i + 100) * 2e6),
      download: Math.round(seeded(i + 200) * 4e7),
      downloadSpeed: Math.round(seeded(i + 300) * 3e5),
      uploadSpeed: Math.round(seeded(i + 400) * 4e4),
      start: new Date(Date.now() - Math.round(seeded(i + 500) * 6e5)).toISOString(),
      chains: direct ? ['DIRECT'] : [nodeName, '⚖️ 日本轮询', '🚀 节点选择'],
      rule: direct ? 'GeoSite' : 'Match',
      rulePayload: direct ? 'cn' : '',
    })
  }
  return out
}

export const prismMockConnections = buildPrismMockConnections()

function hist(base: number, n = 30, spread = 0.25) {
  return Array.from({ length: n }, (_, i) =>
    Math.round(base * (1 - spread / 2 + seeded(base + i) * spread)))
}

export function prismMockSites(): PrismSite[] {
  return [
    { name: 'Google', url: 'https://www.google.com/generate_204', icon: 'google', ms: 108, hist: hist(108) },
    { name: 'YouTube', url: 'https://www.youtube.com/generate_204', icon: 'youtube', ms: 90, hist: hist(90) },
    { name: 'Telegram', url: 'https://web.telegram.org/favicon.ico', icon: 'telegram', ms: 359, hist: hist(300, 30, 0.6) },
    { name: 'GitHub', url: 'https://github.com/favicon.ico', icon: 'github', ms: 111, hist: hist(111) },
    { name: '百度', url: 'https://www.baidu.com/favicon.ico', icon: 'baidu', ms: 34, hist: hist(34) },
    { name: '淘宝', url: 'https://www.taobao.com/favicon.ico', icon: 'taobao', ms: 11, hist: hist(11) },
  ]
}

export function prismMockIP(): PrismIPInfo {
  return {
    local: { ip: '125.83.135.132', country: '中国', region: '重庆', city: '重庆', isp: '电信', location: '中国 重庆 电信' },
    proxy: { ip: '142.91.109.208', country: '日本', region: '东京都', city: 'Minato', isp: '', location: '日本 东京都 Minato' },
    chains: ['jp【日本】JP (5)', '⚖️ 日本轮询', '🚀 节点选择'],
    rule: 'Match',
    at: Math.floor(Date.now() / 1000),
  }
}

export function prismMockStats(): PrismStats {
  const hours = Array.from({ length: 24 }, (_, i) => {
    const req = Math.round(2500 + seeded(i + 40) * 5200 + (i > 8 && i < 23 ? 1500 : 0))
    return {
      h: String((new Date().getHours() + i + 1) % 24).padStart(2, '0'),
      req,
      blk: Math.round(req * (0.1 + seeded(i + 80) * 0.12)),
      dns: Math.round(80 + seeded(i + 120) * 60),
    }
  })
  const req24 = hours.reduce((a, b) => a + b.req, 0)
  const blk24 = hours.reduce((a, b) => a + b.blk, 0)
  return {
    stats: {
      connected: true,
      today: 12283,
      req24,
      blk24,
      hours,
      top: [
        { domain: 'dig.bdurl.net', count: 2329 },
        { domain: 'v2.gdt.qq.com', count: 822 },
        { domain: 'collector.github.com', count: 744 },
        { domain: 'abtest-ch.snssdk.com', count: 653 },
        { domain: 'ads5-normal-lq.zijieapi.com', count: 513 },
        { domain: 'mon.zijieapi.com', count: 402 },
      ],
      days: [],
      dns: {
        avg: 107,
        samples: 1436,
        series: Array.from({ length: 288 }, (_, i) => Math.round(70 + seeded(i + 900) * 90 + (seeded(i + 1900) > 0.93 ? 140 : 0))),
      },
      dns_result: { total: 100, cache: 63, upstream: 31, other: 6 },
    },
    adblock: { enabled: true, rules: 57346, updated: Math.floor(Date.now() / 1000) - 3600 },
  }
}

export function prismMockWatchdog(): PrismWatchdog {
  const t = Math.floor(Date.now() / 1000)
  const events: PrismWatchdog['events'] = []
  for (let i = 0; i < 20; i++) {
    const at = t - (20 - i) * 5400
    events.push(i % 2 === 0
      ? { t: at, kind: 'restart', msg: '核心无响应（超时），自动重启' }
      : { t: at + 40, kind: 'recover', msg: 'mihomo 核心已恢复' })
  }
  return {
    enabled: true,
    state: 'ok',
    fails: 0,
    last_check: t - 12,
    interval: 30,
    threshold: 2,
    version: 'v1.19.17',
    events,
    counts: { restart: 10, bypass: 0, recover: 10 },
  }
}

export function prismMockAdblock(): PrismAdblockState {
  const t = Math.floor(Date.now() / 1000)
  return {
    enabled: true,
    lists: [
      { id: 'anti-ad', name: 'anti-AD', url: 'https://anti-ad.net/domains.txt', enabled: true, count: 54210, updated: t - 3600 },
      { id: 'adguard-dns', name: 'AdGuard DNS filter', url: 'https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt', enabled: false, count: 0, updated: 0 },
    ],
    white: ['s.click.taobao.com'],
    black: ['ads.example.com'],
    rules: 57346,
    updated: t - 3600,
    injected: true,
  }
}

export function prismMockInfo(): PrismInfo {
  return {
    name: 'PrismNet',
    version: 'v0.1.0',
    repo: 'Skycnhe/prismnet',
    core: { alive: true, version: 'v1.19.17', service: 'mihomo', config_path: '/etc/mihomo/config.yaml' },
    features: ['sites', 'ipinfo', 'stats', 'adblock', 'watchdog', 'update', 'core-restart'],
  }
}

export function prismMockUpdate(): PrismUpdateCheck {
  return {
    current: 'v0.1.0',
    latest: 'v0.1.0',
    has_update: false,
    notes: '',
    url: 'https://github.com/Skycnhe/prismnet/releases',
    published: '',
    asset: 'prismnet-linux-arm64.tar.gz',
  }
}

/** 按路径返回演示数据（path 不含 /prism 前缀，例如 "sites"） */
export function prismMockResponse(path: string): unknown {
  const p = path.replace(/^\/+/, '').split('?')[0]
  switch (p) {
    case 'sites': return prismMockSites()
    case 'ipinfo': return prismMockIP()
    case 'stats': return prismMockStats()
    case 'watchdog': return prismMockWatchdog()
    case 'adblock':
    case 'adblock/update':
    case 'adblock/toggle': return prismMockAdblock()
    case 'info': return prismMockInfo()
    case 'update/check': return prismMockUpdate()
    default: return { ok: true }
  }
}
