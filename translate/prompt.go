package translate

import (
	"fmt"
	"strings"
)

// DefaultPromptTemplate - промпт перевода НАЗВАНИЯ на RU+TK + оценка веса.
// Фаза 0 (30.07): убраны 18 нормализованных характеристик и keywords (мёртвая генерация,
// гейтились pushDeepSeekFeatures=false; характеристики CS-Cart идут из attr_cs_mapping).
// Фаза 1 (30.07): убрана генерация ОПИСАНИЙ (крупнейшая часть output). Описания в CS-Cart
// = очищенный оригинальный HTML (push HTML-fallback), существующие LLM-описания сохраняются.
// Осталось: title_ru/tk, estimated_weight_grams (~200 ток/товар вместо 2200).
const DefaultPromptTemplate = `Ты - переводчик для e-commerce маркетплейса. Переведи НАЗВАНИЕ товара с китайского маркетплейса на 2 языка (русский, туркменский) и оцени вес. Верни ТОЛЬКО JSON.

Правила:
- Названия: существительное первым, без бренда, профессиональное e-commerce название. Включи ключевые детали из характеристик (материал, тип, особенность).
- Туркменский: официальный туркменский язык (НЕ турецкий). Алфавит: нет букв C, Q, V, X — вместо V пиши W.
  Обязательный словарь (нарушение = ошибка):
    одежда=geýim, футболка=futbolka, рубашка/платье=köýnek, брюки=balak, джинсы=jinsi balak,
    свитер=switer (не sviter!), куртка=kurtka, шорты=şortik, худи=hudi, лосины=dar balak,
    обувь=aýakgap, кроссовки=sport aýakgaby, сапоги=ädik, туфли=köwüş, тапочки=öý şypbygy,
    сумка=sumka, рюкзак=syrtyňa asylýan sumka, кошелек=gapjyk, шарф=şarf, носки=jorap,
    хлопок=pagta (не pamuk!), полиэстер=poliester, вискоза=wiskoza, шерсть=ýüň, шёлк=ýüpek,
    кожа=deri, замша=zamşa, нейлон=neýlon, трикотаж=trikotaž,
    черный=gara, белый=ak, красный=gyzyl, синий=gök, зеленый=ýaşyl, желтый=sary,
    розовый=gülgüne, серый=çal, коричневый=goňur, бежевый=açyk goňur, фиолетовый=melewşe,
    для женщин=aýallar üçin, для мужчин=erkekler üçin, для детей=çagalar üçin,
    удобный=amatly, стильный=döwrebap, качественный=ýokary hilli, мягкий=ýumşak,
    повседневный=gündelik, размер=ölçeg, Китай=Hytaý, Турция=Türkiýe
  Пример правильного названия: "Aýallar üçin jinsi balak, pagta, gara" (НЕ "Kadın kot pantolon, pamuk, siyah")

JSON формат (строго соблюдай ключи; только RU и TK, без English):
{
  "title_ru": "название на русском",
  "title_tk": "Türkmen ady",
  "estimated_weight_grams": "целое число - оценка веса товара в граммах (одна единица, без упаковки). Футболка ~200, джинсы ~800, куртка ~1200, обувь ~800, сумка ~500. Если в названии указан вес (g/kg/克/斤) - использовать его."
}`

func buildPromptWith(input NormalizeInput, customPrompt string) string {
	var attrs []string
	for name, value := range input.Attributes {
		attrs = append(attrs, fmt.Sprintf("%s=%s", name, value))
	}
	colors := strings.Join(input.Colors, ", ")
	systemPrompt := DefaultPromptTemplate
	if strings.TrimSpace(customPrompt) != "" {
		systemPrompt = customPrompt
	}
	return fmt.Sprintf(`%s

Входные данные товара:

Оригинал (CN): %s
Машинный перевод (RU): %s
Характеристики: %s
Цвета вариаций: %s

Ответ: только JSON.`, systemPrompt, input.TitleOriginal, input.TitleRu, strings.Join(attrs, ", "), colors)
}
