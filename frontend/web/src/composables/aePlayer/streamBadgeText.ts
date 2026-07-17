import type { StreamBadge } from './capLabels'

type T = (key: string) => string

/**
 * Render one consolidated stream badge (owner taxonomy 2026-07-17). CSS
 * uppercases it, so plain lowercase i18n parts come out as e.g.
 * "SUB BURNED-IN RU", "SUB · SELECTABLE", "DUB RU", "DUB EN/RU".
 * Shared by ProviderChip badges and the Source panel's Stream entries so the
 * two surfaces can never drift apart.
 */
export function streamBadgeText(b: StreamBadge, t: T): string {
  const langs = b.langs.map((l) => l.toUpperCase()).join('/')
  if (b.kind === 'dub') {
    return langs ? `${t('player.dub')} ${langs}` : t('player.dub')
  }
  if (b.burnedIn && langs) return `${t('player.sub')} ${t('player.sources.subBurnedIn')} ${langs}`
  if (b.burnedIn) return `${t('player.sub')} · ${t('player.sources.subBurnedIn')}`
  if (b.soft) return `${t('player.sub')} · ${t('player.sources.subSelectable')}`
  return t('player.sub')
}
