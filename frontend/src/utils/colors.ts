// Канонические русские цвета → HEX. Для отображения значений-цветов «в виде цвета».
// Должно совпадать с backend translate/colors.go.
export const COLOR_HEX: Record<string, string> = {
  'светло-серый': '#D3D3D3', 'тёмно-серый': '#555555', 'темно-серый': '#555555', 'антрацит': '#2F2F2F',
  'серебряный': '#CFD8DC', 'серебренный': '#CFD8DC', 'серый': '#9E9E9E',
  'тёмно-синий': '#0D47A1', 'темно-синий': '#0D47A1', 'светло-голубой': '#B3E5FC', 'голубой': '#64B5F6',
  'индиго': '#3F51B5', 'бирюзовый': '#00BCD4', 'морской': '#006064', 'синий': '#1565C0',
  'тёмно-зелёный': '#1B5E20', 'темно-зеленый': '#1B5E20', 'изумрудный': '#00897B',
  'фисташковый': '#AED581', 'мятный': '#80CBC4', 'зелёный': '#43A047', 'зеленый': '#43A047',
  'бордовый': '#7B1FA2', 'малиновый': '#C62828', 'фуксия': '#FF4081', 'красный': '#E53935',
  'светло-розовый': '#FFCDD2', 'пудро-розовый': '#F8BBD0', 'розовый': '#F06292',
  'горчичный': '#F9A825', 'оранжевый': '#FB8C00',
  'золотистый': '#FFD54F', 'золотой': '#FFD54F', 'жёлтый': '#FDD835', 'желтый': '#FDD835',
  'сиреневый': '#CE93D8', 'лиловый': '#AB47BC', 'пурпурный': '#8E24AA', 'фиолетовый': '#7B1FA2',
  'бежевый': '#D7CCC8', 'экрю': '#F5F5DC', 'песочный': '#FFCC80', 'ванильный': '#FFF9C4',
  'персиковый': '#FFAB91', 'коричневый': '#6D4C41', 'хаки': '#8D9B6A',
  'чёрный': '#1A1A1A', 'черный': '#1A1A1A', 'белый': '#FFFFFF',
  'леопардовый': '#C49A00', 'прозрачный': 'rgba(200,200,200,0.4)',
  'разноцветный': 'linear-gradient(to right, red,orange,yellow,green,blue,indigo,violet)',
}

// getColorHex — подбирает HEX по подстроке в названии цвета.
export function getColorHex(name: string): string | undefined {
  if (!name) return undefined
  const lower = name.toLowerCase()
  for (const [key, hex] of Object.entries(COLOR_HEX)) {
    if (lower.includes(key)) return hex
  }
  return undefined
}

// isColorAttr — относится ли название атрибута к цвету.
export function isColorAttr(name: string): boolean {
  const n = (name || '').toLowerCase()
  return n.includes('цвет') || n.includes('color') || n.includes('色')
}
