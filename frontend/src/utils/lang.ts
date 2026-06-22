// Returns true if string contains CJK (Chinese/Japanese/Korean) characters
export function hasChinese(s: string): boolean {
  return /[\u4e00-\u9fff\u3400-\u4dbf]/.test(s)
}

// Показываем переведённое название, если оно есть; иначе — оригинал (без фейкового
// «Переводится…», который вводил в заблуждение: перевод не идёт постоянно в фоне).
export function displayTitle(titleRu: string, titleOriginal: string): string {
  if (titleRu && !hasChinese(titleRu)) return titleRu
  return titleOriginal || titleRu || ''
}
