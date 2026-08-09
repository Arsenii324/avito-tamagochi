package social

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Слой данных: единственное место с SQL. Никаких доменных решений — те живут
// в service.go, этот файл только читает готовые числа.

// ErrNoEntry — у пользователя нет питомца, значит нечем ранжироваться.
var ErrNoEntry = errors.New("social: питомца нет — не в рейтинге")

// Repo — чтение лидерборда из тех же таблиц, что использует internal/pet:
// pets (total_xp, preset_id, name) и pet_action_log (xp_granted, day) для
// окна за 7 дней. Отдельных таблиц под лидерборд не заводится — READ ONLY
// поверх чужих данных, ровно так, как это описывает docs/DECISIONS.md
// («Postgres тянет лидерборд... на масштабе хакатона», решение против Redis).
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo собирает репозиторий.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// rankedCTE — общее ранжирование для Top и ByUser: ROW_NUMBER считает
// глобальную позицию по опыту за окно, тай-брейк user_id (см. service.go —
// стрика ещё нет). Один и тот же СTE в обоих запросах: два места, считающих
// ранг по-разному, разошлись бы молча при первой правке одного из них.
const rankedCTE = `
WITH weekly AS (
	SELECT user_id, COALESCE(SUM(xp_granted), 0) AS weekly_xp
	FROM pet_action_log
	WHERE day >= $1
	GROUP BY user_id
),
ranked AS (
	SELECT
		p.user_id,
		p.preset_id,
		p.name,
		p.total_xp,
		COALESCE(w.weekly_xp, 0) AS weekly_xp,
		ROW_NUMBER() OVER (ORDER BY COALESCE(w.weekly_xp, 0) DESC, p.user_id ASC) AS rank
	FROM pets p
	LEFT JOIN weekly w ON w.user_id = p.user_id
)
`

func scanRow(row pgx.Row) (Row, error) {
	var r Row
	var rank int64
	err := row.Scan(&r.UserID, &r.PresetID, &r.Name, &r.TotalXP, &r.WeeklyXP, &rank)
	if errors.Is(err, pgx.ErrNoRows) {
		return Row{}, ErrNoEntry
	}
	if err != nil {
		return Row{}, fmt.Errorf("social: чтение строки рейтинга: %w", err)
	}
	r.Rank = int(rank)
	return r, nil
}

// Top возвращает страницу рейтинга: ранги строго больше afterRank (0 — с
// начала), не больше limit строк.
func (r *Repo) Top(ctx context.Context, since time.Time, afterRank, limit int) ([]Row, error) {
	rows, err := r.pool.Query(ctx, rankedCTE+`
		SELECT user_id, preset_id, name, total_xp, weekly_xp, rank
		FROM ranked
		WHERE rank > $2
		ORDER BY rank
		LIMIT $3`,
		since, afterRank, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("social: страница рейтинга: %w", err)
	}
	defer rows.Close()

	// scanRow принимает pgx.Row, а rows — pgx.Rows: оба сводятся к одному и
	// тому же Scan(dest ...any) error, поэтому rows подходит без адаптера.
	// Так и та, и другая выборка проверяют колонки одним и тем же кодом —
	// разъехавшийся порядок колонок между Top и ByUser был бы ошибкой,
	// которую находят в проде, а не при чтении диффа.
	out := make([]Row, 0, limit)
	for rows.Next() {
		row, scanErr := scanRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, row)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("social: обход рейтинга: %w", rowsErr)
	}
	return out, nil
}

// ByUser возвращает ранг конкретного пользователя вне зависимости от
// страницы — контракт требует «me» отдельно от items, независимо от того,
// куда пролистал клиент.
func (r *Repo) ByUser(ctx context.Context, since time.Time, userID uuid.UUID) (Row, error) {
	return scanRow(r.pool.QueryRow(ctx, rankedCTE+`
		SELECT user_id, preset_id, name, total_xp, weekly_xp, rank
		FROM ranked
		WHERE user_id = $2`,
		since, userID,
	))
}
