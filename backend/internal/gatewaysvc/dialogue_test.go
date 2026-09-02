package gatewaysvc

import (
	"strings"
	"testing"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/nlpsvc"
)

const threshold = 0.70

func TestDecideRoutesConfidentTechnicalSupportToAutomation(t *testing.T) {
	decision := decide(nlpsvc.Result{Intent: nlpsvc.IntentTechnicalSupport, Confidence: 0.94}, threshold, "", 0)

	if decision.handoff {
		t.Fatal("a confident technical request must stay automated")
	}
	if decision.pendingAction != domain.ActionRestartSignal {
		t.Fatalf("expected a pending restart action, got %q", decision.pendingAction)
	}
	if !strings.Contains(decision.reply, "reiniciar o sinal") {
		t.Fatalf("expected the triage question, got %q", decision.reply)
	}
	if decision.ticketTitle == "" {
		t.Fatal("a technical request must open a protocol the customer can track")
	}
}

// TestDecideAlwaysEscalatesBillingDisputes covers the policy guard: even a
// confident billing dispute goes to a human, because the action would be taken
// over the customer's money.
func TestDecideAlwaysEscalatesBillingDisputes(t *testing.T) {
	for _, confidence := range []float64{0.45, 0.99} {
		decision := decide(nlpsvc.Result{Intent: nlpsvc.IntentBillingDispute, Confidence: confidence}, threshold, "", 0)
		if !decision.handoff {
			t.Fatalf("a billing dispute at confidence %v must be handed to a human", confidence)
		}
		if decision.reason == "" {
			t.Fatal("the dashboard needs a reason for the handoff")
		}
	}
}

func TestDecideEscalatesCancellation(t *testing.T) {
	decision := decide(nlpsvc.Result{Intent: nlpsvc.IntentCancellation, Confidence: 0.99}, threshold, "", 0)
	if !decision.handoff {
		t.Fatal("a cancellation is a retention decision and must reach a person")
	}
}

// TestDecideAsksBeforeEscalatingOnLowConfidence covers the two-strike rule.
func TestDecideAsksBeforeEscalatingOnLowConfidence(t *testing.T) {
	result := nlpsvc.Result{Intent: nlpsvc.IntentUnknown, Confidence: 0.35}

	first := decide(result, threshold, "", 0)
	if first.handoff {
		t.Fatal("the first ambiguous message must ask for detail instead of queueing")
	}
	if first.reply == "" {
		t.Fatal("the customer must get an answer")
	}

	second := decide(result, threshold, "", 1)
	if !second.handoff {
		t.Fatal("a second ambiguous message must escalate")
	}
	if !strings.Contains(second.reason, "abaixo do limite") {
		t.Fatalf("expected the reason to name the threshold, got %q", second.reason)
	}
}

// TestDecideEscalatesConfidentButUnautomatableIntent covers an intent the model
// is sure about but whose confidence is still under the threshold.
func TestDecideEscalatesLowConfidenceKnownIntent(t *testing.T) {
	decision := decide(nlpsvc.Result{Intent: nlpsvc.IntentInvoiceCopy, Confidence: 0.4}, threshold, "", 1)
	if !decision.handoff {
		t.Fatal("a known intent below the threshold must still reach a human")
	}
}

func TestDecideHandlesAnswersToPendingQuestion(t *testing.T) {
	confirm := decide(nlpsvc.Result{Intent: nlpsvc.IntentAffirmative, Confidence: 0.95},
		threshold, domain.ActionRestartSignal, 0)
	if !confirm.resolves {
		t.Fatal("confirming the pending action must complete the journey")
	}
	if confirm.pendingAction != "" {
		t.Fatal("the pending action must be cleared after execution")
	}

	// The same answer arriving on a different channel resolves the resumed
	// action too (flow A, step 7).
	resumed := decide(nlpsvc.Result{Intent: nlpsvc.IntentAffirmative, Confidence: 0.95},
		threshold, domain.ActionContinue, 0)
	if !resumed.resolves {
		t.Fatal("confirming a resumed action must complete the journey")
	}

	decline := decide(nlpsvc.Result{Intent: nlpsvc.IntentNegative, Confidence: 0.93},
		threshold, domain.ActionRestartSignal, 0)
	if decline.resolves || decline.handoff {
		t.Fatal("declining must neither resolve nor escalate")
	}
}

// TestAffirmativeWithoutPendingActionIsNotExecuted guards against a stray "sim"
// triggering an action nobody proposed.
func TestAffirmativeWithoutPendingActionIsNotExecuted(t *testing.T) {
	decision := decide(nlpsvc.Result{Intent: nlpsvc.IntentAffirmative, Confidence: 0.95}, threshold, "", 0)
	if decision.resolves {
		t.Fatal("a confirmation without a pending action must not execute anything")
	}
}

func TestFormatPercent(t *testing.T) {
	cases := map[float64]string{0: "0%", 0.45: "45%", 0.7: "70%", 0.945: "95%", 1: "100%"}
	for value, expected := range cases {
		if got := formatPercent(value); got != expected {
			t.Errorf("formatPercent(%v) = %s, want %s", value, got, expected)
		}
	}
}

// TestEveryDecisionAnswersTheCustomer is a safety net: no routing path may
// leave the customer without a reply.
func TestEveryDecisionAnswersTheCustomer(t *testing.T) {
	intents := []string{
		nlpsvc.IntentTechnicalSupport, nlpsvc.IntentBillingDispute, nlpsvc.IntentInvoiceCopy,
		nlpsvc.IntentPlanUpgrade, nlpsvc.IntentCancellation, nlpsvc.IntentDataUsage,
		nlpsvc.IntentTicketStatus, nlpsvc.IntentAffirmative, nlpsvc.IntentNegative,
		nlpsvc.IntentGreeting, nlpsvc.IntentUnknown, "INTENCAO_DESCONHECIDA",
	}
	pendingStates := []string{"", domain.ActionRestartSignal, domain.ActionContinue, domain.ActionHumanHandoff}

	for _, intent := range intents {
		for _, pending := range pendingStates {
			for _, confidence := range []float64{0.2, 0.75, 0.99} {
				for _, unclear := range []int{0, 1} {
					decision := decide(nlpsvc.Result{Intent: intent, Confidence: confidence}, threshold, pending, unclear)
					if strings.TrimSpace(decision.reply) == "" {
						t.Fatalf("no reply for intent=%s pending=%s confidence=%v unclear=%d",
							intent, pending, confidence, unclear)
					}
				}
			}
		}
	}
}
