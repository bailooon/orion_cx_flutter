// Package seed populates the demo dataset.
//
// It talks to the Authenticator and Call Management services over their public
// contracts rather than writing to the database directly, so seeding exercises
// the same code path the application uses and works identically against the
// in-memory and PostgreSQL backends.
package seed

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/orion-cx/orion-backend/internal/authsvc"
	"github.com/orion-cx/orion-backend/internal/callsvc"
	"github.com/orion-cx/orion-backend/internal/config"
	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
)

// DemoPassword is the shared password of every seeded account. It exists only
// so the two demo flows can be run without a sign-up step, and is printed in
// the README rather than hidden in the code.
const DemoPassword = "orion12345"

// account describes one seeded user.
type account struct {
	email    string
	name     string
	document string
	plan     string
	role     domain.Role
}

var accounts = []account{
	{email: "cliente@orion.dev", name: "Cliente Demo", document: "***.482.***-**", plan: "Claro Pós 50 GB + Fibra", role: domain.RoleCustomer},
	{email: "atendente@orion.dev", name: "Camila Rocha", document: "***.118.***-**", plan: "Equipe de atendimento", role: domain.RoleAgent},
	{email: "maria@orion.dev", name: "Maria Ferreira", document: "***.207.***-**", plan: "Claro Controle 25 GB", role: domain.RoleCustomer},
	{email: "joao@orion.dev", name: "João Martins", document: "***.915.***-**", plan: "Claro Fibra 500 Mega", role: domain.RoleCustomer},
	{email: "paula@orion.dev", name: "Paula Santos", document: "***.031.***-**", plan: "Claro Fibra 350 Mega", role: domain.RoleCustomer},
	{email: "carlos@orion.dev", name: "Carlos Almeida", document: "***.664.***-**", plan: "Claro Pós 30 GB", role: domain.RoleCustomer},
	{email: "beatriz@orion.dev", name: "Beatriz Lima", document: "***.390.***-**", plan: "Claro Fibra 500 Mega", role: domain.RoleCustomer},
}

// Run creates the demo accounts and the conversations the dashboard needs in
// order to look alive on first load. It is idempotent: running it twice leaves
// the dataset unchanged.
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	authClient := httpx.NewClient(cfg.AuthURL, 10*time.Second, 3, logger)
	callClient := httpx.NewClient(cfg.CallMgmtURL, 10*time.Second, 3, logger)

	users := make(map[string]domain.User, len(accounts))
	for _, entry := range accounts {
		user, err := ensureAccount(ctx, authClient, entry)
		if err != nil {
			return err
		}
		users[entry.email] = user
	}

	// Conversations are only seeded on an empty database, so a demo run that
	// resolved a case is not silently resurrected on the next restart.
	var existing []domain.Conversation
	if err := callClient.Do(ctx, "GET", "/v1/conversations", nil, &existing); err != nil {
		return err
	}
	if len(existing) > 0 {
		logger.Info("demo data already present", slog.Int("conversations", len(existing)))
		return nil
	}

	if err := seedWaitingCase(ctx, callClient, users["maria@orion.dev"]); err != nil {
		return err
	}
	if err := seedInProgressCase(ctx, callClient, users["joao@orion.dev"]); err != nil {
		return err
	}
	if err := seedResolvedCase(ctx, callClient, users["paula@orion.dev"]); err != nil {
		return err
	}
	if err := seedRatedCases(ctx, callClient, users["carlos@orion.dev"], users["beatriz@orion.dev"]); err != nil {
		return err
	}

	logger.Info("demo data seeded",
		slog.Int("users", len(users)),
		slog.String("customer_login", "cliente@orion.dev"),
		slog.String("agent_login", "atendente@orion.dev"),
	)
	return nil
}

// ensureAccount registers a user, or logs in when the e-mail already exists.
func ensureAccount(ctx context.Context, client *httpx.Client, entry account) (domain.User, error) {
	var session authsvc.Session
	err := client.Do(ctx, "POST", "/v1/register", authsvc.RegisterInput{
		Email:        entry.email,
		Password:     DemoPassword,
		Name:         entry.name,
		DocumentMask: entry.document,
		PlanName:     entry.plan,
		Role:         entry.role,
	}, &session)
	if err == nil {
		return session.User, nil
	}

	var apiErr httpx.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict {
		return domain.User{}, err
	}
	// Already seeded: recover the id through a login.
	if err := client.Do(ctx, "POST", "/v1/login",
		map[string]string{"email": entry.email, "password": DemoPassword}, &session); err != nil {
		return domain.User{}, err
	}
	return session.User, nil
}

