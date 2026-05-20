package scheduler

import (
	"context"
	"sync"
	"time"

	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

type ServerLister interface {
	List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error)
}

type ServerResolver interface {
	ResolveServer(ctx context.Context, serverID int64) (domain.Server, error)
}

type StatusWriter interface {
	UpsertLatest(ctx context.Context, serverID int64, snapshot collector.Snapshot, sampledAt time.Time) error
	AppendHistory(ctx context.Context, serverID int64, snapshot collector.Snapshot, sampledAt time.Time) error
	MarkCollectStarted(ctx context.Context, serverID int64, startedAt time.Time) error
	MarkCollectFailure(ctx context.Context, serverID int64, err error, finishedAt time.Time) error
	MarkDisabled(ctx context.Context, serverID int64, finishedAt time.Time) error
	MarkStaleBefore(ctx context.Context, cutoff time.Time) error
}

type AlertEvaluator interface {
	EvaluateServerSnapshot(ctx context.Context, server domain.Server, snapshot collector.Snapshot, sampledAt time.Time) error
}

type DisabledAlertResolver interface {
	ResolveServerAlerts(ctx context.Context, server domain.Server, detail string, occurredAt time.Time) error
}

type Poller struct {
	servers     ServerLister
	resolver    ServerResolver
	collector   *collector.SSHCollector
	statuses    StatusWriter
	alerts      AlertEvaluator
	interval    time.Duration
	concurrency int
}

func NewPoller(
	servers ServerLister,
	resolver ServerResolver,
	collector *collector.SSHCollector,
	statuses StatusWriter,
	alerts AlertEvaluator,
	interval time.Duration,
	concurrency int,
) *Poller {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &Poller{
		servers:     servers,
		resolver:    resolver,
		collector:   collector,
		statuses:    statuses,
		alerts:      alerts,
		interval:    interval,
		concurrency: concurrency,
	}
}

func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.collectOnce(ctx)
		}
	}
}

func (p *Poller) collectOnce(ctx context.Context) {
	servers, err := p.servers.List(ctx, storage.ServerFilter{})
	if err != nil {
		return
	}

	sampledAt := time.Now().UTC()
	sem := make(chan struct{}, p.concurrency)
	var wg sync.WaitGroup

	for _, server := range servers {
		if !server.Enabled {
			_ = p.statuses.MarkDisabled(ctx, server.ID, sampledAt)
			if resolver, ok := p.alerts.(DisabledAlertResolver); ok {
				_ = resolver.ResolveServerAlerts(ctx, server, "server disabled", sampledAt)
			}
			continue
		}

		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}

		wg.Add(1)
		go func(server domain.Server) {
			defer wg.Done()
			defer func() { <-sem }()

			_ = p.statuses.MarkCollectStarted(ctx, server.ID, sampledAt)
			resolvedServer, err := p.resolver.ResolveServer(ctx, server.ID)
			if err != nil {
				_ = p.statuses.MarkCollectFailure(ctx, server.ID, err, time.Now().UTC())
				return
			}
			if !resolvedServer.Enabled {
				_ = p.statuses.MarkDisabled(ctx, server.ID, time.Now().UTC())
				return
			}
			snapshot, err := p.collector.Collect(ctx, resolvedServer)
			if err != nil {
				finishedAt := time.Now().UTC()
				_ = p.statuses.MarkCollectFailure(ctx, server.ID, err, finishedAt)
				if p.alerts != nil {
					_ = p.alerts.EvaluateServerSnapshot(ctx, resolvedServer, collector.Snapshot{Online: false, SSHOK: false, Source: "ssh"}, finishedAt)
				}
				return
			}
			_ = p.statuses.UpsertLatest(ctx, server.ID, snapshot, sampledAt)
			_ = p.statuses.AppendHistory(ctx, server.ID, snapshot, sampledAt)
			if p.alerts != nil {
				_ = p.alerts.EvaluateServerSnapshot(ctx, resolvedServer, snapshot, sampledAt)
			}
		}(server)
	}

	wg.Wait()
	_ = p.statuses.MarkStaleBefore(ctx, sampledAt.Add(-p.interval*2))
}
