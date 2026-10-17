package nlpsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// systemPrompt keeps the model inside the platform vocabulary. Confidence is
// explicitly defined so the number the model returns means the same thing as
// the number the rule engine returns, and the gateway can compare both against
// one threshold.
const systemPrompt = `Você é o motor de NLU da plataforma Orion CX, da Claro Brasil.
Classifique a mensagem do cliente em EXATAMENTE uma das intenções abaixo:

- SUPORTE_TECNICO: problemas de conexão, sinal, lentidão, equipamento.
- CONTESTACAO_FATURA: cliente questiona ou contesta um valor cobrado.
- SEGUNDA_VIA_FATURA: pedido de boleto, segunda via ou código de barras.
- UPGRADE_PLANO: interesse em mudar para um plano maior.
- CANCELAMENTO: intenção de cancelar o serviço.
- CONSULTA_CONSUMO: dúvida sobre franquia ou consumo de dados.
- STATUS_CHAMADO: acompanhamento de um protocolo já aberto.
- CONFIRMACAO: resposta afirmativa a uma pergunta pendente.
- NEGACAO: resposta negativa a uma pergunta pendente.
- SAUDACAO: apenas cumprimento, sem pedido.
- EM_ANALISE: não é possível determinar a intenção com segurança.

Regras de confiança (confidence, de 0 a 1):
- Use a probabilidade real de a intenção estar correta, sem inflar.
- Abaixo de 0.70 o atendimento é encaminhado para um humano, então seja
  conservador quando a mensagem for curta, ambígua ou envolver dinheiro.
- CONFIRMACAO e NEGACAO só devem ser usadas se houver pergunta pendente.

O campo summary deve ter no máximo duas frases, em português, descrevendo o
caso para um atendente humano. Não repita dados pessoais do cliente.`

// responseSchema constrains the model output, so the service never has to
// parse prose.
var responseSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"intent": map[string]any{
			"type": "string",
			"enum": []string{
				IntentTechnicalSupport, IntentBillingDispute, IntentInvoiceCopy,
				IntentPlanUpgrade, IntentCancellation, IntentDataUsage,
				IntentTicketStatus, IntentAffirmative, IntentNegative,
				IntentGreeting, IntentUnknown,
			},
		},
		"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
		"summary":    map[string]any{"type": "string"},
	},
	"required":             []string{"intent", "confidence", "summary"},
	"additionalProperties": false,
}

// LLMClassifier calls Claude through the official Anthropic SDK.
type LLMClassifier struct {
	client  anthropic.Client
	model   string
	timeout time.Duration
}

// NewLLMClassifier builds the model-backed engine. It returns nil when no API
// key is configured, which is the signal for the service to run rules only.
func NewLLMClassifier(apiKey, model string, timeout time.Duration) *LLMClassifier {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	return &LLMClassifier{
		client: anthropic.NewClient(
			option.WithAPIKey(apiKey),
			// Bounds the call so a slow model cannot blow the 2s budget: the
			// service falls back to rules when this fires (RNF001, RNF007).
			option.WithRequestTimeout(timeout),
		),
		model:   model,
		timeout: timeout,
	}
}

// Name identifies the engine in the response.
func (LLMClassifier) Name() string { return "claude" }

// modelResponse mirrors responseSchema.
type modelResponse struct {
	Intent     string  `json:"intent"`
	Confidence float64 `json:"confidence"`
	Summary    string  `json:"summary"`
}

// Classify asks the model to label the message. Any failure is returned as an
// error so the caller can fall back to the rule engine.
func (c *LLMClassifier) Classify(ctx context.Context, text string, hasPendingQuestion bool) (Result, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	userPrompt := "Mensagem do cliente: " + text
	if hasPendingQuestion {
		userPrompt += "\n\nContexto: o assistente fez uma pergunta e aguarda uma resposta sim/não."
	} else {
		userPrompt += "\n\nContexto: não há pergunta pendente."
	}

	response, err := c.client.Messages.New(callCtx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 1024,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
		OutputConfig: anthropic.OutputConfigParam{
			// Classification is a shallow task: low effort keeps the call well
			// inside the latency budget without changing the answer.
			Effort: anthropic.OutputConfigEffortLow,
			Format: anthropic.JSONOutputFormatParam{Schema: responseSchema},
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("claude request failed: %w", err)
	}
	if response.StopReason == anthropic.StopReasonRefusal {
		// A declined classification is treated like any other engine failure:
		// the rule engine answers instead, so the customer is never left
		// without a reply.
		return Result{}, fmt.Errorf("claude declined to classify the message")
	}

	var payload string
	for _, block := range response.Content {
		if textBlock, ok := block.AsAny().(anthropic.TextBlock); ok {
			payload += textBlock.Text
		}
	}
	if strings.TrimSpace(payload) == "" {
		return Result{}, fmt.Errorf("claude returned an empty classification")
	}

	var decoded modelResponse
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return Result{}, fmt.Errorf("claude returned invalid JSON: %w", err)
	}
	if decoded.Intent == "" {
		return Result{}, fmt.Errorf("claude returned an empty intent")
	}
	if decoded.Confidence < 0 || decoded.Confidence > 1 {
		return Result{}, fmt.Errorf("claude returned confidence out of range: %v", decoded.Confidence)
	}

	return Result{
		Intent:     decoded.Intent,
		Confidence: decoded.Confidence,
		Summary:    strings.TrimSpace(decoded.Summary),
		Engine:     c.Name(),
	}, nil
}