func openConversation(ctx context.Context, client *httpx.Client, id string, user domain.User) (domain.Conversation, error) {
	var conversation domain.Conversation
	err := client.Do(ctx, "POST", "/v1/conversations", callsvc.NewConversationInput{
		ID:               id,
		UserID:           user.ID,
		CustomerName:     user.Name,
		CustomerDocument: user.DocumentMask,
		PlanName:         user.PlanName,
	}, &conversation)
	return conversation, err
}

func applyTurn(ctx context.Context, client *httpx.Client, id string, turn callsvc.Turn) error {
	return client.Do(ctx, "POST", "/v1/conversations/"+id+"/turns", turn, nil)
}

// seedWaitingCase is the queue entry an agent sees the moment they open the
// dashboard, so flow B can be demonstrated from either end.
func seedWaitingCase(ctx context.Context, client *httpx.Client, user domain.User) error {
	const id = "CX-2026-0139"
	if _, err := openConversation(ctx, client, id, user); err != nil {
		return err
	}
	intent := "CONTESTACAO_FATURA"
	confidence := 0.45
	summary := "Cliente relata cobrança indevida na última fatura. A IA classificou a intenção com " +
		"45% de confiança, abaixo do limite de automação, e solicitou transbordo humano."
	status := domain.StatusWaitingHuman
	unread := true

	if err := applyTurn(ctx, client, id, callsvc.Turn{
		Messages: []callsvc.MessageInput{
			{Actor: domain.ActorCustomer, Text: "Quero reclamar de uma cobrança indevida na minha fatura.", Channel: domain.ChannelApp},
			{Actor: domain.ActorAssistant, Text: "Vou transferir este atendimento para uma pessoa especialista em faturas. Seu histórico será mantido.", Channel: domain.ChannelApp},
			{Actor: domain.ActorSystem, Text: "Evento REQUIRED_HUMAN_ASSISTANCE publicado. Intenção CONTESTACAO_FATURA exige análise humana.", Channel: domain.ChannelApp},
		},
		Intent:           &intent,
		IntentConfidence: &confidence,
		Summary:          &summary,
		Status:           &status,
		HasUnreadEvent:   &unread,
		SetPendingAction: true,
		PendingAction:    domain.ActionHumanHandoff,
	}); err != nil {
		return err
	}

	return client.Do(ctx, "POST", "/v1/tickets", callsvc.TicketInput{
		UserID:         user.ID,
		ConversationID: id,
		Title:          "Contestação de cobrança",
		Category:       "CONTESTACAO_FATURA",
		Status:         domain.TicketOpen,
		Channel:        domain.ChannelApp,
		FirstEvent:     "Chamado aberto pelo canal App Claro.",
	}, nil)
}

// seedInProgressCase shows the dashboard with an attendance already assigned.
func seedInProgressCase(ctx context.Context, client *httpx.Client, user domain.User) error {
	const id = "CX-2026-0136"
	if _, err := openConversation(ctx, client, id, user); err != nil {
		return err
	}
	intent := "CONTESTACAO_FATURA"
	confidence := 0.49
	summary := "Cliente questiona um serviço adicional na fatura. Atendimento assumido e histórico contextualizado."
	status := domain.StatusInProgress
	agent := "Camila Rocha"
	unread := false

	return applyTurn(ctx, client, id, callsvc.Turn{
		Messages: []callsvc.MessageInput{
			{Actor: domain.ActorCustomer, Text: "Apareceu um serviço adicional que eu não contratei.", Channel: domain.ChannelWeb},
			{Actor: domain.ActorAssistant, Text: "Vou encaminhar o caso para um atendente e manter todo o histórico.", Channel: domain.ChannelWeb},
			{Actor: domain.ActorSystem, Text: "Camila Rocha assumiu o atendimento.", Channel: domain.ChannelWeb},
			{Actor: domain.ActorAgent, Text: "Olá, João. Estou verificando a origem desse serviço adicional para você.", Channel: domain.ChannelWeb},
		},
		Intent:           &intent,
		IntentConfidence: &confidence,
		Summary:          &summary,
		Status:           &status,
		AssignedAgent:    &agent,
		HasUnreadEvent:   &unread,
	})
}

