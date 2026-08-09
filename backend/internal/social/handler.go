package social

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"tamagochi/internal/api"
	"tamagochi/internal/httpx"
	"tamagochi/pkg/authctx"
)

// Слой транспорта: знает про Gin и типы контракта, не знает, как считается
// ранг или окно недели — то service.go. depguard (handler-layer) запрещает
// этому файлу импортировать pgx.

// Handler отвечает на HTTP для тега social.
type Handler struct {
	svc *Service
}

// NewHandler собирает обработчик.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register вешает маршруты тега social на переданную группу.
func (h *Handler) Register(r gin.IRoutes) {
	r.GET("/leaderboard", h.leaderboard)
}

// leaderboard отвечает на GET /leaderboard.
func (h *Handler) leaderboard(c *gin.Context) {
	userID, ok := authctx.UserID(c.Request.Context())
	if !ok {
		httpx.Fail(c.Writer, c.Request, http.StatusUnauthorized, api.UNAUTHORIZED, "Нужен вход")
		return
	}

	// Возвращаемое значение не нужно: единственный успешный исход ParseScope —
	// ScopeTop, второй раз проверять уже нечего.
	if _, err := ParseScope(c.Query("scope")); err != nil {
		msg := "scope должен быть одним из: top, league, friends"
		if errors.Is(err, ErrUnsupportedScope) {
			msg = "Этот scope лидерборда пока не реализован: доступен только top"
		}
		httpx.Fail(c.Writer, c.Request, http.StatusUnprocessableEntity, api.VALIDATIONERROR, msg)
		return
	}

	limit := NormalizeLimit(parseLimit(c.Query("limit")))

	page, err := h.svc.List(c.Request.Context(), userID, c.Query("cursor"), limit)
	switch {
	case errors.Is(err, ErrBadCursor):
		httpx.Fail(c.Writer, c.Request, http.StatusUnprocessableEntity, api.VALIDATIONERROR, "Некорректный cursor")
		return
	case err != nil:
		httpx.Fail(c.Writer, c.Request, http.StatusInternalServerError, api.INTERNALERROR, "Внутренняя ошибка сервера")
		return
	}

	httpx.OK(c.Writer, c.Request, http.StatusOK, toAPIPage(page), "Лидерборд")
}

// parseLimit разбирает limit из query. Некорректное значение (не число,
// отрицательное) читается как «лимит не задан» — NormalizeLimit подставит
// дефолт; отдельная 422 ради опечатки в необязательном параметре было бы
// суровее, чем того просит контракт.
func parseLimit(raw string) *int {
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return nil
	}
	return &n
}

// toAPIEntry переводит доменную Entry в тип контракта.
func toAPIEntry(e Entry) api.LeaderboardEntry {
	userID := e.UserID.String()
	level := e.Level
	nickname := e.Nickname
	presetID := api.PresetId(e.PresetID)
	rank := e.Rank
	streak := e.Streak
	weeklyXP := e.WeeklyXP
	isMe := e.IsMe
	return api.LeaderboardEntry{
		UserId:   &userID,
		Nickname: &nickname,
		PresetId: &presetID,
		Level:    &level,
		Streak:   &streak,
		WeeklyXp: &weeklyXP,
		Rank:     &rank,
		IsMe:     &isMe,
	}
}

// toAPIPage переводит доменную Page в тип контракта.
//
// League намеренно nil: поле относится к scope=league, которого этот срез
// не реализует (см. докстринг пакета). Пустое поле в JSON честнее выдуманных
// данных о лиге, которой нет.
func toAPIPage(p Page) api.LeaderboardPage {
	items := make([]api.LeaderboardEntry, 0, len(p.Items))
	for _, e := range p.Items {
		items = append(items, toAPIEntry(e))
	}

	scope := api.LeaderboardPageScope(ScopeTop)
	out := api.LeaderboardPage{
		Items: &items,
		Scope: &scope,
	}
	if p.NextCursor != "" {
		out.NextCursor = &p.NextCursor
	}
	if p.Me != nil {
		me := toAPIEntry(*p.Me)
		out.Me = &me
	}
	return out
}
