package nlpsvc

import "testing"

func TestNormalizeStripsAccentsAndCase(t *testing.T) {
	cases := map[string]string{
		"Minha internet está LENTA": "minha internet esta lenta",
		"Contestação":               "contestacao",
		"  Olá  ":                   "ola",
	}
	for input, expected := range cases {
		if got := Normalize(input); got != expected {
			t.Errorf("Normalize(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestRuleClassifierIntents(t *testing.T) {
	classifier := NewRuleClassifier()

	tests := []struct {
		name            string
		text            string
		pendingQuestion bool
		wantIntent      string
		// automatable says whether the confidence should clear the default 0.70
		// automation threshold.
		automatable bool
	}{
		{
			name:        "acceptance flow A phrase is confidently technical",
			text:        "minha internet está lenta",
			wantIntent:  IntentTechnicalSupport,
			automatable: true,
		},
		{
			name:        "accent-free spelling matches the same rule",
			text:        "minha internet esta lenta",
			wantIntent:  IntentTechnicalSupport,
			automatable: true,
		},
		{
			name:        "connection dropping is technical support",
			text:        "o wifi fica caindo toda hora",
			wantIntent:  IntentTechnicalSupport,
			automatable: true,
		},
		{
			name:        "acceptance flow B phrase is a billing dispute below threshold",
			text:        "quero contestar uma cobrança indevida",
			wantIntent:  IntentBillingDispute,
			automatable: false,
		},
		{
			name:        "unrecognised charge is also a dispute",
			text:        "não reconheço esse valor na fatura",
			wantIntent:  IntentBillingDispute,
			automatable: false,
		},
		{
			name:        "invoice copy is automatable",
			text:        "preciso da segunda via do boleto",
			wantIntent:  IntentInvoiceCopy,
			automatable: true,
		},
		{
			name:        "cancellation stays below the threshold for retention",
			text:        "quero cancelar meu plano",
			wantIntent:  IntentCancellation,
			automatable: false,
		},
		{
			name:        "data usage question",
			text:        "quanto ainda tenho de franquia?",
			wantIntent:  IntentDataUsage,
			automatable: true,
		},
		{
			name:        "greeting",
			text:        "bom dia",
			wantIntent:  IntentGreeting,
			automatable: true,
		},
		{
			name:        "gibberish is not classified",
			text:        "preciso de uma ajuda aí",
			wantIntent:  IntentUnknown,
			automatable: false,
		},
		{
			name:            "yes only counts as confirmation when a question is pending",
			text:            "sim, pode reiniciar",
			pendingQuestion: true,
			wantIntent:      IntentAffirmative,
			automatable:     true,
		},
		{
			name:            "no is a refusal when a question is pending",
			text:            "agora não",
			pendingQuestion: true,
			wantIntent:      IntentNegative,
			automatable:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := classifier.Classify(test.text, test.pendingQuestion)
			if result.Intent != test.wantIntent {
				t.Fatalf("Classify(%q) intent = %s, want %s (confidence %v)",
					test.text, result.Intent, test.wantIntent, result.Confidence)
			}
			if automatable := result.Confidence >= 0.70; automatable != test.automatable {
				t.Fatalf("Classify(%q) confidence %v: automatable=%v, want %v",
					test.text, result.Confidence, automatable, test.automatable)
			}
			if result.Engine != "rules" {
				t.Fatalf("expected the rule engine to answer, got %q", result.Engine)
			}
			if result.Summary == "" {
				t.Fatal("every classification must carry a summary for the agent")
			}
		})
	}
}

// TestAffirmativeIgnoredWithoutPendingQuestion guards the rule that a bare
// "sim" outside a pending question must not be read as a confirmation.
func TestAffirmativeIgnoredWithoutPendingQuestion(t *testing.T) {
	result := NewRuleClassifier().Classify("sim", false)
	if result.Intent == IntentAffirmative {
		t.Fatal("a confirmation without a pending question must not be accepted")
	}
}

// TestShortWordsMatchWholeWordsOnly guards against "nao" firing inside a longer
// word.
func TestShortWordsMatchWholeWordsOnly(t *testing.T) {
	if containsWord("naoperacional", "nao") {
		t.Fatal("short terms must match whole words only")
	}
	if !containsWord("agora nao mesmo", "nao") {
		t.Fatal("expected a whole-word match")
	}
}

// TestEmptyTextIsHandled ensures the service never panics on empty input.
func TestEmptyTextIsHandled(t *testing.T) {
	result := NewRuleClassifier().Classify("", false)
	if result.Intent != IntentUnknown {
		t.Fatalf("expected %s for empty text, got %s", IntentUnknown, result.Intent)
	}
}
