// Tiny UA → "Browser on OS" parser. We don't need precision; just enough
// for the user to recognize "this is my phone vs my work laptop". For
// anything pathological, we fall back to the raw UA string.

export function parseUserAgent(ua: string): string {
  if (!ua) return 'Unknown device'

  let browser = ''
  if (/Edg\//.test(ua)) browser = 'Edge'
  else if (/OPR\//.test(ua) || /Opera/.test(ua)) browser = 'Opera'
  else if (/YaBrowser/.test(ua)) browser = 'Yandex'
  else if (/Firefox\//.test(ua)) browser = 'Firefox'
  else if (/Chrome\//.test(ua)) browser = 'Chrome'
  else if (/Safari\//.test(ua) && /Version\//.test(ua)) browser = 'Safari'

  let os = ''
  if (/Windows NT/.test(ua)) os = 'Windows'
  else if (/Android/.test(ua)) os = 'Android'
  else if (/iPhone|iPad|iPod/.test(ua)) os = 'iOS'
  else if (/Macintosh|Mac OS X/.test(ua)) os = 'macOS'
  else if (/Linux/.test(ua)) os = 'Linux'

  if (browser && os) return `${browser} on ${os}`
  if (browser) return browser
  if (os) return os
  return ua.length > 60 ? ua.slice(0, 57) + '...' : ua
}
