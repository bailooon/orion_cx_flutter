package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactMasksPersonalData(t *testing.T) {
	tests := []struct {
		input       string
		mustNotHave string
		mustHave    string
	}{
		{input: "documento 123.456.789-00", mustNotHave: "123.456.789-00", mustHave: "[CPF]"},
		{input: "documento 12345678900", mustNotHave: "12345678900", mustHave: "[CPF]"},
		{input: "contato cliente@orion.dev", mustNotHave: "cliente@orion.dev", mustHave: "[EMAIL]"},
		{input: "ligar para (11) 98765-4321", mustNotHave: "98765-4321", mustHave: "[TELEFONE]"},
	}
	for _, test := range tests {
		got := Redact(test.input)
		if strings.Contains(got, test.mustNotHave) {
			t.Errorf("Redact(%q) = %q still contains %q", test.input, got, test.mustNotHave)
		}
		if !strings.Contains(got, test.mustHave) {
			t.Errorf("Redact(%q) = %q, expected %q", test.input, got, test.mustHave)
		}
	}
}

func TestRedactLeavesOrdinaryTextAlone(t *testing.T) {
	const input = "conversa CX-2026-0142 encaminhada para atendimento humano"
	if got := Redact(input); got != input {
		t.Fatalf("Redact changed non-personal text: %q", got)
	}
}

// TestHandlerRedactsAttributes proves the redaction is enforced by the logger
// itself, not only by callers remembering to sanitise (LGPD).
func TestHandlerRedactsAttributes(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(redactHandler{
		inner: slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug}),
	})

	logger.Info("mensagem recebida de cliente@orion.dev",
		slog.String("email", "cliente@orion.dev"),
		slog.String("password", "orion12345"),
		slog.String("text", "meu cpf é 123.456.789-00"),
		slog.String("conversation_id", "CX-2026-0142"),
	)

	output := buffer.String()
	for _, forbidden := range []string{"cliente@orion.dev", "orion12345", "123.456.789-00"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("log leaked %q: %s", forbidden, output)
		}
	}
	if !strings.Contains(output, "CX-2026-0142") {
		t.Fatalf("the protocol number must survive redaction for support purposes: %s", output)
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Fatalf("expected sensitive keys to be replaced: %s", output)
	}
}

func TestNewLoggerCarriesServiceName(t *testing.T) {
	logger := New("gateway", "development")
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("development logging should include debug records")
	}
	if production := New("gateway", "production"); production.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("production logging should drop debug records")
	}
}
