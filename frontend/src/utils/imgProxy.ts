/**
 * Оборачивает alicdn.com / taobao / 1688 URL через серверный прокси
 * чтобы обойти Referer ACL блокировку.
 */
export function imgProxy(url: string | undefined | null): string {
  if (!url) return ''
  // Уже наш URL (текущий хост магазина) или data: — без изменений
  const ownHost = typeof window !== 'undefined' ? window.location.hostname : ''
  if (url.startsWith('/') || url.startsWith('data:') || (ownHost && url.includes(ownHost))) return url
  return `/otweb/img-proxy?url=${encodeURIComponent(url)}`
}
