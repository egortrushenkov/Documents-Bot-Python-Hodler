// Package util — мелкие общие помощники: экранирование, числа, имена файлов.
package util

import (
	"html"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// Esc экранирует пользовательский текст для parse_mode=HTML.
func Esc(v string) string { return html.EscapeString(v) }

// SafeNamePart — часть имени файла из названия компании: только буквы/цифры/_-.
func SafeNamePart(name string, limit int, def string) string {
	if name == "" {
		name = def
	}
	var b strings.Builder
	for _, ch := range name {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '-' || ch == '.' {
			b.WriteRune(ch)
		} else {
			b.WriteRune('_')
		}
	}
	s := b.String()
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	s = TruncRunes(strings.Trim(s, "_"), limit)
	if s == "" {
		return def
	}
	return s
}

// TruncRunes обрезает строку до n символов (не байтов).
func TruncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// ParseFloat — «мягкий» разбор числа: запятая как разделитель, пробелы игнорируются.
func ParseFloat(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), " ", "")
	s = strings.ReplaceAll(s, " ", "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// ToFloat — как ParseFloat, но 0 при ошибке.
func ToFloat(s string) float64 {
	f, _ := ParseFloat(s)
	return f
}

// PyFloat печатает float так же, как str(float) в Python: 80.0, 6287.726.
func PyFloat(f float64) string {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.ContainsAny(s, ".e") {
		s += ".0"
	}
	return s
}

// Num печатает float без лишнего «.0»: 500000, 6287.726.
func Num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// Fmt2 — формат с 2 знаками без хвостовых нулей: 12.50 → 12.5, 10.00 → 10.
func Fmt2(f float64) string {
	s := strconv.FormatFloat(f, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// Round2 — округление до копеек.
func Round2(f float64) float64 { return math.Round(f*100) / 100 }

// groupThousands: 1234567 → «1 234 567».
func groupThousands(intPart string) string {
	neg := strings.HasPrefix(intPart, "-")
	intPart = strings.TrimPrefix(intPart, "-")
	var out []byte
	for i := range len(intPart) {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			out = append(out, ' ')
		}
		out = append(out, intPart[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// FormatNumber — число для документа: разряды через пробел, до 6 знаков после точки.
// Нечисловое значение возвращается как есть.
func FormatNumber(value string) string {
	n, ok := ParseFloat(value)
	if !ok || math.IsInf(n, 0) || math.IsNaN(n) {
		return value
	}
	if n == math.Trunc(n) && math.Abs(n) < 1e18 {
		return groupThousands(strconv.FormatInt(int64(n), 10))
	}
	s := strconv.FormatFloat(n, 'f', 6, 64)
	s = strings.TrimSuffix(strings.TrimRight(s, "0"), ".")
	parts := strings.SplitN(s, ".", 2)
	parts[0] = groupThousands(parts[0])
	return strings.Join(parts, ".")
}

// RU — число по-русски: разряды через пробел, запятая, ровно dec знаков.
// RU(22629.97, 2) = «22 629,97».
func RU(f float64, dec int) string {
	s := strconv.FormatFloat(f, 'f', dec, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	out := groupThousands(intPart)
	if dec > 0 {
		out += "," + frac
	}
	return out
}

// RUAmount — сумма: без копеек, если целая (1 850 000), иначе 2 знака.
func RUAmount(f float64) string {
	if math.Abs(f-math.Round(f)) < 0.005 {
		return RU(math.Round(f), 0)
	}
	return RU(f, 2)
}
