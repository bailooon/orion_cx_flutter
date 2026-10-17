// Package nlpsvc is the ORION Motor NLP/IA. It turns free text into an intent
// plus a calibrated confidence score (RF004).
//
// Two engines implement the same contract: a Claude-backed classifier and a
// deterministic rule engine. The rule engine is not a stub — it is the
// production fallback that keeps the platform answering when the model API is
// unavailable, slow, or simply not configured (RNF007).
package nlpsvc

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Intents recognised by the platform.
const (
	IntentTechnicalSupport = "SUPORTE_TECNICO"
	IntentBillingDispute   = "CONTESTACAO_FATURA"
	IntentInvoiceCopy      = "SEGUNDA_VIA_FATURA"
	IntentPlanUpgrade      = "UPGRADE_PLANO"
	IntentCancellation     = "CANCELAMENTO"
	IntentDataUsage        = "CONSULTA_CONSUMO"
	IntentTicketStatus     = "STATUS_CHAMADO"
	IntentAffirmative      = "CONFIRMACAO"
	IntentNegative         = "NEGACAO"
	IntentGreeting         = "SAUDACAO"
	IntentUnknown          = "EM_ANALISE"
)

// Result is what both engines return.
type Result struct {
	Intent     string  `json:"intent"`
	Confidence float64 `json:"confidence"`
	Summary    string  `json:"summary"`
	// Engine records which classifier produced the answer, so the UI and the
	// logs can tell an LLM decision from a fallback decision.
	Engine string `json:"engine"`
}

