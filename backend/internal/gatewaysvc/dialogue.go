package gatewaysvc

import (
	"strconv"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/nlpsvc"
)

// alwaysHuman lists the intents that never run automatically, whatever
// confidence the model reports. These are decisions taken over a customer's
// money or contract, where a wrong automated action is expensive to undo. The
// confidence threshold and this list are two independent guards (RF005).
var alwaysHuman = map[string]bool{
	nlpsvc.IntentBillingDispute: true,
	nlpsvc.IntentCancellation:   true,
}

// route is the decision the gateway takes for one classified message.
type route struct {
	// handoff means the conversation goes to the human queue.
	handoff bool
	// reason explains the handoff to the customer and to the dashboard.
	reason string
	// reply is what the assistant answers.
	reply string
	// pendingAction is stored on the conversation and in the session context.
	pendingAction string
	// resolves marks the conversation as finished.
	resolves bool
	// ticketTitle, when set, opens a protocol for this interaction.
	ticketTitle string
	ticketKind  string
}

// decide maps an NLU result plus the current state to a routing decision.
func decide(result nlpsvc.Result, threshold float64, pendingAction string, unclearTurns int) route {
	hasPending := pendingAction == domain.ActionRestartSignal || pendingAction == domain.ActionContinue

	// 1. Answers to a pending question are handled first: they are about the
	//    action already on the table, not a new intent.
	if hasPending {
		switch result.Intent {
		case nlpsvc.IntentAffirmative:
			return route{
				reply:    "Pronto! O sinal foi reiniciado. Aguarde cerca de 30 segundos e teste a conexão novamente.",
				resolves: true,
			}
		case nlpsvc.IntentNegative:
			return route{
				reply: "Sem problema, não fiz nenhuma alteração. Posso tentar outro diagnóstico " +
					"ou encaminhar você para um atendente.",
			}
		}
	}

	// 2. Sensitive intents and low confidence both go to a human.
	if alwaysHuman[result.Intent] {
		return route{
			handoff: true,
			reason: "Intenção " + result.Intent + " exige análise humana por envolver cobrança ou contrato " +
				"(confiança da IA: " + formatPercent(result.Confidence) + ").",
			reply: "Vou transferir este atendimento para uma pessoa especialista. Seu histórico já está " +
				"sendo encaminhado, então você não precisará repetir as informações.",
			ticketTitle: ticketTitleFor(result.Intent),
			ticketKind:  result.Intent,
		}
	}

	if result.Intent == nlpsvc.IntentUnknown || result.Confidence < threshold {
		// First unclear message: ask for detail instead of queueing. Sending a
		// customer to a human because one sentence was ambiguous is a worse
		// experience than one clarifying question.
		if unclearTurns == 0 {
			return route{
				reply: "Quero te ajudar da melhor forma. Pode me contar um pouco mais? " +
					"Consigo resolver questões de internet, fatura, consumo do plano e chamados abertos.",
			}
		}
		return route{
			handoff: true,
			reason: "Confiança da IA abaixo do limite de automação (" +
				formatPercent(result.Confidence) + " < " + formatPercent(threshold) + ").",
			reply: "Prefiro não arriscar uma resposta errada. Vou te transferir para um atendente, " +
				"levando todo o histórico desta conversa.",
			ticketTitle: "Atendimento encaminhado para especialista",
			ticketKind:  result.Intent,
		}
	}

	// 3. Confident, automatable intents.
	switch result.Intent {
	case nlpsvc.IntentTechnicalSupport:
		return route{
			reply:         "Entendi que sua conexão está com problema. Posso reiniciar o sinal da sua conexão agora?",
			pendingAction: domain.ActionRestartSignal,
			ticketTitle:   "Diagnóstico de conexão",
			ticketKind:    result.Intent,
		}
	case nlpsvc.IntentInvoiceCopy:
		return route{
			reply: "Já gerei a segunda via da sua fatura. Ela está disponível no app e também foi " +
				"enviada para o e-mail cadastrado.",
			resolves:    true,
			ticketTitle: "Segunda via de fatura",
			ticketKind:  result.Intent,
		}
	case nlpsvc.IntentDataUsage:
		return route{
			reply: "Consultei seu plano: você utilizou 63% da franquia neste ciclo, com renovação " +
				"no dia 28. Quer que eu ative o aviso de consumo em 80%?",
			resolves: true,
		}
	case nlpsvc.IntentPlanUpgrade:
		return route{
			reply: "Posso te mostrar as opções de upgrade disponíveis para o seu endereço. " +
				"Separei as ofertas na aba de recomendações do seu portal.",
			resolves: true,
		}
	case nlpsvc.IntentTicketStatus:
		return route{
			reply: "Seus chamados e protocolos estão na aba Chamados do portal, com o histórico " +
				"completo e o status atualizado em tempo real.",
			resolves: true,
		}
	case nlpsvc.IntentGreeting:
		return route{
			reply: "Olá! Eu sou o assistente Orion. Posso ajudar com internet, fatura, consumo do " +
				"plano ou o andamento dos seus chamados. O que você precisa hoje?",
		}
	default:
		return route{
			reply: "Entendi seu pedido e registrei a solicitação. Um especialista pode dar " +
				"continuidade se você preferir.",
		}
	}
}

func ticketTitleFor(intent string) string {
	switch intent {
	case nlpsvc.IntentBillingDispute:
		return "Contestação de cobrança"
	case nlpsvc.IntentCancellation:
		return "Solicitação de cancelamento"
	case nlpsvc.IntentTechnicalSupport:
		return "Diagnóstico de conexão"
	default:
		return "Atendimento ao cliente"
	}
}

// formatPercent renders a 0..1 score as an integer percentage.
func formatPercent(value float64) string {
	return strconv.Itoa(int(value*100+0.5)) + "%"
}
