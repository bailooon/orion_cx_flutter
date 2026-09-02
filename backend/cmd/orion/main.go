// Command orion runs one Orion CX service, selected with -service.
//
// The same binary backs all five containers of docker-compose. The extra mode
// `-service=all` starts every service inside a single process, which is what
// makes the prototype runnable with `go run` and no Docker, Postgres, Redis or
// Kafka installed.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/orion-cx/orion-backend/internal/authsvc"
	"github.com/orion-cx/orion-backend/internal/callsvc"
	"github.com/orion-cx/orion-backend/internal/config"
	"github.com/orion-cx/orion-backend/internal/gatewaysvc"
	"github.com/orion-cx/orion-backend/internal/nlpsvc"
	"github.com/orion-cx/orion-backend/internal/notifysvc"
	"github.com/orion-cx/orion-backend/internal/platform/bus"
	"github.com/orion-cx/orion-backend/internal/platform/cache"
	"github.com/orion-cx/orion-backend/internal/platform/db"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
	"github.com/orion-cx/orion-backend/internal/platform/logging"
	"github.com/orion-cx/orion-backend/internal/platform/security"
	"github.com/orion-cx/orion-backend/internal/seed"
)

func main() {
	service := flag.String("service", envOrDefault("ORION_SERVICE", config.ServiceAll),
		"which service to run: gateway, nlp, auth, callmgmt, notification, seed, all")
	flag.Parse()

	if err := run(*service); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func run(service string) error {
	cfg, err := config.Load(service)
	if err != nil {
		return err
	}
	logger := logging.New(service, cfg.Env)

	// A SIGINT/SIGTERM cancels this context, which unwinds every server and
	// bus consumer in an orderly way.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The seeder is a one-shot job: it drives the other services over HTTP and
	// exits, which is how the compose `orion-seed` container behaves.
	if service == config.ServiceSeed {
		return seed.Run(ctx, cfg, logger)
	}

	runtime, err := newRuntime(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer runtime.Close()

	servers, err := runtime.build(ctx, service)
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		return fmt.Errorf("unknown service %q", service)
	}

	var waitGroup sync.WaitGroup
	failures := make(chan error, len(servers))
	for _, server := range servers {
		waitGroup.Add(1)
		go func(srv *http.Server) {
			defer waitGroup.Done()
			logger.Info("listening", slog.String("addr", srv.Addr))
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				failures <- err
			}
		}(server)
	}

	// Seeding runs once the listeners are up, because the seeder talks to the
	// services over HTTP like any other client.
	if cfg.SeedOnBoot && service == config.ServiceAll {
		go func() {
			time.Sleep(300 * time.Millisecond)
			if err := seed.Run(ctx, cfg, logger); err != nil {
				logger.Warn("seeding failed", slog.String("err", err.Error()))
			}
		}()
	}

	select {
	case err := <-failures:
		return err
	case <-ctx.Done():
		logger.Info("shutdown requested")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, server := range servers {
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Warn("graceful shutdown failed", slog.String("addr", server.Addr), slog.String("err", err.Error()))
		}
	}
	waitGroup.Wait()
	return nil
}

// runtime owns the shared infrastructure of a process.
type runtime struct {
	cfg      config.Config
	logger   *slog.Logger
	pool     *pgxpool.Pool
	sessions cache.SessionStore
	bus      bus.Bus
	tokens   *security.Tokens
	closers  []func()
}

