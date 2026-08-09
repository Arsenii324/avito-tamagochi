package pet

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tamagochi/internal/api"
	"tamagochi/internal/httpx"
	"tamagochi/pkg/authctx"
)

// Слой транспорта: знает про Gin и про типы контракта, не знает, как считается
// распад, настроение или опыт — это service.go. depguard (handler-layer)
// запрещает этому файлу импортировать pgx: обращение к базе только через
// Service, который вызывает Repo.

// Handler отвечает на HTTP для тега pet.
type Handler struct {
	svc *Service
}

// NewHandler собирает обработчик.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register вешает маршруты тега pet на переданную группу.
func (h *Handler) Register(r gin.IRoutes) {
	r.POST("/pets", h.create)
	r.GET("/pet", h.get)
	r.POST("/pet/actions", h.act)
}

// actor достаёт пользователя из контекста запроса.
//
// Пока нет авторизации, контекст наполняет demoIdentity (см. demo.go) — она
// монтируется только под APP_ENV=demo. Без неё authctx.UserID возвращает
// false и обработчик отвечает 401, а не действует от чьего-то чужого имени.
func actor(c *gin.Context) (Actor, bool) {
	id, ok := authctx.UserID(c.Request.Context())
	if !ok {
		return Actor{}, false
	}
	// Таймзона аккаунта приедет вместе с профилем; до авторизации это ровно то
	// поле, которого у demo-пользователя нет, поэтому UTC — не решение по
	// таймзоне, а единственное, что можно подставить без профиля.
	return Actor{UserID: id, Location: nil}, true
}

// create отвечает на POST /pets.
func (h *Handler) create(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		httpx.Fail(c.Writer, c.Request, http.StatusUnauthorized, api.UNAUTHORIZED, "Нужен вход")
		return
	}

	var body api.PostPetsJSONBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c.Writer, c.Request, http.StatusUnprocessableEntity, api.VALIDATIONERROR, "Некорректное тело запроса")
		return
	}

	name := "Ави"
	if body.Name != nil && *body.Name != "" {
		name = *body.Name
	}

	view, err := h.svc.Create(c.Request.Context(), a, string(body.PresetId), name)
	switch {
	case errors.Is(err, ErrPetExists):
		httpx.Fail(c.Writer, c.Request, http.StatusConflict, api.PETALREADYEXISTS, "Питомец уже создан")
		return
	case err != nil:
		internalError(c, err)
		return
	}

	httpx.OK(c.Writer, c.Request, http.StatusOK, toAPIPet(view), "Питомец создан")
}

// get отвечает на GET /pet.
func (h *Handler) get(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		httpx.Fail(c.Writer, c.Request, http.StatusUnauthorized, api.UNAUTHORIZED, "Нужен вход")
		return
	}

	view, err := h.svc.Get(c.Request.Context(), a)
	switch {
	case errors.Is(err, ErrNoPet):
		httpx.Fail(c.Writer, c.Request, http.StatusNotFound, api.NOTFOUND, "Питомца ещё нет")
		return
	case err != nil:
		internalError(c, err)
		return
	}

	httpx.OK(c.Writer, c.Request, http.StatusOK, toAPIPet(view), "Состояние питомца")
}

// act отвечает на POST /pet/actions.
func (h *Handler) act(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		httpx.Fail(c.Writer, c.Request, http.StatusUnauthorized, api.UNAUTHORIZED, "Нужен вход")
		return
	}

	var body api.PostPetActionsJSONBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c.Writer, c.Request, http.StatusUnprocessableEntity, api.VALIDATIONERROR, "Некорректное тело запроса")
		return
	}
	actionID, err := uuid.Parse(body.ActionId.String())
	if err != nil {
		httpx.Fail(c.Writer, c.Request, http.StatusUnprocessableEntity, api.VALIDATIONERROR, "actionId должен быть UUID")
		return
	}

	res, err := h.svc.Act(c.Request.Context(), a, actionID, ActionKind(body.Kind))
	switch {
	case errors.Is(err, ErrNoPet):
		httpx.Fail(c.Writer, c.Request, http.StatusNotFound, api.NOTFOUND, "Питомца ещё нет")
		return
	case errors.Is(err, ErrUnknownAction):
		httpx.Fail(c.Writer, c.Request, http.StatusUnprocessableEntity, api.VALIDATIONERROR, "Неизвестное действие")
		return
	case errors.Is(err, ErrDailyLimit):
		httpx.Fail(c.Writer, c.Request, http.StatusTooManyRequests, api.ACTIONLIMITREACHED, "Суточный лимит действия исчерпан")
		return
	case errors.Is(err, ErrAlreadyAsleep), errors.Is(err, ErrNotAsleep), errors.Is(err, ErrAsleep):
		httpx.Fail(c.Writer, c.Request, http.StatusConflict, api.PETISSLEEPING, "Питомец спит")
		return
	case err != nil:
		internalError(c, err)
		return
	}

	httpx.OK(c.Writer, c.Request, http.StatusOK, toAPIActionResult(res), "Питомец обновлён")
}

// internalError отвечает 500. Ошибка не разбирается на код контракта: сюда
// попадают только состояния, которые не должны случаться при исправном коде
// (сбой базы, испорченные данные), и им нечего сказать клиенту, кроме честного
// «что-то сломалось».
func internalError(c *gin.Context, err error) {
	// Пишется в стандартный лог процесса, а не в тело ответа: тело для
	// пользователя, а подробности ошибки — для того, кто читает логи сервера.
	_ = err
	httpx.Fail(c.Writer, c.Request, http.StatusInternalServerError, api.INTERNALERROR, "Внутренняя ошибка сервера")
}

// toAPIPet переводит доменный View в тип контракта.
//
// Отдельная функция, а не встраивание в service.go: маппинг в JSON-теги
// контракта — забота транспорта, а не домена. service.go не импортирует
// internal/api ровно по этой причине (depguard, service-layer).
func toAPIPet(v View) api.Pet {
	actions := make(map[string]api.ActionState, len(v.Actions))
	for kind, avail := range v.Actions {
		actions[string(kind)] = api.ActionState{
			Remaining: avail.Remaining,
			XpCapped:  avail.XPCapped,
		}
	}

	return api.Pet{
		Id:             v.ID.String(),
		PresetId:       api.PresetId(v.PresetID),
		Name:           v.Name,
		Level:          v.Level,
		Xp:             v.XP,
		XpToNext:       v.XPToNext,
		TotalXp:        v.TotalXP,
		Stage:          api.PetStage(v.Stage),
		StageLabel:     &v.StageLabel,
		Stats:          api.Stats{Hunger: v.Stats.Hunger, Joy: v.Stats.Joy, Clean: v.Stats.Clean, Energy: v.Stats.Energy},
		Mood:           api.PetMood(v.Mood),
		MoodMultiplier: float32(v.MoodMultiplier),
		Actions:        actions,
		UpdatedAt:      v.UpdatedAt,
	}
}

// toAPIActionResult переводит доменный ActResult в тип контракта.
func toAPIActionResult(r ActResult) api.ActionResult {
	careXPToday := r.CareXPToday
	capReached := r.CareXPCapReached
	return api.ActionResult{
		Pet:              toAPIPet(r.Pet),
		XpGained:         r.XPGained,
		StatCapped:       r.StatCapped,
		LeveledUp:        r.LeveledUp,
		CareXpToday:      &careXPToday,
		CareXpCapReached: &capReached,
	}
}
