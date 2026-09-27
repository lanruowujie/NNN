import english from "./locales/en.json";
import simplifiedChinese from "./locales/zh-CN.json";

export type Language = "en" | "zh-CN";
type TranslationValues = Record<string, string | number>;
type Dictionary = Record<string, string>;

const storageKey = "nfcx-language";
const dictionaries: Record<Language, Dictionary> = { en: english, "zh-CN": simplifiedChinese };
let currentLanguage: Language = detectLanguage();

function isSimplifiedChinese(locale: string): boolean {
  const normalized = locale.replace("_", "-").toLowerCase();
  return normalized === "zh" || normalized.startsWith("zh-cn") || normalized.startsWith("zh-sg") || normalized.startsWith("zh-hans");
}

export function detectLanguage(locales: readonly string[] = navigator.languages): Language {
  try {
    const saved = window.localStorage.getItem(storageKey);
    if (saved === "en" || saved === "zh-CN") return saved;
  } catch { /* Storage access must never block startup. */ }
  return locales.some(isSimplifiedChinese) ? "zh-CN" : "en";
}

export function language(): Language { return currentLanguage; }

export function t(key: string, values: TranslationValues = {}): string {
  const template = dictionaries[currentLanguage][key] ?? dictionaries.en[key] ?? key;
  return template.replace(/\{(\w+)\}/g, (match, name: string) => String(values[name] ?? match));
}

export function setLanguage(next: Language): void {
  currentLanguage = next;
  try { window.localStorage.setItem(storageKey, next); } catch { /* Current session still updates. */ }
}

export function applyTranslations(root: ParentNode = document): void {
  root.querySelectorAll<HTMLElement>("[data-i18n]").forEach((node) => { node.textContent = t(node.dataset.i18n || ""); });
  root.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>("[data-i18n-placeholder]").forEach((node) => { node.placeholder = t(node.dataset.i18nPlaceholder || ""); });
  root.querySelectorAll<HTMLElement>("[data-i18n-title]").forEach((node) => { node.title = t(node.dataset.i18nTitle || ""); });
  document.documentElement.lang = currentLanguage;
}
