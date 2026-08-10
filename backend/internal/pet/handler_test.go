package pet_test

// Проверка валидации тела POST /pets на границе HTTP, а не в сервисе:
// enum контракта нарушается именно на входе, и падало это раньше не в домене,
// а у клиента, которому нечем нарисовать неизвестный пресет.

import (
	"bytes"
	"net/http"
	"testing"
)

// invariant-adjacent: контракт (docs/openapi.json → PresetId) допускает только
// green|blue|purple. До этого теста в базу проходил любой presetId: ShouldBindJSON
// проверяет типы и required, но не enum, а Valid() у сгенерированного типа никто
// не звал. Живьём это выглядело как 200 на presetId=notareal, а потом пустая
// страница у пользователя — фронт падал на питомце, которого нечем отрисовать.
func TestCreatePetRejectsUnknownPreset(t *testing.T) {
	f := newWSFixture(t)

	for _, preset := range []string{"notareal", "fox", "", "GREEN", "green "} {
		t.Run(preset, func(t *testing.T) {
			body := bytes.NewBufferString(`{"presetId":"` + preset + `","name":"Ави"}`)
			resp, err := http.Post(f.server.URL+"/api/v1/pets", "application/json", body)
			if err != nil {
				t.Fatalf("запрос: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("presetId=%q: получили %d, ожидали 422 — неизвестный пресет не должен доезжать до базы",
					preset, resp.StatusCode)
			}
		})
	}
}

// Обратная сторона: все три значения контракта обязаны проходить, иначе
// проверка выше «работает», просто запретив вообще всё.
func TestCreatePetAcceptsEveryContractPreset(t *testing.T) {
	for _, preset := range []string{"green", "blue", "purple"} {
		t.Run(preset, func(t *testing.T) {
			f := newWSFixture(t) // свой пользователь на каждый пресет: питомец один на аккаунт
			body := bytes.NewBufferString(`{"presetId":"` + preset + `","name":"Ави"}`)
			resp, err := http.Post(f.server.URL+"/api/v1/pets", "application/json", body)
			if err != nil {
				t.Fatalf("запрос: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("presetId=%q: получили %d, ожидали 200", preset, resp.StatusCode)
			}
		})
	}
}