// seedResolvedCase gives the history screen something to show, including a
// conversation that crossed channels.
func seedResolvedCase(ctx context.Context, client *httpx.Client, user domain.User) error {
	const id = "CX-2026-0128"
	if _, err := openConversation(ctx, client, id, user); err != nil {
		return err
	}
	intent := "SUPORTE_TECNICO"
	confidence := 0.96
	summary := "Cliente autorizou o reinício do sinal pelo WhatsApp e confirmou o restabelecimento da conexão pelo Web Portal."
	status := domain.StatusResolved

	if err := applyTurn(ctx, client, id, callsvc.Turn{
		Messages: []callsvc.MessageInput{
			{Actor: domain.ActorCustomer, Text: "Minha internet está lenta.", Channel: domain.ChannelWhatsApp},
			{Actor: domain.ActorAssistant, Text: "Posso reiniciar o sinal da sua conexão agora?", Channel: domain.ChannelWhatsApp},
			{Actor: domain.ActorSystem, Text: "Sessão CX-2026-0128 recuperada de WhatsApp em Web Portal.", Channel: domain.ChannelWeb},
			{Actor: domain.ActorCustomer, Text: "Sim, pode reiniciar.", Channel: domain.ChannelWeb},
			{Actor: domain.ActorAssistant, Text: "O sinal foi reiniciado. Aguarde alguns segundos e teste novamente.", Channel: domain.ChannelWeb},
			{Actor: domain.ActorCustomer, Text: "Voltou ao normal, obrigado!", Channel: domain.ChannelWeb},
		},
		Intent:           &intent,
		IntentConfidence: &confidence,
		Summary:          &summary,
		Status:           &status,
	}); err != nil {
		return err
	}

	return client.Do(ctx, "POST", "/v1/tickets", callsvc.TicketInput{
		UserID:         user.ID,
		ConversationID: id,
		Title:          "Diagnóstico de conexão",
		Category:       "SUPORTE_TECNICO",
		Status:         domain.TicketResolved,
		Channel:        domain.ChannelWhatsApp,
		FirstEvent:     "Chamado aberto pelo canal WhatsApp.",
	}, nil)
}

// seedRatedCases gives the satisfaction dashboard something to show on first
// load: answered surveys for one AI-resolved and one human-resolved case,
// plus the survey of the resolved case above. All of it is demo data.
func seedRatedCases(ctx context.Context, client *httpx.Client, carlos, beatriz domain.User) error {
	rate := func(id string, score int, comment string) error {
		return client.Do(ctx, "POST", "/v1/conversations/"+id+"/survey",
			map[string]any{"score": score, "comment": comment}, nil)
	}
	if err := rate("CX-2026-0128", 5, "Resolveu sem eu precisar repetir nada quando troquei de canal."); err != nil {
		return err
	}

	// Human-resolved billing question, rated 3.
	const humanID = "CX-2026-0121"
	if _, err := openConversation(ctx, client, humanID, carlos); err != nil {
		return err
	}
	intent, confidence := "CONTESTACAO_FATURA", 0.47
	summary := "Cliente questionou cobrança de roaming. Atendente estornou o valor."
	waiting := domain.StatusWaitingHuman
	if err := applyTurn(ctx, client, humanID, callsvc.Turn{
		Messages: []callsvc.MessageInput{
			{Actor: domain.ActorCustomer, Text: "Veio uma cobrança de roaming que eu não usei.", Channel: domain.ChannelApp},
			{Actor: domain.ActorAssistant, Text: "Vou transferir para uma pessoa especialista em faturas.", Channel: domain.ChannelApp},
		},
		Intent: &intent, IntentConfidence: &confidence, Summary: &summary, Status: &waiting,
	}); err != nil {
		return err
	}
	if err := client.Do(ctx, "POST", "/v1/conversations/"+humanID+"/assign", map[string]string{"agentName": "Camila Rocha"}, nil); err != nil {
		return err
	}
	if err := client.Do(ctx, "POST", "/v1/conversations/"+humanID+"/agent-messages",
		map[string]string{"agentName": "Camila Rocha", "text": "Estornei o valor do roaming na próxima fatura."}, nil); err != nil {
		return err
	}
	if err := client.Do(ctx, "POST", "/v1/conversations/"+humanID+"/resolve", nil, nil); err != nil {
		return err
	}
	if err := rate(humanID, 3, "Resolveu, mas demorou para alguém responder."); err != nil {
		return err
	}

	// AI-resolved usage question, rated 4.
	const aiID = "CX-2026-0124"
	if _, err := openConversation(ctx, client, aiID, beatriz); err != nil {
		return err
	}
	usageIntent, usageConfidence := "CONSULTA_CONSUMO", 0.88
	usageSummary := "Cliente consultou o consumo do plano pelo WhatsApp."
	resolved := domain.StatusResolved
	if err := applyTurn(ctx, client, aiID, callsvc.Turn{
		Messages: []callsvc.MessageInput{
			{Actor: domain.ActorCustomer, Text: "Quanto ainda tenho de franquia?", Channel: domain.ChannelWhatsApp},
			{Actor: domain.ActorAssistant, Text: "Você já usou 31,5 GB de 50 GB (63% da franquia). O ciclo renova dia 28.", Channel: domain.ChannelWhatsApp},
		},
		Intent: &usageIntent, IntentConfidence: &usageConfidence, Summary: &usageSummary, Status: &resolved,
	}); err != nil {
		return err
	}
	return rate(aiID, 4, "")
}
