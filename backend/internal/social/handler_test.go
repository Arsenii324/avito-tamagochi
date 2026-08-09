package social_test

// Тест через настоящий HTTP, а не вызов Handler-методов напрямую: имена
// полей JSON — то самое, что уже один раз разошлось молча в internal/pet/ws.go
// (доменный тип без json-тегов вместо типа контракта). Разбор реального тела
// ответа — единственный способ поймать такую ошибку до реального клиента.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"tamagochi/internal/config"
	"tamagochi/internal/social"
	"tamagochi/pkg/authctx"
	"tamagochi/pkg/clock"
	"tamagochi/pkg/pgtest"
)

type handlerFixture struct {
	server *httptest.Server
	pool   *pgxpool.Pool
	userID uuid.UUID
}

func newHandlerFixture(t *testing.T) *handlerFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	svc, err := social.NewService(social.NewRepo(pool), clock.NewFixed(base), config.DefaultCurve)
	if err != nil {
		t.Fatalf("сборка сервиса: %v", err)
	}
	h := social.NewHandler(svc)

	userID := uuid.New()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	identity := func(c *gin.Context) {
		c.Request = c.Request.WithContext(authctx.WithUserID(c.Request.Context(), userID))
		c.Next()
	}
	h.Register(r.Group("/api/v1", identity))

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &handlerFixture{server: srv, pool: pool, userID: userID}
}

// seedPet — тот же приём, что в repo_test.go: этот пакет только читает
// pets/pet_action_log, поэтому его тестам можно заполнять их напрямую.
func (f *handlerFixture) seedPet(t *testing.T, userID uuid.UUID, name string, totalXP int) {
	t.Helper()
	_, err := f.pool.Exec(context.Background(), `
		INSERT INTO pets (id, user_id, preset_id, name, hunger, joy, clean, energy, sleeping, stats_at, total_xp)
		VALUES ($1, $2, 'blue', $3, 100, 100, 100, 100, false, $4, $5)`,
		uuid.New(), userID, name, base, totalXP,
	)
	if err != nil {
		t.Fatalf("сидирование питомца: %v", err)
	}
	_, err = f.pool.Exec(context.Background(), `
		INSERT INTO pet_action_log (user_id, action_id, kind, xp_granted, day, result)
		VALUES ($1, $2, 'feed', $3, $4, '{}'::jsonb)`,
		userID, uuid.New(), totalXP, day(0),
	)
	if err != nil {
		t.Fatalf("сидирование опыта: %v", err)
	}
}

