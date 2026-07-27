// Package worker — цикл фоновых задач: claim, выполнение, завершение.
package worker

import (
	"context"
	"log/slog"
	"time"
)

// Handler обрабатывает задачу одного вида. Регистрируется в Registry;
// в каркасе реестр пуст — обработчики появляются вместе с ручками.
type Handler interface {
	Handle(ctx context.Context, payload []byte) error
}

type Options struct {
	ID           string
	Kinds        []string
	PollInterval time.Duration
	Log          *slog.Logger
}

type Loop struct {
	opts     Options
	handlers map[string]Handler
}

func New(opts Options) *Loop {
	if opts.PollInterval <= 0 {
		opts.PollInterval = 2 * time.Second
	}
	return &Loop{opts: opts, handlers: make(map[string]Handler)}
}

func (l *Loop) Register(kind string, handler Handler) {
	l.handlers[kind] = handler
}

func (l *Loop) Run(ctx context.Context) error {
	ticker := time.NewTicker(l.opts.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := l.tick(ctx); err != nil {
				l.opts.Log.Error("worker_tick_failed", "error", err)
			}
		}
	}
}

// TODO(ir): забирать задачу из jobs через FOR UPDATE SKIP LOCKED и звать
// handlers[kind]. Пока обработчиков нет, цикл только держит процесс живым.
func (l *Loop) tick(_ context.Context) error {
	return nil
}
