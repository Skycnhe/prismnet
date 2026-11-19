// PrismNet 前端工具函数。

/** 延迟着色：≤200 绿、≤400 黄、其余红；≤0 视为超时 */
export function prismLatencyClass(ms: number | null | undefined): string {
  if (!ms || ms <= 0)
    return 'prism-muted'
  if (ms <= 200)
    return 'prism-lat-good'
  if (ms <= 400)
    return 'prism-lat-mid'
  return 'prism-lat-bad'
}

/** 速率格式化：B/s、KB/s、MB/s */
export function prismSpeed(bps: number): string {
  const v = Math.max(0, bps || 0)
  if (v < 1024)
    return `${Math.round(v)} B/s`
  if (v < 1024 * 1024)
    return `${(v / 1024).toFixed(1)} KB/s`
  if (v < 1024 * 1024 * 1024)
    return `${(v / 1024 / 1024).toFixed(2)} MB/s`
  return `${(v / 1024 / 1024 / 1024).toFixed(2)} GB/s`
}

/** 千分位 */
export function prismNum(n: number | null | undefined): string {
  return (n ?? 0).toLocaleString('en-US')
}

/** 遮挡 IP：保留首段，其余替换为 */
export function prismMaskIP(ip: string): string {
  if (!ip)
    return ''
  if (ip.includes(':'))
    return `${ip.split(':')[0]}:****:****`
  const p = ip.split('.')
  return p.length === 4 ? `${p[0]}.*.*.${'*'.repeat(p[3]!.length)}` : '***'
}

/** 策略组类型 → 中文 */
export function prismGroupKind(type: string): string {
  switch (type) {
    case 'URLTest': return '自动测速'
    case 'LoadBalance': return '负载均衡'
    case 'Fallback': return '故障转移'
    case 'Selector': return '手动选择'
    case 'Smart': return '智能选择'
    case 'Relay': return '链式代理'
    default: return type
  }
}

/** 去掉名称里开头的 emoji / 符号，便于紧凑显示 */
export function prismStripEmoji(name: string): string {
  return name.replace(/^[\p{Extended_Pictographic}\p{Regional_Indicator}\uFE0F\u200D\s]+/u, '')
}

/** 名称开头的 emoji（国旗、⚖️ 等） */
export function prismLeadEmoji(name: string): string {
  const m = name.match(/^[\p{Extended_Pictographic}\p{Regional_Indicator}\uFE0F\u200D]+/u)
  return m ? m[0] : ''
}

export const PRISM_LOCALE_KEY = 'prismnet_lang'

/** 记住用户选择的界面语言（启动时由 plugins/prism-locale 恢复） */
export function rememberPrismLocale(code: string) {
  try {
    localStorage.setItem(PRISM_LOCALE_KEY, code)
  }
  catch {}
}
