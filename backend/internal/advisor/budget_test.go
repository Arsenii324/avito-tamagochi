package advisor_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"tamagochi/internal/advisor"
)

// countingProvider считает, сколько раз до него реально дошёл вызов.
type countingProvider struct {
	calls atomic.Int64
}

func (c *countingProvider) Advise(context.Context, advisor.Situation) (advisor.Advice, error) {
	c.calls.Add(1)
	return advisor.Advice{Note: "ок", Tone: advisor.ToneNeutral}, nil
}

func TestBudgetStopsCallingModelAfterLimit(t *testing.T) {
	inner := &countingProvider{}
	b := advisor.NewBudgetProvider(inner, 3)

	for i := range 3 {
		if _, err := b.Advise(context.Background(), advisor.Situation{}); err != nil {
			t.Fatalf("вызов %d должен был пройти в пределах лимита: %v", i+1, err)
		}
	}

	for i := range 5 {
		_, err := b.Advise(context.Background(), advisor.Situation{})
		if !errors.Is(err, advisor.ErrBudgetExhausted) {
			t.Fatalf("вызов %d сверх лимита: ожидали ErrBudgetExhausted, получили %v", i+4, err)
		}
	}

	if got := inner.calls.Load(); got != 3 {
		t.Fatalf("модель позвали %d раз(а), а лимит был 3", got)
	}
	if got := b.Remaining(); got != 0 {
		t.Fatalf("Remaining() = %d, ожидали 0", got)
	}
}

func TestBudgetZeroMeansModelIsNeverCalled(t *testing.T) {
	inner := &countingProvider{}
	b := advisor.NewBudgetProvider(inner, 0)

	_, err := b.Advise(context.Background(), advisor.Situation{})
	if !errors.Is(err, advisor.ErrBudgetExhausted) {
		t.Fatalf("ожидали ErrBudgetExhausted, получили %v", err)
	}
	if got := inner.calls.Load(); got != 0 {
		t.Fatalf("модель позвали %d раз(а) при лимите 0", got)
	}
}

// Лимит обязан держаться при конкурентных запросах: /summary/daily может
// прийти одновременно с нескольких вкладок, и потолок, который держится
// только в один поток, — не потолок.
//
// Горутины отпускаются общим барьером, а не стартуют по мере создания:
// без барьера они выполняются почти последовательно, не пересекаются во
// времени, и тест зеленеет, даже если атомарности нет вовсе. В этом проекте
// такой «тест на гонку без гонки» уже дважды проходил, ничего не проверяя, —
// см. docs/AI-USAGE.md (internal/pet, internal/rewards).
func TestBudgetHoldsUnderConcurrentCalls(t *testing.T) {
	const (
		limit      = 10
		goroutines = 200
	)

	inner := &countingProvider{}
	b := advisor.NewBudgetProvider(inner, limit)

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(goroutines)

	var allowed atomic.Int64
	for range goroutines {
		go func() {
			defer done.Done()
			start.Wait() // все стартуют одновременно
			if _, err := b.Advise(context.Background(), advisor.Situation{}); err == nil {
				allowed.Add(1)
			}
		}()
	}

	start.Done()
	done.Wait()

	if got := allowed.Load(); got != limit {
		t.Fatalf("прошло %d вызовов, лимит был %d", got, limit)
	}
	if got := inner.calls.Load(); got != limit {
		t.Fatalf("модель позвали %d раз(а), лимит был %d", got, limit)
	}
	if got := b.Remaining(); got != 0 {
		t.Fatalf("Remaining() = %d, ожидали 0", got)
	}
}

// Исчерпанный бюджет для пользователя обязан выглядеть как обычный фолбэк:
// aiNote остаётся, просто его пишет шаблон.
func TestFallbackServesTemplateOnceBudgetIsSpent(t *testing.T) {
	inner := &countingProvider{}
	budgeted := advisor.NewBudgetProvider(inner, 1)
	f := advisor.NewFallbackProvider(budgeted, advisor.TemplateProvider{})

	situation := advisor.Situation{
		PetName:          "Ави",
		Level:            2,
		LowestStat:       "hunger",
		LowestStatValue:  20,
		AvailableActions: []string{"feed"},
	}

	first, err := f.Advise(context.Background(), situation)
	if err != nil {
		t.Fatalf("первый вызов: %v", err)
	}
	if first.Note == "" {
		t.Fatal("первый вызов вернул пустой Note")
	}

	second, err := f.Advise(context.Background(), situation)
	if err != nil {
		t.Fatalf("после исчерпания бюджета Advise обязан отдать совет шаблона, а не ошибку: %v", err)
	}
	if second.Note == "" {
		t.Fatal("после исчерпания бюджета Note пуст — фолбэк не сработал")
	}
	if got := inner.calls.Load(); got != 1 {
		t.Fatalf("модель позвали %d раз(а), ожидали ровно 1", got)
	}
}