func newRuntime(ctx context.Context, cfg config.Config, logger *slog.Logger) (*runtime, error) {
	rt := &runtime{cfg: cfg, logger: logger, tokens: security.NewTokens(cfg.JWTSecret, cfg.JWTTTL)}

	if cfg.UsePostgres() {
		pool, err := db.Connect(ctx, cfg.PostgresURL, logger)
		if err != nil {
			return nil, err
		}
		if err := db.Migrate(ctx, pool, logger); err != nil {
			pool.Close()
			return nil, err
		}
		rt.pool = pool
		rt.closers = append(rt.closers, pool.Close)
	} else {
		logger.Warn("ORION_POSTGRES_URL is not set: running with in-memory repositories")
	}

	if cfg.UseRedis() {
		redis, err := cache.NewRedis(cfg.RedisURL)
		if err != nil {
			return nil, err
		}
		if err := redis.Ping(ctx); err != nil {
			// Redis only caches the resume context; the platform degrades to a
			// Call Management lookup instead of refusing to start (RNF007).
			logger.Warn("redis unreachable, using in-memory session store",
				slog.String("err", err.Error()))
			rt.sessions = cache.NewMemory()
		} else {
			rt.sessions = redis
			rt.closers = append(rt.closers, func() { _ = redis.Close() })
		}
	} else {
		rt.sessions = cache.NewMemory()
	}

	if cfg.UseKafka() {
		kafka := bus.NewKafka(cfg.KafkaBroker, logger)
		rt.bus = kafka
		rt.closers = append(rt.closers, func() { _ = kafka.Close() })
	} else {
		logger.Warn("ORION_KAFKA_BROKER is not set: using the in-process event bus")
		rt.bus = bus.NewInProcess(logger)
	}
	return rt, nil
}

func (r *runtime) Close() {
	for index := len(r.closers) - 1; index >= 0; index-- {
		r.closers[index]()
	}
}

// build returns the HTTP servers this process must run.
func (r *runtime) build(ctx context.Context, service string) ([]*http.Server, error) {
	all := service == config.ServiceAll
	servers := make([]*http.Server, 0, 5)

	if all || service == config.ServiceAuth {
		var repo authsvc.Repository = authsvc.NewMemoryRepository()
		if r.pool != nil {
			repo = authsvc.NewPostgresRepository(r.pool)
		}
		auth := authsvc.NewService(repo, r.tokens, r.cfg.BcryptCost, r.logger)
		servers = append(servers, r.server(r.cfg.AuthPort, auth.Routes()))
	}

	if all || service == config.ServiceCallMgmt {
		var repo callsvc.Repository = callsvc.NewMemoryRepository()
		if r.pool != nil {
			repo = callsvc.NewPostgresRepository(r.pool)
		}
		calls := callsvc.NewService(repo, r.bus, r.logger)
		servers = append(servers, r.server(r.cfg.CallMgmtPort, calls.Routes()))
	}

	if all || service == config.ServiceNLP {
		nlp := nlpsvc.NewService(
			nlpsvc.NewLLMClassifier(r.cfg.AnthropicAPIKey, r.cfg.AnthropicModel, r.cfg.NLUTimeout),
			r.logger,
		)
		servers = append(servers, r.server(r.cfg.NLPPort, nlp.Routes()))
	}

	if all || service == config.ServiceNotification {
		var repo notifysvc.Repository = notifysvc.NewMemoryRepository()
		if r.pool != nil {
			repo = notifysvc.NewPostgresRepository(r.pool)
		}
		notifications := notifysvc.NewService(repo, r.bus, r.logger)
		if err := notifications.Start(ctx); err != nil {
			return nil, fmt.Errorf("start notification consumer: %w", err)
		}
		servers = append(servers, r.server(r.cfg.NotificationPort, notifications.Routes()))
	}

	if all || service == config.ServiceGateway {
		gateway := gatewaysvc.NewService(r.cfg, r.tokens, r.sessions, r.bus, r.logger)
		if err := gateway.Start(ctx); err != nil {
			return nil, fmt.Errorf("start gateway consumer: %w", err)
		}
		servers = append(servers, r.server(r.cfg.GatewayPort, gateway.Routes()))
	}

	return servers, nil
}

// server applies the same timeouts and middleware to every internal service.
// The gateway brings its own middleware chain, so only the base timeouts and
// the request id are added here.
func (r *runtime) server(port int, handler http.Handler) *http.Server {
	wrapped := httpx.WithRequestID(httpx.Recover(r.logger)(handler))
	return &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: wrapped,
		// ReadHeaderTimeout guards against slow-header attacks; WriteTimeout is
		// left at zero because the gateway serves long-lived WebSockets.
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
