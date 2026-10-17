package agents

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/security"
)

// Tool names. They are the function-calling contract shared with the model.
const (
	ToolDiagnose      = "diagnosticar_conexao"
	ToolRestartSignal = "reiniciar_sinal"
	ToolInvoice       = "consultar_fatura"
	ToolInvoiceCopy   = "gerar_segunda_via"
	ToolUsage         = "consultar_consumo"
	ToolListOffers    = "listar_ofertas_upgrade"
	ToolChangePlan    = "alterar_plano"
	ToolListTickets   = "consultar_chamados"
	ToolHumanHandoff  = "transferir_para_humano"
)

// Pending action codes stored on the conversation while a sensitive tool waits
// for the customer's confirmation. RESTART_SIGNAL predates the agents and is
// kept so existing clients and channel-switch handling keep working.
const (
	PendingRestart    = domain.ActionRestartSignal
	PendingPlanChange = "CONFIRM_PLAN_CHANGE"
)

// Call is one invocation of a tool.
type Call struct {
	// Principal is the authenticated customer. Tools act on Principal.UserID
	// only — no tool accepts a customer id as an argument, so neither the
	// model nor a prompt-injected message can point a tool at another account.
	Principal      security.Principal
	ConversationID string
	Args           map[string]string
}

// Result is what a tool returns: structured data for the model and a
// customer-facing sentence for the deterministic path.
type Result struct {
	Data          map[string]any
	Summary       string
	Resolves      bool
	Handoff       bool
	HandoffReason string
	TicketTitle   string
}

// Param describes one tool argument.
type Param struct {
	Description string
	Required    bool
	// Enum, when set, restricts the value. It is enforced server side, not
	// only advertised to the model.
	Enum func(call Call) []string
}

// Tool is a function the agents can call.
type Tool struct {
	Name        string
	Description string
	Params      map[string]Param
	// Sensitive tools change something for the customer. They never run on
	// the model's say-so: the platform asks the customer to confirm and
	// re-verifies the request before executing (double check).
	Sensitive     bool
	PendingAction string
	ConfirmPrompt func(call Call) string
	Run           func(ctx context.Context, call Call) (Result, error)
}

// TicketLister is the slice of Call Management the tools need.
type TicketLister interface {
	Tickets(ctx context.Context, userID string) ([]domain.Ticket, error)
}

// Toolbox holds every tool, keyed by name.
type Toolbox struct {
	tools map[string]*Tool
	now   func() time.Time
}

