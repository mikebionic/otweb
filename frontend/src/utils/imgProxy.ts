/**
 * Оборачивает alicdn.com / taobao / 1688 URL через серверный прокси
 * чтобы обойти Referer ACL блокировку.
 */
export function imgProxy(url: string | undefined | null): string {
  if (!url) return ''
  // Уже наш URL или data: — без изменений
  if (url.startsWith('/') || url.startsWith('data:') || url.includes('wabrum.com')) return url
  return `/otweb/img-proxy?url=${encodeURIComponent(url)}`
}