func TestLeaderboardHTTPFieldNames(t *testing.T) {
	f := newHandlerFixture(t)
	f.seedPet(t, f.userID, "ФронтТестик", 42)

	resp, err := http.Get(f.server.URL + "/api/v1/leaderboard?scope=top")
	if err != nil {
		t.Fatalf("GET /leaderboard: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("статус %d, ожидался 200", resp.StatusCode)
	}

	var env struct {
		Data map[string]any `json:"data"`
		Meta map[string]any `json:"meta"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&env); decodeErr != nil {
		t.Fatalf("ответ не разбирается: %v", decodeErr)
	}

	if scope, _ := env.Data["scope"].(string); scope != "top" {
		t.Errorf("data.scope = %v, ожидалось top", env.Data["scope"])
	}

	items, ok := env.Data["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("data.items = %v (%T), ожидался один элемент", env.Data["items"], env.Data["items"])
	}
	entry, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("items[0] не объект: %T", items[0])
	}

	// Имена полей — ровно контрактные (camelCase из docs/openapi.json →
	// LeaderboardEntry), а не имена Go-полей доменной Entry.
	for field, want := range map[string]any{
		"userId":   f.userID.String(),
		"nickname": "ФронтТестик",
		"presetId": "blue",
		"weeklyXp": float64(42),
		"rank":     float64(1),
		"streak":   float64(0),
		"isMe":     true,
	} {
		got, present := entry[field]
		if !present {
			t.Errorf("items[0].%s отсутствует; ключи: %v", field, entry)
			continue
		}
		if got != want {
			t.Errorf("items[0].%s = %v, ожидалось %v", field, got, want)
		}
	}
	if _, present := entry["Nickname"]; present {
		t.Error("items[0] содержит Nickname с большой буквы — поле домена, а не тег контракта")
	}

	// «me» — тот же пользователь, независимо от того, что он же в items.
	me, ok := env.Data["me"].(map[string]any)
	if !ok {
		t.Fatalf("data.me = %v, ожидался объект", env.Data["me"])
	}
	if me["userId"] != f.userID.String() {
		t.Errorf("data.me.userId = %v, ожидалось %v", me["userId"], f.userID.String())
	}
}

// scope=league и scope=friends отвечают понятной 422, а не 500 и не молча
// отдают top: контракт различает три scope, а этот срез поддерживает один.
// Сообщение обязано отличаться от «scope не существует» (см. TestLeaderboard
// RejectsGarbageScope) — это найденный вручную баг: ParseScope раньше
// заворачивал обе причины в один код ошибки, и ветка с другим текстом в
// handler.go была мертва — errors.Is был истинным всегда, второе сообщение
// никогда не показывалось.
func TestLeaderboardRejectsUnimplementedScopes(t *testing.T) {
	f := newHandlerFixture(t)

	for _, scope := range []string{"league", "friends"} {
		status, meta := getLeaderboardMeta(t, f.server.URL+"/api/v1/leaderboard?scope="+scope)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("scope=%s: статус %d, ожидался %d", scope, status, http.StatusUnprocessableEntity)
		}
		msg, _ := meta["message"].(string)
		if msg != "Этот scope лидерборда пока не реализован: доступен только top" {
			t.Errorf("scope=%s: message = %q, ожидалось сообщение про «пока не реализован»", scope, msg)
		}
	}
}

// scope, которого контракт вообще не знает, — другое сообщение, не
// «пока не реализован» (это про league/friends, которые в контракте есть).
func TestLeaderboardRejectsGarbageScope(t *testing.T) {
	f := newHandlerFixture(t)

	for _, scope := range []string{"bogus", "TOP"} {
		status, meta := getLeaderboardMeta(t, f.server.URL+"/api/v1/leaderboard?scope="+scope)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("scope=%s: статус %d, ожидался %d", scope, status, http.StatusUnprocessableEntity)
		}
		msg, _ := meta["message"].(string)
		if msg != "scope должен быть одним из: top, league, friends" {
			t.Errorf("scope=%s: message = %q, ожидалось сообщение про допустимые значения", scope, msg)
		}
	}
}

// getLeaderboardMeta делает GET, закрывает тело и возвращает статус вместе
// с разобранным meta — обеим функциям выше нужно и то, и другое, а держать
// снаружи *http.Response с уже закрытым телом было бы обманчивым API.
func getLeaderboardMeta(t *testing.T, url string) (status int, meta map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	var env struct {
		Meta map[string]any `json:"meta"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&env); decodeErr != nil {
		t.Fatalf("ответ не разбирается: %v", decodeErr)
	}
	return resp.StatusCode, env.Meta
}

// Без scope в query — тоже отказ, а не тихий дефолт на top: контракт
// объявляет scope required.
func TestLeaderboardRequiresScope(t *testing.T) {
	f := newHandlerFixture(t)

	resp, err := http.Get(f.server.URL + "/api/v1/leaderboard")
	if err != nil {
		t.Fatalf("GET /leaderboard: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("без scope: статус %d, ожидался %d", resp.StatusCode, http.StatusUnprocessableEntity)
	}
}

func TestLeaderboardRequiresIdentity(t *testing.T) {
	pool := pgtest.Pool(t)
	svc, err := social.NewService(social.NewRepo(pool), clock.NewFixed(base), config.DefaultCurve)
	if err != nil {
		t.Fatalf("сборка сервиса: %v", err)
	}
	h := social.NewHandler(svc)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.Register(r.Group("/api/v1")) // без идентичности

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/leaderboard?scope=top")
	if err != nil {
		t.Fatalf("GET /leaderboard: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("без идентичности: статус %d, ожидался %d", resp.StatusCode, http.StatusUnauthorized)
	}
}
