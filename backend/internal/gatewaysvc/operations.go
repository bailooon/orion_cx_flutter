package gatewaysvc

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/orion-cx/orion-backend/internal/callsvc"
	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/nlpsvc"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
	"github.com/orion-cx/orion-backend/internal/platform/security"
)

// AssignToAgent hands a queued conversation to the agent making the request
// (flow B, step 5).
func (s *Service) AssignToAgent(ctx context.Context, principal security.Principal, conversationID string) (domain.Conversation, error) {
	conversation, err := s.calls.Assign(ctx, conversationID, principal.Name)
	if err != nil {
		return domain.Conversation{}, err
	}
	s.broadcastSnapshots(ctx)
	return conversation, nil
}

// AgentReply sends a manual answer back on the customer's channel.
func (s *Service) AgentReply(ctx context.Context, principal security.Principal, conversationID, text string) (domain.Conversation, error) {
	if strings.TrimSpace(text) == "" {
		return domain.Conversation{}, httpx.BadRequest("Escreva uma resposta antes de enviar.")
	}
	conversation, err := s.calls.AgentReply(ctx, conversationID, principal.Name, text)
	if err != nil {
		return domain.Conversation{}, err
	}
	s.broadcastSnapshots(ctx)
	return conversation, nil
}

// ResolveConversation closes an attendance.
func (s *Service) ResolveConversation(ctx context.Context, conversationID string) (domain.Conversation, error) {
	conversation, err := s.calls.Resolve(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}
	// The journey is over: drop the resume context so the next message starts
	// a fresh conversation instead of reopening a closed one.
	if err := s.sessions.Delete(ctx, conversation.UserID); err != nil {
		s.logger.Warn("failed to clear session context", slog.String("err", err.Error()))
	}
	s.broadcastSnapshots(ctx)
	return conversation, nil
}

// MarkConversationRead clears the unread badge of a queue entry.
func (s *Service) MarkConversationRead(ctx context.Context, conversationID string) (domain.Conversation, error) {
	conversation, err := s.calls.MarkRead(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}
	s.broadcastSnapshots(ctx)
	return conversation, nil
}

// ResetConversation clears a conversation. It exists so the demo can be run
// repeatedly without restarting the stack.
func (s *Service) ResetConversation(ctx context.Context, principal security.Principal, conversationID string) (domain.Conversation, error) {
	conversation, err := s.calls.Get(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}
	if conversation.UserID != principal.UserID && principal.Role != domain.RoleAgent {
		return domain.Conversation{}, httpx.ErrForbidden
	}
	reset, err := s.calls.Reset(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}
	if err := s.sessions.Delete(ctx, reset.UserID); err != nil {
		s.logger.Warn("failed to clear session context", slog.String("err", err.Error()))
	}
	greeted, err := s.calls.ApplyTurn(ctx, conversationID, turnGreeting(reset.LastChannel()))
	if err != nil {
		s.broadcastSnapshots(ctx)
		return reset, nil
	}
	s.saveSession(ctx, reset.UserID, greeted, domain.ChannelApp, "")
	s.broadcastSnapshots(ctx)
	return greeted, nil
}

// SetAlertVisible toggles the dashboard alert banner.
func (s *Service) SetAlertVisible(ctx context.Context, visible bool) {
	s.setAlertVisible(visible)
	s.broadcastSnapshots(ctx)
}

