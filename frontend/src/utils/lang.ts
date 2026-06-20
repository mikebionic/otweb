// Returns true if string contains CJK (Chinese/Japanese/Korean) characters
export function hasChinese(s: string): boolean {
  return /[\u4e00-\u9fff\u3400-\u4dbf]/.test(s)
}

// If title looks like untranslated Chinese, return placeholder
export function displayTitle(titleRu: string, _titleOriginal: string): string {
  if (!titleRu || hasChinese(titleRu)) return '⏳ Переводится...'
  return titleRu
}
