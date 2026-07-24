package sync

import (
	"testing"

	"otapi-hub/otapi"
)

// sku — хелпер для сборки SKU с одной вариацией (Vid) и ценой.
func sku(vid string, price float64) otapi.SKU {
	return otapi.SKU{
		Configurators: []otapi.Configurator{{Pid: "颜色分类", Vid: vid}},
		Price:         otapi.Price{OriginalPrice: price},
	}
}

func TestIsAccessorySKU(t *testing.T) {
	cases := []struct {
		vid  string
		want bool
	}{
		{"单买滤芯高品质滤网实拍", true},   // купить фильтр отдельно
		{"单买一个地板刷高品质实拍", true},  // купить насадку отдельно
		{"替换装刷头", true},          // сменная насадка
		{"配件：电池", true},          // аксессуар
		{"107无刷款银灰色", false},     // сам товар (модель/цвет)
		{"107基础款 绿色", false},     // базовая модель
		{"红色", false},            // просто цвет
	}
	for _, c := range cases {
		if got := isAccessorySKU(sku(c.vid, 10)); got != c.want {
			t.Errorf("isAccessorySKU(%q) = %v, want %v", c.vid, got, c.want)
		}
	}
}

func TestRepresentativeCardPrice(t *testing.T) {
	// Кейс пылесоса: аксессуары 1.94/3/6.5, реальный товар от 35.
	skus := []otapi.SKU{
		sku("单买滤芯高品质滤网实拍", 1.94),
		sku("单买一个地板刷高品质实拍", 3.00),
		sku("单买一个VE包手机实拍", 6.50),
		sku("107基础款 灰白色", 35.00),
		sku("107无刷款银灰色", 60.00),
		sku("107无刷款套餐1", 74.00),
	}
	if got := representativeCardPrice(skus); got != 35.0 {
		t.Errorf("representativeCardPrice = %.2f, want 35.00 (мин среди не-аксессуаров)", got)
	}

	// Все вариации — аксессуары → 0 (не корректируем).
	allAcc := []otapi.SKU{sku("单买A", 1), sku("替换B", 2)}
	if got := representativeCardPrice(allAcc); got != 0 {
		t.Errorf("representativeCardPrice(all accessories) = %.2f, want 0", got)
	}

	// Нет аксессуаров → обычный минимум.
	normal := []otapi.SKU{sku("红色", 12), sku("蓝色 XL", 15)}
	if got := representativeCardPrice(normal); got != 12.0 {
		t.Errorf("representativeCardPrice(normal) = %.2f, want 12.00", got)
	}
}
