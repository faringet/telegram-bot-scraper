package classifier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

type WarmupClient interface {
	Warmup(ctx context.Context, model string) error
}

type ScheduleConfig struct {
	Timezone        string
	RunTimes        []string
	MaxRunDuration  time.Duration
	WarmupBeforeRun bool
}

type scheduledHM struct {
	hour   int
	minute int
	raw    string
}

type ScheduledWorker struct {
	log    *slog.Logger
	cfg    ScheduleConfig
	model  string
	loc    *time.Location
	times  []scheduledHM
	worker *Worker
	warmup WarmupClient
}

func NewScheduledWorker(log *slog.Logger, cfg ScheduleConfig, model string, worker *Worker, warmup WarmupClient) (*ScheduledWorker, error) {
	if log == nil {
		log = slog.Default()
	}
	if worker == nil {
		return nil, errors.New("classifier scheduled worker: worker is nil")
	}

	cfg.Timezone = strings.TrimSpace(cfg.Timezone)
	if cfg.Timezone == "" {
		cfg.Timezone = "Europe/Moscow"
	}
	if cfg.MaxRunDuration <= 0 {
		cfg.MaxRunDuration = 3 * time.Hour
	}

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("classifier scheduled worker: load timezone: %w", err)
	}

	runTimes, err := parseScheduledRunTimes(cfg.RunTimes)
	if err != nil {
		return nil, fmt.Errorf("classifier scheduled worker: %w", err)
	}

	model = strings.TrimSpace(model)
	if model == "" {
		model = "qwen2.5:7b"
	}

	baseLog := log.With(
		slog.String("layer", "worker"),
		slog.String("module", "classifier.scheduled_worker"),
	)

	return &ScheduledWorker{
		log:    baseLog,
		cfg:    cfg,
		model:  model,
		loc:    loc,
		times:  runTimes,
		worker: worker,
		warmup: warmup,
	}, nil
}

func (w *ScheduledWorker) Run(ctx context.Context) error {
	if w == nil {
		return errors.New("classifier scheduled worker: worker is nil")
	}

	w.log.Info("scheduled worker started",
		slog.String("timezone", w.loc.String()),
		slog.Any("run_times", rawRunTimes(w.times)),
		slog.Duration("max_run_duration", w.cfg.MaxRunDuration),
		slog.Bool("warmup_before_run", w.cfg.WarmupBeforeRun),
		slog.String("model", w.model),
	)

	for {
		now := time.Now().In(w.loc)
		nextRun := nextScheduledRun(now, w.times, w.loc)
		wait := time.Until(nextRun)

		w.log.Info("next scheduled run",
			slog.Time("next_run", nextRun),
			slog.Duration("wait", wait),
		)

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}

		runStart := time.Now().In(w.loc)
		runDeadline := runStart.Add(w.cfg.MaxRunDuration)

		w.log.Info("scheduled run started",
			slog.Time("run_start", runStart),
			slog.Time("run_deadline", runDeadline),
		)

		if w.cfg.WarmupBeforeRun && w.warmup != nil {
			warmupTimeout := 2 * time.Minute
			if w.cfg.MaxRunDuration > 0 && w.cfg.MaxRunDuration < warmupTimeout {
				warmupTimeout = w.cfg.MaxRunDuration
			}

			warmupCtx, cancel := context.WithTimeout(ctx, warmupTimeout)
			err := w.warmup.Warmup(warmupCtx, w.model)
			cancel()

			if err != nil {
				w.log.Warn("ollama warmup failed", slog.Any("err", err))
			} else {
				w.log.Info("ollama warmup completed")
			}
		}

		runCtx, cancel := context.WithDeadline(ctx, runDeadline)
		err := w.worker.Run(runCtx)
		cancel()

		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		w.log.Info("scheduled run finished",
			slog.Time("run_start", runStart),
			slog.Time("run_deadline", runDeadline),
		)
	}
}

func parseScheduledRunTimes(vals []string) ([]scheduledHM, error) {
	out := make([]scheduledHM, 0, len(vals))
	seen := make(map[string]struct{}, len(vals))

	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}

		t, err := time.Parse("15:04", v)
		if err != nil {
			return nil, fmt.Errorf("invalid run_time %q: must be HH:MM: %w", v, err)
		}

		out = append(out, scheduledHM{
			hour:   t.Hour(),
			minute: t.Minute(),
			raw:    v,
		})
		seen[v] = struct{}{}
	}

	if len(out) == 0 {
		return nil, errors.New("run_times must not be empty")
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].hour != out[j].hour {
			return out[i].hour < out[j].hour
		}
		return out[i].minute < out[j].minute
	})

	return out, nil
}

func nextScheduledRun(now time.Time, times []scheduledHM, loc *time.Location) time.Time {
	y, m, d := now.Date()

	for _, rt := range times {
		candidate := time.Date(y, m, d, rt.hour, rt.minute, 0, 0, loc)
		if candidate.After(now) {
			return candidate
		}
	}

	first := times[0]
	return time.Date(y, m, d+1, first.hour, first.minute, 0, 0, loc)
}

func rawRunTimes(times []scheduledHM) []string {
	out := make([]string, 0, len(times))
	for _, t := range times {
		out = append(out, t.raw)
	}
	return out
}