// Normalize lowercases text and strips accents so that "está lenta" and
// "esta lenta" hit the same rules.
func Normalize(text string) string {
	lowered := strings.ToLower(strings.TrimSpace(text))
	decomposed := norm.NFD.String(lowered)
	var builder strings.Builder
	builder.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// rule is one intent matcher. A rule fires when at least one term of every
// group is present, which makes multi-word intents ("internet" + "lenta")
// precise without needing a trained model.
type rule struct {
	intent string
	// groups is an AND of ORs: every group must match one of its terms.
	groups [][]string
	// confidence is the calibrated score for this rule.
	confidence float64
	summary    string
}

// rules are evaluated in order; the first match wins. They are ordered from the
// most specific to the most generic.
var rules = []rule{
	{
		intent: IntentBillingDispute,
		groups: [][]string{{"contestar", "contestacao", "indevida", "indevido", "nao reconheco",
			"nao contratei", "cobranca a mais", "cobrado a mais", "reclamar de uma cobranca",
			"cobranca indevida", "valor errado"}},
		// Deliberately below the automation threshold: a financial dispute is
		// resolved with the customer's money, and the phrasings above overlap
		// with invoice-copy and plan questions. The rule engine cannot
		// disambiguate them safely, so it reports low confidence and the
		// gateway hands the conversation to a person (flow B).
		confidence: 0.45,
		summary: "Cliente contesta uma cobrança na fatura. A confiança da classificação ficou " +
			"abaixo do limite de automação e o caso requer atendimento humano.",
	},
	{
		intent:     IntentTechnicalSupport,
		groups:     [][]string{{"internet", "conexao", "wifi", "wi-fi", "sinal", "banda larga", "fibra"}, {"lenta", "lento", "lentidao", "caindo", "caiu", "instavel", "oscilando", "sem ", "nao funciona", "travando", "ruim"}},
		confidence: 0.94,
		summary: "Cliente relata problema de conexão. A ação sugerida é reiniciar o sinal, " +
			"mantendo a sessão disponível entre canais.",
	},
	{
		intent:     IntentTechnicalSupport,
		groups:     [][]string{{"modem", "roteador", "decodificador"}, {"reiniciar", "reset", "problema", "nao liga", "piscando"}},
		confidence: 0.9,
		summary:    "Cliente relata problema no equipamento de acesso.",
	},
	{
		intent:     IntentInvoiceCopy,
		groups:     [][]string{{"segunda via", "2a via", "boleto", "codigo de barras", "linha digitavel"}},
		confidence: 0.92,
		summary:    "Cliente solicita a segunda via da fatura.",
	},
	{
		intent:     IntentDataUsage,
		groups:     [][]string{{"consumo", "franquia", "quanto de internet", "gigas", "gb restante", "meus dados"}},
		confidence: 0.88,
		summary:    "Cliente quer consultar o consumo do plano.",
	},
	{
		intent:     IntentPlanUpgrade,
		groups:     [][]string{{"upgrade", "aumentar o plano", "mudar de plano", "trocar de plano", "plano maior", "mais velocidade"}},
		confidence: 0.86,
		summary:    "Cliente demonstra interesse em evoluir o plano contratado.",
	},
	{
		intent: IntentCancellation,
		groups: [][]string{{"cancelar", "cancelamento", "encerrar contrato", "quero sair"}},
		// Retention is a commercial decision, never automated.
		confidence: 0.5,
		summary:    "Cliente sinaliza intenção de cancelamento. Caso sensível de retenção.",
	},
	{
		intent:     IntentTicketStatus,
		groups:     [][]string{{"protocolo", "chamado", "andamento", "status do"}},
		confidence: 0.85,
		summary:    "Cliente quer acompanhar um chamado aberto.",
	},
	{
		intent:     IntentGreeting,
		groups:     [][]string{{"oi", "ola", "bom dia", "boa tarde", "boa noite"}},
		confidence: 0.8,
		summary:    "Saudação inicial do cliente.",
	},
}

// affirmative/negative terms are checked before the intent rules, because a
// bare "sim" only makes sense as an answer to a pending question.
var (
	affirmativeTerms = []string{"sim", "pode reiniciar", "pode sim", "por favor", "claro", "isso mesmo", "confirmo", "aceito", "continuar", "quero sim"}
	negativeTerms    = []string{"nao", "agora nao", "depois", "prefiro nao", "negativo"}
)

// RuleClassifier is the deterministic engine.
type RuleClassifier struct{}

// NewRuleClassifier builds the fallback engine.
func NewRuleClassifier() *RuleClassifier { return &RuleClassifier{} }

// Name identifies the engine in the response.
func (RuleClassifier) Name() string { return "rules" }

// Classify matches text against the rule table.
func (c *RuleClassifier) Classify(text string, hasPendingQuestion bool) Result {
	normalized := Normalize(text)

	if hasPendingQuestion {
		if matchesAny(normalized, affirmativeTerms) {
			return Result{Intent: IntentAffirmative, Confidence: 0.95,
				Summary: "Cliente confirmou a ação pendente.", Engine: c.Name()}
		}
		if matchesAny(normalized, negativeTerms) {
			return Result{Intent: IntentNegative, Confidence: 0.93,
				Summary: "Cliente recusou a ação pendente.", Engine: c.Name()}
		}
	}

	for _, candidate := range rules {
		if candidate.matches(normalized) {
			return Result{
				Intent:     candidate.intent,
				Confidence: candidate.confidence,
				Summary:    candidate.summary,
				Engine:     c.Name(),
			}
		}
	}

	return Result{
		Intent:     IntentUnknown,
		Confidence: 0.35,
		Summary:    "Solicitação em análise pelo assistente conversacional. A intenção ainda não foi identificada com segurança.",
		Engine:     c.Name(),
	}
}

func (r rule) matches(normalized string) bool {
	for _, group := range r.groups {
		if !matchesAny(normalized, group) {
			return false
		}
	}
	return len(r.groups) > 0
}

// matchesAny reports whether any term appears in the text. Single short words
// such as "sim" or "nao" must match as whole words, otherwise "nao" would fire
// inside "naoperacional".
func matchesAny(normalized string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(term, " ") || len(term) > 4 {
			if strings.Contains(normalized, term) {
				return true
			}
			continue
		}
		if containsWord(normalized, term) {
			return true
		}
	}
	return false
}

func containsWord(normalized, word string) bool {
	for _, field := range strings.FieldsFunc(normalized, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if field == word {
			return true
		}
	}
	return false
}
