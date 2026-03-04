export const genreEmojis: Record<string, string> = {
  'Action': '⚔️',
  'Adventure': '🗺️',
  'Comedy': '😂',
  'Drama': '🎭',
  'Fantasy': '🧙',
  'Horror': '👻',
  'Mystery': '🔍',
  'Romance': '💕',
  'Sci-Fi': '🚀',
  'Slice of Life': '☕',
  'Sports': '⚽',
  'Supernatural': '✨',
  'Thriller': '😱',
  'Mecha': '🤖',
  'Music': '🎵',
  'Psychological': '🧠',
  'School': '🏫',
  'Shounen': '👊',
  'Shoujo': '🌸',
  'Isekai': '🌀',
}

export function getGenreEmoji(name: string): string {
  return genreEmojis[name] || '🎬'
}