// Recommendations derives next-best-actions from the customer's own history
// (RF007). Nothing is random: every card states the fact that produced it.
func (s *Service) Recommendations(ctx context.Context, userID string) ([]domain.Recommendation, error) {
	conversations, err := s.calls.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	var (
		technicalCount int
		billingCount   int
		usageCount     int
		upgradeCount   int
		hasHandoff     bool
	)
	for _, conversation := range conversations {
		switch conversation.Intent {
		case nlpsvc.IntentTechnicalSupport:
			technicalCount++
		case nlpsvc.IntentBillingDispute, nlpsvc.IntentInvoiceCopy:
			billingCount++
		case nlpsvc.IntentDataUsage:
			usageCount++
		case nlpsvc.IntentPlanUpgrade:
			upgradeCount++
		}
		if conversation.Status == domain.StatusWaitingHuman || conversation.Status == domain.StatusInProgress {
			hasHandoff = true
		}
	}

	recommendations := make([]domain.Recommendation, 0, 4)
	if technicalCount >= 2 {
		recommendations = append(recommendations, domain.Recommendation{
			ID:     "REC-WIFI-MESH",
			Title:  "Wi-Fi Plus com repetidor incluso",
			Body:   "Melhora a cobertura em casas com mais de um andar e reduz quedas de sinal.",
			Reason: "Você abriu " + strconv.Itoa(technicalCount) + " atendimentos de conexão.",
			Action: "SIMULAR_UPGRADE_WIFI",
		})
	} else if technicalCount == 1 {
		recommendations = append(recommendations, domain.Recommendation{
			ID:     "REC-DIAG-AUTO",
			Title:  "Diagnóstico automático no app",
			Body:   "Ative o teste de conexão agendado e receba um alerta antes de perceber a lentidão.",
			Reason: "Seu último atendimento foi sobre a qualidade da conexão.",
			Action: "ATIVAR_DIAGNOSTICO",
		})
	}
	if billingCount > 0 {
		recommendations = append(recommendations, domain.Recommendation{
			ID:     "REC-DEBITO-AUTOMATICO",
			Title:  "Débito automático e fatura digital",
			Body:   "Evita atrasos e mantém o histórico de cobranças sempre disponível para consulta.",
			Reason: "Você já consultou ou contestou uma cobrança.",
			Action: "ATIVAR_DEBITO_AUTOMATICO",
		})
	}
	if usageCount > 0 || upgradeCount > 0 {
		recommendations = append(recommendations, domain.Recommendation{
			ID:     "REC-PLANO-MAIOR",
			Title:  "Claro Fibra 500 Mega + streaming",
			Body:   "Dobra a velocidade contratada pelo mesmo custo por mega do seu plano atual.",
			Reason: "Você demonstrou interesse em consumo ou upgrade de plano.",
			Action: "SIMULAR_UPGRADE_PLANO",
		})
	}
	if hasHandoff {
		recommendations = append(recommendations, domain.Recommendation{
			ID:     "REC-ACOMPANHAR-PROTOCOLO",
			Title:  "Acompanhe seu protocolo em tempo real",
			Body:   "A aba Chamados mostra cada atualização do seu atendimento, em qualquer canal.",
			Reason: "Você tem um atendimento em andamento com um especialista.",
			Action: "ABRIR_CHAMADOS",
		})
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, domain.Recommendation{
			ID:     "REC-BOAS-VINDAS",
			Title:  "Tudo certo com seus serviços",
			Body:   "Você pode falar com o Orion pelo App, pelo Web Portal ou pelo WhatsApp — o histórico é o mesmo.",
			Reason: "Nenhuma pendência encontrada no seu histórico.",
			Action: "EXPLORAR_CANAIS",
		})
	}
	return recommendations, nil
}

// ForgetMe fulfils an LGPD erasure request across every service that holds
// data about the customer.
func (s *Service) ForgetMe(ctx context.Context, principal security.Principal) error {
	if err := s.calls.PurgeUser(ctx, principal.UserID); err != nil {
		return err
	}
	if err := s.notifies.DeleteByUser(ctx, principal.UserID); err != nil {
		return err
	}
	if err := s.sessions.Delete(ctx, principal.UserID); err != nil {
		return err
	}
	if err := s.auth.Anonymize(ctx, principal.UserID); err != nil {
		return err
	}
	s.logger.Info("LGPD erasure completed", slog.String("user_id", principal.UserID))
	s.broadcastSnapshots(ctx)
	return nil
}

// Health reports the gateway and its dependencies, backing the container
// health checks (RNF002).
func (s *Service) Health(ctx context.Context) map[string]any {
	sessionStatus := "ok"
	if err := s.sessions.Ping(ctx); err != nil {
		sessionStatus = "degraded"
	}
	return map[string]any{
		"status":  "ok",
		"service": "orion-gateway",
		"dependencies": map[string]string{
			"auth":         healthCheck(ctx, s.auth.client),
			"nlp":          healthCheck(ctx, s.nlp.client),
			"callmgmt":     healthCheck(ctx, s.calls.client),
			"notification": healthCheck(ctx, s.notifies.client),
			"sessionStore": sessionStatus,
		},
		"websocketClients": s.hub.Clients(),
	}
}

// Ready is the readiness probe: it fails while a hard dependency is down, so
// an orchestrator does not route traffic to a broken instance.
func (s *Service) Ready(ctx context.Context) (map[string]any, bool) {
	report := s.Health(ctx)
	dependencies, _ := report["dependencies"].(map[string]string)
	ready := dependencies["auth"] == "ok" && dependencies["callmgmt"] == "ok"
	if !ready {
		report["status"] = "degraded"
	}
	return report, ready
}

// turnGreeting is the opening assistant message of a fresh conversation.
func turnGreeting(channel domain.Channel) callsvc.Turn {
	if !channel.Valid() {
		channel = domain.ChannelApp
	}
	return callsvc.Turn{
		Messages: []callsvc.MessageInput{{
			Actor:   domain.ActorAssistant,
			Text:    "Olá! Eu sou o assistente Orion. Como posso ajudar com sua internet ou sua fatura hoje?",
			Channel: channel,
		}},
	}
}
