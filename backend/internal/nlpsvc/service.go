package nlpsvc

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/orion-cx/orion-backend/internal/platform/httpx"
	"github.com/orion-cx/orion-backend/internal/platform/queue"
)

// Service is the ORION Motor NLP/IA. It prefers the model-backed engine and
// degrades to the rule engine on any failure, so classification never becomes
// a single point of failure (RNF007).
type Service struct {
	llm     *LLMClassifier
	rules   *RuleClassifier
	limiter *queue.Limiter
	logger  *slog.Logger
}

// NewService wires the NLU engines. llm may be nil, in which case the service
// runs rules only and says so in every response.
func NewService(llm *LLMClassifier, logger *slog.Logger) *Service {
	return NewServiceWithQueue(llm, queue.New("nlu", 8, 32, 2*time.Second), logger)
}

// NewServiceWithQueue wires the NLU with an explicit queue in front of the
// model. When the queue is saturated the message is classified by the rule
// engine instead of waiting: the customer gets an answer, just a simpler one.
func NewServiceWithQueue(llm *LLMClassifier, limiter *queue.Limiter, logger *slog.Logger) *Service {
	if llm == nil {
		logger.Warn("no ANTHROPIC_API_KEY configured: NLU running on the rule engine only")
	}
	return &Service{llm: llm, rules: NewRuleClassifier(), limiter: limiter, logger: logger}
}

// Request is the classification input.
type Request struct {
	Text string `json:"text"`
	// HasPendingQuestion tells the engines that a bare "sim" or "não" is an
	// answer to the assistant, not a new intent.
	HasPendingQuestion bool `json:"hasPendingQuestion"`
}

// Classify labels the message, falling back to rules when the model fails.
func (s *Service) Classify(ctx context.Context, request Request) Result {
	text := strings.TrimSpace(request.Text)
	if text == "" {
		return Result{
			Intent:     IntentUnknown,
			Confidence: 0,
			Summary:    "Mensagem vazia recebida.",
			Engine:     "rules",
		}
	}

	var release func()
	if s.llm != nil {
		var err error
		release, err = s.limiter.Acquire(ctx)
		if err != nil {
			s.logger.Warn("nlu queue saturated, using rule engine", slog.String("err", err.Error()))
		}
	}
	if s.llm != nil && release != nil {
		started := time.Now()
		result, err := s.llm.Classify(ctx, text, request.HasPendingQuestion)
		release()
		if err == nil {
			s.logger.Info("intent classified",
				slog.String("engine", result.Engine),
				slog.String("intent", result.Intent),
				slog.Float64("confidence", result.Confidence),
				slog.Int64("duration_ms", time.Since(started).Milliseconds()),
			)
			return result
		}
		// The message itself is never logged: it is customer content (LGPD).
		s.logger.Warn("model classification failed, using rule engine",
			slog.String("err", err.Error()),
			slog.Int64("duration_ms", time.Since(started).Milliseconds()),
		)
	}

	result := s.rules.Classify(text, request.HasPendingQuestion)
	s.logger.Info("intent classified",
		slog.String("engine", result.Engine),
		slog.String("intent", result.Intent),
		slog.Float64("confidence", result.Confidence),
	)
	return result
}

// Routes mounts the internal API of the NLP service.
func (s *Service) Routes() http.Handler {
	router := chi.NewRouter()

	router.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		engine := "rules"
		if s.llm != nil {
			engine = "claude+rules"
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "ok", "service": "orion-nlp", "engine": engine,
		})
	})

	router.Get("/v1/stats", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, s.limiter.Stats())
	})

	router.Post("/v1/classify", func(w http.ResponseWriter, r *http.Request) {
		var request Request
		if err := httpx.DecodeJSON(r, &request); err != nil {
			httpx.WriteError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, s.Classify(r.Context(), request))
	})

	return router
}