// NewToolbox builds the tools over the (simulated) Claro systems.
func NewToolbox(tickets TicketLister) *Toolbox {
	box := &Toolbox{tools: map[string]*Tool{}, now: time.Now}
	add := func(tool *Tool) { box.tools[tool.Name] = tool }

	add(&Tool{
		Name:        ToolDiagnose,
		Description: "Executa um diagnóstico remoto da conexão do cliente autenticado: velocidade medida, perda de pacotes, sinal óptico e recomendação.",
		Run: func(_ context.Context, call Call) (Result, error) {
			d := diagnose(call.Principal.UserID)
			return Result{
				Data: map[string]any{"diagnostico": d},
				Summary: fmt.Sprintf("Fiz um diagnóstico remoto da sua conexão: a velocidade medida foi de %d Mbps, de %d Mbps contratados, com %.1f%% de perda de pacotes no %s.",
					d.MeasuredMbps, d.ContractedMbps, d.PacketLossPercent, d.Equipment),
				TicketTitle: "Diagnóstico de conexão",
			}, nil
		},
	})

	add(&Tool{
		Name:          ToolRestartSignal,
		Description:   "Reinicia remotamente o sinal do equipamento do cliente. Ação sensível: o sistema pede confirmação ao cliente antes de executar.",
		Sensitive:     true,
		PendingAction: PendingRestart,
		ConfirmPrompt: func(Call) string {
			return "Posso reiniciar o sinal da sua conexão agora? A internet fica indisponível por cerca de 30 segundos."
		},
		Run: func(context.Context, Call) (Result, error) {
			return Result{
				Data:     map[string]any{"executado": true, "previsaoNormalizacao": "30 segundos"},
				Summary:  "Pronto! O sinal foi reiniciado. Aguarde cerca de 30 segundos e teste a conexão novamente.",
				Resolves: true,
			}, nil
		},
	})

	add(&Tool{
		Name:        ToolInvoice,
		Description: "Consulta a fatura atual do cliente autenticado: valor, vencimento, status e itens cobrados.",
		Run: func(_ context.Context, call Call) (Result, error) {
			inv := currentInvoice(call.Principal.UserID, box.now())
			items := make([]string, 0, len(inv.Items))
			for _, item := range inv.Items {
				items = append(items, fmt.Sprintf("%s (%s)", item.Description, brl(item.Amount)))
			}
			return Result{
				Data: map[string]any{"fatura": inv},
				Summary: fmt.Sprintf("Sua fatura de %s está %s: %s, com vencimento em %s. Itens: %s.",
					inv.Reference, inv.Status, brl(inv.Amount), inv.DueDate, strings.Join(items, "; ")),
			}, nil
		},
	})

	add(&Tool{
		Name:        ToolInvoiceCopy,
		Description: "Gera a segunda via da fatura atual e envia ao e-mail cadastrado do cliente autenticado.",
		Run: func(_ context.Context, call Call) (Result, error) {
			inv := currentInvoice(call.Principal.UserID, box.now())
			return Result{
				Data: map[string]any{"linhaDigitavel": barcodeFor(call.Principal.UserID), "enviadaPara": "e-mail cadastrado"},
				Summary: fmt.Sprintf("Gerei a segunda via da fatura de %s (%s, vencimento %s). A linha digitável começa com %s e o boleto completo foi enviado para o seu e-mail cadastrado.",
					inv.Reference, brl(inv.Amount), inv.DueDate, barcodeFor(call.Principal.UserID)),
				Resolves:    true,
				TicketTitle: "Segunda via de fatura",
			}, nil
		},
	})

	add(&Tool{
		Name:        ToolUsage,
		Description: "Consulta o consumo de dados do ciclo atual do cliente autenticado.",
		Run: func(_ context.Context, call Call) (Result, error) {
			u := currentUsage(call.Principal.UserID, box.now())
			return Result{
				Data: map[string]any{"consumo": u},
				Summary: fmt.Sprintf("Você já usou %.1f GB de %.0f GB (%d%% da franquia). O ciclo renova em %s.",
					u.UsedGB, u.AllowanceGB, u.Percent, u.RenewsOn),
				Resolves: true,
			}, nil
		},
	})

	add(&Tool{
		Name:        ToolListOffers,
		Description: "Lista as ofertas de upgrade de plano disponíveis para o cliente autenticado.",
		Run: func(_ context.Context, call Call) (Result, error) {
			offers := offersFor(call.Principal.UserID)
			names := make([]string, 0, len(offers))
			for _, offer := range offers {
				names = append(names, fmt.Sprintf("%s por %s/mês (%s)", offer.Name, brl(offer.MonthlyFee), offer.Benefit))
			}
			return Result{
				Data:    map[string]any{"ofertas": offers},
				Summary: "Encontrei estas opções para você: " + strings.Join(names, "; ") + ".",
			}, nil
		},
	})

	offerIDs := func(call Call) []string {
		offers := offersFor(call.Principal.UserID)
		ids := make([]string, 0, len(offers))
		for _, offer := range offers {
			ids = append(ids, offer.ID)
		}
		return ids
	}
	findOffer := func(call Call) (Offer, bool) {
		for _, offer := range offersFor(call.Principal.UserID) {
			if offer.ID == call.Args["oferta_id"] {
				return offer, true
			}
		}
		return Offer{}, false
	}

	add(&Tool{
		Name:        ToolChangePlan,
		Description: "Altera o plano do cliente autenticado para uma oferta da lista de upgrades. Ação sensível: o sistema pede confirmação ao cliente antes de executar.",
		Params: map[string]Param{
			"oferta_id": {Description: "Identificador da oferta retornado por listar_ofertas_upgrade.", Required: true, Enum: offerIDs},
		},
		Sensitive:     true,
		PendingAction: PendingPlanChange,
		ConfirmPrompt: func(call Call) string {
			offer, _ := findOffer(call)
			return fmt.Sprintf("Posso alterar o seu plano para %s por %s/mês (%s)? A mudança vale a partir do próximo ciclo. Confirma?",
				offer.Name, brl(offer.MonthlyFee), offer.Benefit)
		},
		Run: func(_ context.Context, call Call) (Result, error) {
			offer, ok := findOffer(call)
			if !ok {
				return Result{}, fmt.Errorf("oferta %q indisponível para este cliente", call.Args["oferta_id"])
			}
			return Result{
				Data: map[string]any{"planoNovo": offer, "vigencia": "próximo ciclo"},
				Summary: fmt.Sprintf("Pronto! Seu plano foi alterado para %s, valendo a partir do próximo ciclo. Você recebe a confirmação por e-mail.",
					offer.Name),
				Resolves:    true,
				TicketTitle: "Alteração de plano",
			}, nil
		},
	})

	add(&Tool{
		Name:        ToolListTickets,
		Description: "Lista os chamados (protocolos) do cliente autenticado e o status de cada um.",
		Run: func(ctx context.Context, call Call) (Result, error) {
			list, err := tickets.Tickets(ctx, call.Principal.UserID)
			if err != nil {
				return Result{}, err
			}
			if len(list) == 0 {
				return Result{Data: map[string]any{"chamados": []any{}}, Summary: "Você não tem chamados abertos no momento.", Resolves: true}, nil
			}
			labels := map[domain.TicketStatus]string{domain.TicketOpen: "aberto", domain.TicketInProgress: "em atendimento", domain.TicketResolved: "concluído"}
			lines := make([]string, 0, len(list))
			for i, ticket := range list {
				if i == 3 {
					break
				}
				lines = append(lines, fmt.Sprintf("%s — %s (%s)", ticket.ID, ticket.Title, labels[ticket.Status]))
			}
			return Result{
				Data:     map[string]any{"chamados": list},
				Summary:  "Seus chamados mais recentes: " + strings.Join(lines, "; ") + ". O detalhe de cada um está na aba Chamados.",
				Resolves: true,
			}, nil
		},
	})

	add(&Tool{
		Name:        ToolHumanHandoff,
		Description: "Transfere o atendimento para uma pessoa atendente, com o histórico. Use quando o pedido fugir das suas ferramentas, envolver contestação de valores ou quando o cliente pedir.",
		Params: map[string]Param{
			"motivo": {Description: "Motivo da transferência, em uma frase.", Required: true},
		},
		Run: func(_ context.Context, call Call) (Result, error) {
			return Result{
				Handoff:       true,
				HandoffReason: "Agente de IA solicitou transbordo: " + call.Args["motivo"],
				Summary:       "Vou transferir você para uma pessoa especialista, levando todo o histórico desta conversa.",
			}, nil
		},
	})

	return box
}

