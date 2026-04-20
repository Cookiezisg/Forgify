import { createContext, useContext, useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import { zhCN } from './locales/zh-CN'
import { en } from './locales/en'

export type Locale = 'zh-CN' | 'en'
const locales = { 'zh-CN': zhCN, en } as const
const STORAGE_KEY = 'forgify.locale'

type LeafPaths<T, P extends string = ''> = T extends string
  ? P
  : { [K in Extract<keyof T, string>]: LeafPaths<T[K], P extends '' ? K : `${P}.${K}`> }[Extract<keyof T, string>]

export type TranslationKey = LeafPaths<typeof zhCN>

function resolve(obj: Record<string, unknown>, key: string): string {
  const parts = key.split('.')
  let cur: unknown = obj
  for (const part of parts) {
    if (cur == null || typeof cur !== 'object') return key
    cur = (cur as Record<string, unknown>)[part]
  }
  return typeof cur === 'string' ? cur : key
}

interface I18nContextValue {
  locale: Locale
  setLocale: (locale: Locale) => void
  t: (key: TranslationKey) => string
}

const I18nContext = createContext<I18nContextValue | null>(null)

export function LocaleProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(() => {
    const saved = localStorage.getItem(STORAGE_KEY)
    return saved === 'en' || saved === 'zh-CN' ? saved : 'zh-CN'
  })
  const setLocale = useCallback((l: Locale) => {
    localStorage.setItem(STORAGE_KEY, l)
    setLocaleState(l)
  }, [])
  const t = useCallback(
    (key: TranslationKey): string => resolve(locales[locale] as Record<string, unknown>, key),
    [locale]
  )
  return <I18nContext.Provider value={{ locale, setLocale, t }}>{children}</I18nContext.Provider>
}

export function useI18n() {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useI18n must be used within LocaleProvider')
  return ctx
}

export function useT() { return useI18n().t }
