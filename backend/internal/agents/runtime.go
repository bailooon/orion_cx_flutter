package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/nlpsvc"
	"github.com/orion-cx/orion-backend/internal/platform/queue"
	"github.com/orion-cx/orion-backend/internal/platform/security"
)

// Engines reported in an Outcome.
const (
	EngineClaude        = "claude-function-calling"
	EngineDeterministic = "plano-deterministico"
	EngineShed          = "fila-sobrecarregada"
)

// Execution statuses recorded for each tool call.
const (
	StatusOK                    = "ok"
	StatusError                 = "erro"
	StatusBlocked               = "bloqueada"
	StatusConfirmationRequested = "confirmacao_solicitada"
)

// MaxToolCallsPerTurn bounds how many tools an agent may call in one turn, so
// a looping or manipulated model cannot hammer the internal systems.
const MaxToolCallsPerTurn = 5

// Request is one customer turn handed to an agent.
type Request struct {
	Principal      security.Principal
	ConversationID string
	Intent         string
	Text           string
	Channel        domain.Channel
	History        []domain.Message
}

// Execution is the audit record of one tool call.
type Execution struct {
	Tool      string            `json:"tool"`
	Args      map[string]string `json:"args,omitempty"`
	Status    string            `json:"status"`
	Detail    string            `json:"detail,omitempty"`
	LatencyMs int64             `json:"latencyMs"`
}

// Outcome is what the gateway needs to finish the turn.
type Outcome struct {
	Agent  Agent
	Engine string
	Reply  string

	// Set when a sensitive tool is waiting for the customer's confirmation.
	PendingAction string
	PendingTool   string
	PendingArgs   map[string]string

	Resolves      bool
	Handoff       bool
	HandoffReason string
	TicketTitle   string

	Executions []Execution
	// ReplyCorrection is set when the verification step rewrote the model's
	// reply (personal data, leaked instructions, hallucinated action).
	ReplyCorrection string
	Shed            bool
}

// Runtime executes the agents.
type Runtime struct {
	toolbox *Toolbox
	limiter *queue.Limiter
	llm     *llmEngine
	logger  *slog.Logger
}

// NewRuntime wires the agents. llm may be nil: the agents then run their
// deterministic plans, which use the very same tools and safeguards.
func NewRuntime(toolbox *Toolbox, limiter *queue.Limiter, apiKey, model string, timeout time.Duration, logger *slog.Logger) *Runtime {
	return &Runtime{
		toolbox: toolbox,
		limiter: limiter,
		llm:     newLLMEngine(apiKey, model, timeout),
		logger:  logger,
	}
}

// QueueStats exposes the agent queue for the dashboard.
func (r *Runtime) QueueStats() queue.Stats { return r.limiter.Stats() }

// UsesLLM reports whether a model is configured.
func (r *Runtime) UsesLLM() bool { return r.llm != nil }

// Handle runs the agent that owns the request's intent. It goes through the
// queue: when the agents are saturated the request is shed and the caller
// answers without tools instead of making the customer wait.
func (r *Runtime) Handle(ctx context.Context, req Request) (Outcome, error) {
	agent, ok := AgentFor(req.Intent)
	if !ok {
		return Outcome{}, fmt.Errorf("nenhum agente para a intenção %s", req.Intent)
	}

	release, err := r.limiter.Acquire(ctx)
	if err != nil {
		if errors.Is(err, queue.ErrQueueFull) || errors.Is(err, queue.ErrWaitTimeout) {
			r.logger.Warn("agent queue shed a request", slog.String("agent", agent.ID), slog.String("reason", err.Error()))
			return Outcome{Agent: agent, Engine: EngineShed, Shed: true}, nil
		}
		return Outcome{}, err
	}
	defer release()

	if r.llm != nil {
		outcome, err := r.runLLM(ctx, agent, req)
		if err == nil {
			return outcome, nil
		}
		// The model failed: the deterministic plan answers instead, with the
		// same tools and the same confirmation rules (RNF007).
		r.logger.Warn("agent model failed, using deterministic plan",
			slog.String("agent", agent.ID), slog.String("err", err.Error()))
	}
	return r.runPlan(ctx, agent, req), nil
}

// invoke validates and runs one tool on behalf of an agent. Sensitive tools are
// never executed here: they turn into a confirmation request.
func (r *Runtime) invoke(ctx context.Context, agent Agent, call namedCall, outcome *Outcome) (Result, bool) {
	started := time.Now()
	record := func(tool, status, detail string) {
		outcome.Executions = append(outcome.Executions, Execution{
			Tool: tool, Args: call.Args, Status: status, Detail: detail,
			LatencyMs: time.Since(started).Milliseconds(),
		})
	}

	tool, ok := r.toolbox.Get(call.toolName)
	if !ok || !agent.allows(call.toolName) {
		record(call.toolName, StatusBlocked, "ferramenta fora da lista do agente")
		return Result{}, false
	}
	if len(outcome.Executions) >= MaxToolCallsPerTurn {
		record(tool.Name, StatusBlocked, "limite de chamadas por turno")
		return Result{}, false
	}
	if err := tool.ValidateArgs(call.Call); err != nil {
		record(tool.Name, StatusBlocked, err.Error())
		return Result{}, false
	}
	if tool.Sensitive {
		outcome.PendingAction = tool.PendingAction
		outcome.PendingTool = tool.Name
		outcome.PendingArgs = call.Args
		outcome.Reply = tool.ConfirmPrompt(call.Call)
		record(tool.Name, StatusConfirmationRequested, "aguardando confirmação do cliente")
		return Result{}, false
	}

	result, err := tool.Run(ctx, call.Call)
	if err != nil {
		record(tool.Name, StatusError, err.Error())
		return Result{}, false
	}
	record(tool.Name, StatusOK, "")
	if result.TicketTitle != "" && outcome.TicketTitle == "" {
		outcome.TicketTitle = result.TicketTitle
	}
	if result.Handoff {
		outcome.Handoff = true
		outcome.HandoffReason = result.HandoffReason
	}
	return result, true
}