// Get returns a tool by name.
func (b *Toolbox) Get(name string) (*Tool, bool) {
	tool, ok := b.tools[name]
	return tool, ok
}

// ValidateArgs enforces the tool's argument contract server side: unknown
// arguments, missing required ones and values outside the allowed set are all
// rejected, whatever the model produced.
func (t *Tool) ValidateArgs(call Call) error {
	for key := range call.Args {
		if _, ok := t.Params[key]; !ok {
			return fmt.Errorf("argumento não permitido para %s: %s", t.Name, key)
		}
	}
	for name, param := range t.Params {
		value := strings.TrimSpace(call.Args[name])
		if param.Required && value == "" {
			return fmt.Errorf("argumento obrigatório ausente em %s: %s", t.Name, name)
		}
		if len(value) > 200 {
			return fmt.Errorf("argumento %s excede o tamanho permitido", name)
		}
		if param.Enum != nil && value != "" {
			allowed := param.Enum(call)
			found := false
			for _, option := range allowed {
				if option == value {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("valor não permitido para %s: %s", name, value)
			}
		}
	}
	return nil
}

// sortedParamNames keeps the generated JSON Schema stable between calls,
// which keeps the model's prompt cache warm.
func (t *Tool) sortedParamNames() []string {
	names := make([]string, 0, len(t.Params))
	for name := range t.Params {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
