package advisor

import (
	"context"
	"errors"
	"sync/atomic"
)

// ErrBudgetExhausted — лимит обращений к платной модели исчерпан. Возвращается
// вместо вызова, поэтому FallbackProvider трактует это как любую другую
// ошибку Primary и уходит на TemplateProvider: пользователь разницы не видит,
// aiNote не пропадает.
var ErrBudgetExhausted = errors.New("advisor: лимит обращений к модели исчерпан")

// BudgetProvider — жёсткий потолок на число обращений к платной модели за всё
// время жизни процесса.
//
// Зачем: /summary/daily зовёт модель на КАЖДЫЙ запрос, кэша ответов нет, а
// демо-стенд открыт в интернет. Без потолка один любопытный посетитель (или
// краулер, нашедший IP) тратит квоту ключа без ограничений. Счётчик делает
// расход ограниченным сверху числом, которое видно в конфигурации, а не
// «сколько получится».
//
// Резерв снимается ДО вызова, а не после успешного ответа: неудачный запрос
// к API тоже может стоить квоты, поэтому считаются попытки. Это сознательно
// консервативно — потолок ограничивает трату, а не число удачных советов.
//
// Исчерпав лимит, провайдер не восстанавливается сам: перезапуск процесса —
// единственный способ начать заново, и это тоже намеренно. Автосброс по
// таймеру превратил бы жёсткий потолок в скользящий лимит, то есть в другую
// гарантию, чем та, что здесь написана.
type BudgetProvider struct {
	inner Provider
	left  atomic.Int64
}

// NewBudgetProvider оборачивает inner потолком в maxCalls обращений.
// maxCalls <= 0 означает «не звонить вообще»: такой провайдер сразу отдаёт
// ErrBudgetExhausted, что для FallbackProvider равносильно работе только на
// шаблоне.
func NewBudgetProvider(inner Provider, maxCalls int) *BudgetProvider {
	b := &BudgetProvider{inner: inner}
	if maxCalls > 0 {
		b.left.Store(int64(maxCalls))
	}
	return b
}

// Advise реализует Provider.
func (b *BudgetProvider) Advise(ctx context.Context, s Situation) (Advice, error) {
	if !b.reserve() {
		return Advice{}, ErrBudgetExhausted
	}
	return b.inner.Advise(ctx, s)
}

// reserve атомарно занимает одно обращение. CAS-цикл, а не Add(-1): Add увёл
// бы счётчик в минус при конкурентных вызовах на исходе лимита, и тогда
// «сколько осталось» перестало бы быть правдой. Здесь счётчик не опускается
// ниже нуля ни при какой гонке, поэтому число вызовов inner не может
// превысить maxCalls.
func (b *BudgetProvider) reserve() bool {
	for {
		left := b.left.Load()
		if left <= 0 {
			return false
		}
		if b.left.CompareAndSwap(left, left-1) {
			return true
		}
	}
}

// Remaining — сколько обращений к модели ещё разрешено. Для логов и ручной
// проверки на защите («а что мешает этому стоить денег?»).
func (b *BudgetProvider) Remaining() int {
	return int(b.left.Load())
}
