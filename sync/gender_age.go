package sync

import (
	"otapi-hub/otapi"
	"strings"
)

// NormalizeGenderAge извлекает пол и возрастную группу из атрибутов товара OT (китайские значения).
// Источники (1688/Taobao):
//   - пол:    属性 «适用性别» → 男 / 女 / 中性·男女均可
//   - возраст: 属性 «适合年龄段» → 新生儿·婴幼童 (0–3) / 中小童 (3–8) / 中大童 (8+)
//
// Возвращает нейтральные токены:
//   gender:    "male" | "female" | "unisex" | ""
//   ageGroup:  CSV из {baby,kids,children} — товар может подходить нескольким группам (напр. "baby,kids")
func NormalizeGenderAge(attrs []otapi.Attribute) (gender, ageGroup string) {
	ageSet := map[string]bool{}
	for _, a := range attrs {
		name := a.PropertyName
		val := a.Value
		switch {
		case strings.Contains(name, "性别"): // 适用性别 / 性别
			if g := normGender(val); g != "" {
				gender = g
			}
		case strings.Contains(name, "年龄"): // 适合年龄段 / 年龄段
			if ag := normAge(val); ag != "" {
				ageSet[ag] = true
			}
		}
	}
	// стабильный порядок baby<kids<children
	for _, b := range []string{"baby", "kids", "children"} {
		if ageSet[b] {
			if ageGroup != "" {
				ageGroup += ","
			}
			ageGroup += b
		}
	}
	return gender, ageGroup
}

func normGender(v string) string {
	switch {
	case strings.Contains(v, "男女"), strings.Contains(v, "中性"), strings.Contains(v, "通用"):
		return "unisex"
	case strings.Contains(v, "女"):
		return "female"
	case strings.Contains(v, "男"):
		return "male"
	}
	return ""
}

func normAge(v string) string {
	switch {
	case strings.Contains(v, "新生"), strings.Contains(v, "婴"): // 新生儿 / 婴幼童 (0–3)
		return "baby"
	case strings.Contains(v, "中小童"), strings.Contains(v, "小童"): // 3–8
		return "kids"
	case strings.Contains(v, "中大童"), strings.Contains(v, "大童"): // 8+
		return "children"
	}
	// запасной разбор по диапазону лет в скобках, напр. «(1~3岁...)»
	switch {
	case strings.Contains(v, "0~1"), strings.Contains(v, "1~3"):
		return "baby"
	case strings.Contains(v, "3~8"), strings.Contains(v, "3-8"):
		return "kids"
	case strings.Contains(v, "8岁以上"):
		return "children"
	}
	return ""
}
