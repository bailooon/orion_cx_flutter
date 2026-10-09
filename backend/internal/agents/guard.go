package agents

import (
	"strings"
	"unicode"

	"github.com/orion-cx/orion-backend/internal/nlpsvc"
	"github.com/orion-cx/orion-backend/internal/platform/logging"
)

// MaxInputRunes bounds a customer message. Longer texts are rejected: they are
// the usual carrier of prompt-injection payloads and no legitimate support
// request needs them.
const MaxInputRunes = 1000

// Verdict is the outcome of inspecting a customer message before it reaches
// any model or tool.
type Verdict struct {
	Allowed bool
	// Reason is a short machine-readable code recorded in the audit trail.
	Reason string
	// Text is the sanitised message to use when Allowed.
	Text string
}

// injectionMarkers are phrasings that try to override the assistant's
// instructions, extract them, impersonate staff, or reach another customer's
// data. They are matched on the normalised (lowercase, accent-free) text.
var injectionMarkers = []string{
	"ignore as instrucoes", "ignore suas instrucoes", "ignore todas as", "ignore as regras",
	"ignore previous", "ignore all previous", "disregard previous", "esqueca suas instrucoes",
	"esqueca as instrucoes", "desconsidere as instrucoes", "novas instrucoes:",
	"system prompt", "prompt do sistema", "prompt de sistema", "suas instrucoes internas",
	"revele suas instrucoes", "mostre suas instrucoes", "repita suas instrucoes",
	"voce agora e", "a partir de agora voce", "you are now", "act as", "aja como se",
	"finja que voce", "modo desenvolvedor", "developer mode", "jailbreak", "modo dan",
	"<system>", "</system>", "[system]", "<tool", "</mensagem_do_cliente", "assistant:", "system:",
	"sou o atendente", "sou atendente da claro", "sou administrador", "sou o administrador",
	"acesso de administrador", "outro cliente", "de outra pessoa", "dados do cliente ",
	"cpf de outra", "conta de outra", "chame a ferramenta", "execute a ferramenta",
	"use a ferramenta", "call the tool",
}

// InspectInput sanitises a message and blocks prompt-injection attempts.
func InspectInput(text string) Verdict {
	cleaned := stripControl(text)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return Verdict{Allowed: false, Reason: "mensagem_vazia"}
	}
	if len([]rune(cleaned)) > MaxInputRunes {
		return Verdict{Allowed: false, Reason: "mensagem_longa_demais"}
	}

	normalized := nlpsvc.Normalize(cleaned)
	for _, marker := range injectionMarkers {
		if strings.Contains(normalized, marker) {
			return Verdict{Allowed: false, Reason: "tentativa_de_injecao_de_prompt"}
		}
	}
	return Verdict{Allowed: true, Text: cleaned}
}

// stripControl removes control characters (except newline and tab), which
// can hide instructions from a human reviewer while a model still reads them.
func stripControl(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	for _, r := range text {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			if unicode.In(r, unicode.Cf) { // zero-width and bidi overrides
				continue
			}
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// BlockedReply is what the customer sees when a message is refused. It does
// not explain which rule fired, so the guard cannot be probed.
const BlockedReply = "Não consigo seguir esse tipo de instrução. Posso ajudar com sua internet, fatura, consumo, plano ou chamados — me conte o que você precisa."

// ReplyCheck is the outcome of verifying a model-written reply before it is
// sent to the customer (the second half of the double check).
type ReplyCheck struct {
	Text      string
	Corrected bool
	Reason    string
}

// promptLeakMarkers are fragments of the system prompt that must never appear
// in a reply.
var promptLeakMarkers = []string{"regras de seguranca", "<mensagem_do_cliente", "instrucoes do sistema", "ferramentas disponiveis:"}

// actionClaims maps a sensitive tool to the phrasings that would tell the
// customer it ran. A reply that makes the claim without the tool having run
// is a hallucinated action and is rewritten.
var actionClaims = map[string][]string{
	ToolRestartSignal: {"sinal foi reiniciado", "reiniciei o sinal", "reiniciei seu", "reiniciei o seu", "reinicio foi feito", "reiniciado com sucesso"},
	ToolChangePlan:    {"plano foi alterado", "alterei seu plano", "alterei o seu plano", "mudei seu plano", "upgrade foi realizado", "plano ja foi alterado"},
}

// VerifyReply double-checks a model-written reply: it redacts personal data,
// blocks system-prompt leaks and rejects claims of actions that did not run.
// fallback is the deterministic text used when the reply cannot be trusted.
func VerifyReply(reply string, executed map[string]bool, fallback string) ReplyCheck {
	text := strings.TrimSpace(reply)
	if text == "" {
		return ReplyCheck{Text: fallback, Corrected: true, Reason: "resposta_vazia"}
	}
	normalized := nlpsvc.Normalize(text)
	for _, marker := range promptLeakMarkers {
		if strings.Contains(normalized, marker) {
			return ReplyCheck{Text: fallback, Corrected: true, Reason: "vazamento_de_instrucoes"}
		}
	}
	for tool, claims := range actionClaims {
		if executed[tool] {
			continue
		}
		for _, claim := range claims {
			if strings.Contains(normalized, claim) {
				return ReplyCheck{Text: fallback, Corrected: true, Reason: "acao_nao_executada_afirmada:" + tool}
			}
		}
	}
	redacted := logging.Redact(text)
	if redacted != text {
		return ReplyCheck{Text: redacted, Corrected: true, Reason: "dado_pessoal_mascarado"}
	}
	if len([]rune(redacted)) > 1200 {
		return ReplyCheck{Text: fallback, Corrected: true, Reason: "resposta_longa_demais"}
	}
	return ReplyCheck{Text: redacted}
}
