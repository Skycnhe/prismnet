// PrismNet：默认中文；用户在语言切换里选过的语言保存在 localStorage 并在启动时恢复。
import { PRISM_LOCALE_KEY } from '~/utils/prism'

export default defineNuxtPlugin((nuxtApp) => {
  let saved: string | null = null
  try {
    saved = localStorage.getItem(PRISM_LOCALE_KEY)
  }
  catch {}
  const i18n = nuxtApp.$i18n as { locale: { value: string }, setLocale: (c: string) => Promise<void> }
  if (saved && saved !== i18n.locale.value)
    i18n.setLocale(saved)
})