type namedCall struct {
	Call
	toolName string
}

func (r *Runtime) newCall(req Request, tool string, args map[string]string) namedCall {
	if args == nil {
		args = map[string]string{}
	}
	return namedCall{Call: Call{Principal: req.Principal, ConversationID: req.ConversationID, Args: args}, toolName: tool}
}

// runPlan is the deterministic agent: a fixed, auditable sequence of tool
// calls per intent. It is the fallback when no model is configured, when the
// model fails, and a reference behaviour the model is compared against.
func (r *Runtime) runPlan(ctx context.Context, agent Agent, req Request) Outcome {
	outcome := Outcome{Agent: agent, Engine: EngineDeterministic}
	parts := make([]string, 0, 3)

	run := func(tool string, args map[string]string) (Result, bool) {
		result, ok := r.invoke(ctx, agent, r.newCall(req, tool, args), &outcome)
		if ok && result.Summary != "" {
			parts = append(parts, result.Summary)
		}
		if ok && result.Resolves {
			outcome.Resolves = true
		}
		return result, ok
	}

	switch req.Intent {
	case nlpsvc.IntentTechnicalSupport:
		run(ToolDiagnose, nil)
		r.invoke(ctx, agent, r.newCall(req, ToolRestartSignal, nil), &outcome)
	case nlpsvc.IntentInvoiceCopy:
		run(ToolInvoiceCopy, nil)
	case nlpsvc.IntentDataUsage:
		run(ToolUsage, nil)
	case nlpsvc.IntentPlanUpgrade:
		result, ok := run(ToolListOffers, nil)
		if ok {
			if offers, _ := result.Data["ofertas"].([]Offer); len(offers) > 0 {
				r.invoke(ctx, agent, r.newCall(req, ToolChangePlan, map[string]string{"oferta_id": offers[0].ID}), &outcome)
			}
		}
	case nlpsvc.IntentTicketStatus:
		run(ToolListTickets, nil)
	}

	// A pending confirmation question goes last, after what was found.
	if outcome.PendingTool != "" {
		parts = append(parts, outcome.Reply)
		outcome.Resolves = false
	}
	outcome.Reply = strings.Join(parts, " ")
	if outcome.Reply == "" {
		outcome.Reply = "Não consegui consultar os sistemas agora. Posso tentar de novo ou te transferir para um atendente."
	}
	return outcome
}

// ExecuteConfirmed runs a sensitive tool after the customer confirmed and the
// gateway re-verified the request. This is the only path that executes a
// sensitive tool.
func (r *Runtime) ExecuteConfirmed(ctx context.Context, principal security.Principal, conversationID, toolName string, args map[string]string) (Outcome, error) {
	tool, ok := r.toolbox.Get(toolName)
	if !ok || !tool.Sensitive {
		return Outcome{}, fmt.Errorf("ferramenta %q não pode ser confirmada", toolName)
	}
	agent := ownerOf(toolName)
	call := Call{Principal: principal, ConversationID: conversationID, Args: args}
	if call.Args == nil {
		call.Args = map[string]string{}
	}
	outcome := Outcome{Agent: agent, Engine: EngineDeterministic}
	// Arguments are validated again at execution time: an offer may have
	// expired between the question and the answer.
	if err := tool.ValidateArgs(call); err != nil {
		outcome.Executions = append(outcome.Executions, Execution{Tool: toolName, Args: args, Status: StatusBlocked, Detail: err.Error()})
		return outcome, err
	}
	started := time.Now()
	result, err := tool.Run(ctx, call)
	execution := Execution{Tool: toolName, Args: args, Status: StatusOK, LatencyMs: time.Since(started).Milliseconds()}
	if err != nil {
		execution.Status, execution.Detail = StatusError, err.Error()
		outcome.Executions = append(outcome.Executions, execution)
		return outcome, err
	}
	outcome.Executions = append(outcome.Executions, execution)
	outcome.Reply = result.Summary
	outcome.Resolves = result.Resolves
	outcome.TicketTitle = result.TicketTitle
	return outcome, nil
}

// ConfirmationPrompt rebuilds the question for a pending tool, used when the
// customer switches channel and the question has to be asked again.
func (r *Runtime) ConfirmationPrompt(principal security.Principal, toolName string, args map[string]string) string {
	tool, ok := r.toolbox.Get(toolName)
	if !ok || tool.ConfirmPrompt == nil {
		return ""
	}
	return tool.ConfirmPrompt(Call{Principal: principal, Args: args})
}

// PendingToolFor maps a stored pending-action code back to its tool, for
// conversations whose session context was lost.
func PendingToolFor(action string) string {
	switch action {
	case PendingRestart, domain.ActionContinue:
		return ToolRestartSignal
	case PendingPlanChange:
		return ToolChangePlan
	default:
		return ""
	}
}

// IsConfirmationAction reports whether a pending action waits for a yes/no.
func IsConfirmationAction(action string) bool {
	return action == PendingRestart || action == domain.ActionContinue || strings.HasPrefix(action, "CONFIRM_")
}

func ownerOf(toolName string) Agent {
	for _, agent := range AllAgents {
		if agent.allows(toolName) && toolName != ToolHumanHandoff {
			return agent
		}
	}
	return ServiceAgent
}

// toJSON renders tool data for the model.
func toJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `{"erro":"falha ao serializar resultado"}`
	}
	return string(encoded)
}
