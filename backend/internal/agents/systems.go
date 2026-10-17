// Package agents runs the ORION AI agents: specialised assistants that answer
// customers by calling tools (function calling) over Claro's internal systems.
//
// Each agent only sees the tools of its domain (least privilege), every tool
// acts on the authenticated customer and never on an id supplied by the model,
// and tools that change something require a double check before running.
package agents

import (
	"fmt"
	"hash/fnv"
	"time"
)

// The functions below stand in for Claro's internal systems (network
// inventory, billing, usage metering, product catalogue). They are
// deterministic per customer, so the same account always sees the same data
// and a demo is reproducible, but they hold no real information.

func seedFor(userID string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(userID))
	return hash.Sum32()
}

// Diagnosis is the network inventory view of a customer's connection.
type Diagnosis struct {
	Status             string  `json:"status"`
	ContractedMbps     int     `json:"velocidadeContratadaMbps"`
	MeasuredMbps       int     `json:"velocidadeMedidaMbps"`
	PacketLossPercent  float64 `json:"perdaDePacotesPercentual"`
	SignalDBm          int     `json:"sinalOpticoDBm"`
	Equipment          string  `json:"equipamento"`
	Recommendation     string  `json:"recomendacao"`
	RestartRecommended bool    `json:"reinicioRecomendado"`
	OutageInRegion     bool    `json:"instabilidadeNaRegiao"`
}

func diagnose(userID string) Diagnosis {
	seed := seedFor(userID)
	contracted := []int{350, 500, 600, 1000}[seed%4]
	measured := contracted * (35 + int(seed%30)) / 100
	diagnosis := Diagnosis{
		Status:             "degradado",
		ContractedMbps:     contracted,
		MeasuredMbps:       measured,
		PacketLossPercent:  float64(2+seed%6) + 0.4,
		SignalDBm:          -18 - int(seed%9),
		Equipment:          []string{"Modem Claro Wi-Fi 6 HGU", "Modem Claro Fibra Wi-Fi 5", "Roteador Claro Mesh"}[seed%3],
		RestartRecommended: true,
		OutageInRegion:     false,
	}
	diagnosis.Recommendation = "O sinal está dentro da faixa, mas o equipamento apresenta perda de pacotes. Um reinício remoto costuma normalizar a conexão."
	return diagnosis
}

// InvoiceItem is one line of a bill.
type InvoiceItem struct {
	Description string  `json:"descricao"`
	Amount      float64 `json:"valor"`
}

// Invoice is the billing system view of the current bill.
type Invoice struct {
	Reference string        `json:"referencia"`
	Amount    float64       `json:"valorTotal"`
	DueDate   string        `json:"vencimento"`
	Status    string        `json:"status"`
	Items     []InvoiceItem `json:"itens"`
}

func currentInvoice(userID string, now time.Time) Invoice {
	seed := seedFor(userID)
	base := []float64{129.90, 159.90, 199.90, 249.90}[seed%4]
	items := []InvoiceItem{
		{Description: "Plano de internet fibra", Amount: base},
		{Description: "Claro TV+ Box", Amount: 39.90},
	}
	if seed%2 == 0 {
		items = append(items, InvoiceItem{Description: "Serviço adicional de streaming", Amount: 19.90})
	}
	total := 0.0
	for _, item := range items {
		total += item.Amount
	}
	due := time.Date(now.Year(), now.Month(), 10+int(seed%15), 0, 0, 0, 0, time.UTC)
	return Invoice{
		Reference: fmt.Sprintf("%02d/%d", int(now.Month()), now.Year()),
		Amount:    roundCents(total),
		DueDate:   due.Format("02/01/2006"),
		Status:    "em aberto",
		Items:     items,
	}
}

// barcodeFor returns a masked digitable line: enough for the customer to
// recognise the bill, while the full line goes only to the registered e-mail.
func barcodeFor(userID string) string {
	seed := seedFor(userID)
	return fmt.Sprintf("84670000001-%d ...... %04d", seed%10, seed%10000)
}

// Usage is the metering view of the current billing cycle.
type Usage struct {
	AllowanceGB float64 `json:"franquiaGB"`
	UsedGB      float64 `json:"usadoGB"`
	Percent     int     `json:"percentualUsado"`
	RenewsOn    string  `json:"renovaEm"`
}

func currentUsage(userID string, now time.Time) Usage {
	seed := seedFor(userID)
	allowance := []float64{25, 50, 80}[seed%3]
	used := allowance * float64(40+seed%50) / 100
	renew := time.Date(now.Year(), now.Month(), 28, 0, 0, 0, 0, time.UTC)
	if !renew.After(now) {
		renew = renew.AddDate(0, 1, 0)
	}
	return Usage{
		AllowanceGB: allowance,
		UsedGB:      roundCents(used),
		Percent:     int(used / allowance * 100),
		RenewsOn:    renew.Format("02/01/2006"),
	}
}

// Offer is a product the commercial agent may propose.
type Offer struct {
	ID         string  `json:"id"`
	Name       string  `json:"nome"`
	SpeedMbps  int     `json:"velocidadeMbps"`
	MonthlyFee float64 `json:"mensalidade"`
	Benefit    string  `json:"beneficio"`
}

var catalogue = []Offer{
	{ID: "FIBRA-600", Name: "Claro Fibra 600 Mega", SpeedMbps: 600, MonthlyFee: 159.90, Benefit: "Wi-Fi 6 incluso"},
	{ID: "FIBRA-1G", Name: "Claro Fibra 1 Giga", SpeedMbps: 1000, MonthlyFee: 219.90, Benefit: "Wi-Fi 6 + repetidor Mesh"},
	{ID: "FIBRA-1G-STREAM", Name: "Claro Fibra 1 Giga + Streaming", SpeedMbps: 1000, MonthlyFee: 249.90, Benefit: "Globoplay e Max inclusos"},
}

func offersFor(userID string) []Offer {
	seed := seedFor(userID)
	// A customer only sees upgrades, so the cheapest tier drops out for the
	// accounts that already have it.
	if seed%2 == 0 {
		return append([]Offer(nil), catalogue[1:]...)
	}
	return append([]Offer(nil), catalogue...)
}

func roundCents(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}

func brl(value float64) string {
	whole := int(value)
	cents := int((value-float64(whole))*100 + 0.5)
	if cents == 100 {
		whole++
		cents = 0
	}
	return fmt.Sprintf("R$ %d,%02d", whole, cents)
}
