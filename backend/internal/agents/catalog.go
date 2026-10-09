package agents

import "github.com/orion-cx/orion-backend/internal/nlpsvc"

// Agent is a specialised assistant with its own persona and tool allowlist.
type Agent struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Persona string   `json:"-"`
	Tools   []string `json:"tools"`
}

// The agents of the platform. Each one only reaches the systems of its own
// domain: the financial agent cannot restart a modem, and the technical agent
// cannot change a plan. A compromised or confused agent can only do what its
// allowlist permits.
var (
	TechnicalAgent = Agent{
		ID:      "agente_suporte_tecnico",
		Name:    "Agente de Suporte Técnico",
		Persona: "Você é o Agente de Suporte Técnico da Claro. Resolve problemas de conexão, sinal e equipamento: primeiro diagnostica, depois propõe a ação.",
		Tools:   []string{ToolDiagnose, ToolRestartSignal, ToolListTickets, ToolHumanHandoff},
	}
	FinancialAgent = Agent{
		ID:      "agente_financeiro",
		Name:    "Agente Financeiro",
		Persona: "Você é o Agente Financeiro da Claro. Consulta faturas e gera segunda via. Contestação de valores é sempre de uma pessoa: transfira.",
		Tools:   []string{ToolInvoice, ToolInvoiceCopy, ToolHumanHandoff},
	}
	CommercialAgent = Agent{
		ID:      "agente_comercial",
		Name:    "Agente Comercial",
		Persona: "Você é o Agente Comercial da Claro. Consulta consumo e apresenta ofertas de upgrade adequadas; mudanças de plano exigem confirmação.",
		Tools:   []string{ToolUsage, ToolListOffers, ToolChangePlan, ToolHumanHandoff},
	}
	ServiceAgent = Agent{
		ID:      "agente_atendimento",
		Name:    "Agente de Atendimento",
		Persona: "Você é o Agente de Atendimento da Claro. Informa o andamento de chamados e orienta o cliente.",
		Tools:   []string{ToolListTickets, ToolHumanHandoff},
	}
)

// AllAgents lists the agents, used by the dashboard.
var AllAgents = []Agent{TechnicalAgent, FinancialAgent, CommercialAgent, ServiceAgent}

// agentForIntent routes an NLU intent to the agent that owns it. Intents that
// are not here are answered without tools (greeting, clarification) or never
// automated at all (billing disputes, cancellations).
var agentForIntent = map[string]Agent{
	nlpsvc.IntentTechnicalSupport: TechnicalAgent,
	nlpsvc.IntentInvoiceCopy:      FinancialAgent,
	nlpsvc.IntentDataUsage:        CommercialAgent,
	nlpsvc.IntentPlanUpgrade:      CommercialAgent,
	nlpsvc.IntentTicketStatus:     ServiceAgent,
}

// AgentFor returns the agent responsible for an intent.
func AgentFor(intent string) (Agent, bool) {
	agent, ok := agentForIntent[intent]
	return agent, ok
}

func (a Agent) allows(tool string) bool {
	for _, name := range a.Tools {
		if name == tool {
			return true
		}
	}
	return false
}

// AgentByName finds an agent by its display name.
func AgentByName(name string) (Agent, bool) {
	for _, agent := range AllAgents {
		if agent.Name == name {
			return agent, true
		}
	}
	return Agent{}, false
}
