package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"tamagochi/internal/api"
	"tamagochi/internal/config"
	"tamagochi/internal/httpx"
	"tamagochi/internal/pet"
	"tamagochi/pkg/clock"
	"tamagochi/pkg/wsh"
)

// apiPrefix — префикс всех эндпоинтов контракта. Взят из блока servers в
// docs/openapi.json (http://localhost:8080/api/v1), а не придуман здесь.
const apiPrefix = "/api/v1"

// newRouter собирает роутер целиком.
//
// Вынесено из main отдельным файлом, а не дописано в main.go, по одной
// конкретной причине: main.go переписывает и feat/pet-service, и всякий, кто
// добавляет свой пакет, — это самый конфликтный файл бэкенда. Пока сборка
// живёт здесь, чужой мерж трогает три строки в main.go, а не весь wiring.
//
// pool обязателен, не *pgxpool.Pool-или-nil: приложению без базы нечего
// отвечать на /pet, а «роутер работает, но фича молча выключена» — это
// расхождение между тем, что говорит код, и тем, что видит пользователь.
// Тесты, которым база не нужна семантически (healthz, /config), всё равно
// поднимают её через pkg/pgtest — так же, как это уже делают тесты
// internal/pet, и по той же причине: собрать роутер без базы — значит
// проверить не тот роутер, что поедет в прод.
func newRouter(pool *pgxpool.Pool) (*gin.Engine, error) {
	// ReleaseMode: в отладочном Gin печатает в stdout список маршрутов и
	// предупреждение о режиме на каждом старте, включая каждый запуск тестов.
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	// Recovery, но не Logger: логировать каждый запрос через принтер Gin'а
	// незачем, а паника в обработчике не должна ронять процесс целиком.
	r.Use(gin.Recovery())

	// Без этого Gin отвечает 404 на существующий путь с другим методом, и
	// клиент контракта не отличит «нет такого ресурса» от «нельзя так».
	r.HandleMethodNotAllowed = true

	// Ответы на несуществующий маршрут — в том же конверте, что и всё
	// остальное. Иначе клиент, который умеет разбирать { data, meta },
	// на первой же опечатке в URL получает текст «404 page not found».
	r.NoRoute(func(c *gin.Context) {
		httpx.Fail(c.Writer, c.Request, http.StatusNotFound, api.NOTFOUND, "Ресурс не найден")
	})
	// Отдельного кода для 405 в перечислении ErrorCode нет; NOT_FOUND здесь
	// читается как «такого маршрута с таким методом нет», что и произошло.
	r.NoMethod(func(c *gin.Context) {
		httpx.Fail(c.Writer, c.Request, http.StatusMethodNotAllowed, api.NOTFOUND, "Метод не поддерживается")
	})

	// /healthz намеренно вне apiPrefix и вне конверта: его нет в контракте,
	// это служебная проверка для docker-compose и балансировщика, а не
	// эндпоинт для фронта.
	r.GET("/healthz", healthz)

	v1 := r.Group(apiPrefix)

	cfg, err := config.NewHandler(config.DefaultCurve, config.DefaultDailyCareXPCap)
	if err != nil {
		return nil, fmt.Errorf("сборка обработчика config: %w", err)
	}
	cfg.Register(v1)

	petSvc, err := pet.NewService(pet.NewRepo(pool), clock.Real{}, pet.DefaultEconomy, config.DefaultCurve, config.DefaultDailyCareXPCap)
	if err != nil {
		return nil, fmt.Errorf("сборка сервиса pet: %w", err)
	}

	// Демо-личность монтируется здесь, а не в main, потому что порядок важен:
	// r.Use на группе действует только на маршруты, зарегистрированные ПОСЛЕ
	// вызова. /config уже зарегистрирован строкой выше и остаётся публичным
	// (security: [] в контракте); всё, что регистрируется после этой строки,
	// проходит через withDemoIdentity.
	//
	// Вне APP_ENV=demo миддлварь не монтируется вообще: authctx.UserID тогда
	// всегда возвращает false, и pet-обработчики честно отвечают 401 —
	// закрыто по умолчанию, а не «работает, пока никто не заметил».
	if demoModeEnabled() {
		v1.Use(withDemoIdentity())
	}
	petHandler := pet.NewHandler(petSvc, wsh.NewHub(), clock.Real{})
	petHandler.Register(v1)

	// /ws — своя группа, а не v1: контракт монтирует сокет вне /api/v1
	// (docs/openapi.json → x-websocket.url). Group("") с пустым путём — это
	// приём Gin для «та же основа, но своя цепочка middleware»: Use на этой
	// группе не затрагивает ни v1, ни остальные маршруты r.
	ws := r.Group("")
	if demoModeEnabled() {
		ws.Use(withDemoIdentity())
	}
	petHandler.RegisterWS(ws)

	return r, nil
}

// healthz отвечает на служебную проверку живости.
func healthz(c *gin.Context) {
	c.Data(http.StatusOK, "application/json", []byte(`{"status":"ok"}`))
}
