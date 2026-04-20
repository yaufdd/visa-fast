package translit

import "testing"

func TestRuToLatICAO(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Иванов Иван", "IVANOV IVAN"},
		{"Петров Сергей", "PETROV SERGEI"},
		{"Щегловский", "SHCHEGLOVSKII"},
		{"Ёлкина", "ELKINA"},
		{"Цой", "TCOI"},
		{"Жуков", "ZHUKOV"},
		{"Юрий Хромов", "IURII KHROMOV"},
		{"Яковлев", "IAKOVLEV"},
		{"Вячеслав", "VIACHESLAV"},
		{"Анна", "ANNA"},
		{"Мария", "MARIIA"},
		{"Абвгдеёжзийклмнопрстуфхцчшщъыьэюя",
			"ABVGDEEZHZIIKLMNOPRSTUFKHTCCHSHSHCHIEYEIUIA"},
		{"", ""},
		{"IVANOV", "IVANOV"}, // already Latin
	}
	for _, c := range cases {
		got := RuToLatICAO(c.in)
		if got != c.want {
			t.Errorf("RuToLatICAO(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
