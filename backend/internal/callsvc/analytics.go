package callsvc

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/orion-cx/orion-backend/internal/domain"
)

// Bucket is a labelled count.
type Bucket struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// Segment is satisfaction aggregated over one slice of conversations.
type Segment struct {
	Key       string  `json:"key"`
	Responses int     `json:"responses"`
	Average   float64 `json:"average"`
	CSAT      float64 `json:"csat"`
}

// SurveyEntry is one answered survey, shown on the dashboard. Only the
// customer's first name travels with it (LGPD minimisation).
type SurveyEntry struct {
	ConversationID string    `json:"conversationId"`
	Customer       string    `json:"customer"`
	Score          int       `json:"score"`
	Comment        string    `json:"comment"`
	ResolvedBy     string    `json:"resolvedBy"`
	Agent          string    `json:"agent"`
	Intent         string    `json:"intent"`
	Channel        string    `json:"channel"`
	RatedAt        time.Time `json:"ratedAt"`
}

// DayPoint is one point of a daily series.
type DayPoint struct {
	Day       string  `json:"day"`
	Responses int     `json:"responses"`
	Average   float64 `json:"average"`
	Resolved  int     `json:"resolved"`
}

// Satisfaction is the CSAT section of the dashboard.
type Satisfaction struct {
	Responses    int     `json:"responses"`
	ResponseRate float64 `json:"responseRate"`
	Average      float64 `json:"average"`
	CSAT         float64 `json:"csat"`
	// NPSLike is promoters (5) minus detractors (1-3), as a percentage of
	// answers: a stricter view than CSAT, which counts 4 and 5 alike.
	NPSLike      float64       `json:"npsLike"`
	Distribution [5]int        `json:"distribution"`
	ByResolver   []Segment     `json:"byResolver"`
	ByIntent     []Segment     `json:"byIntent"`
	ByAgent      []Segment     `json:"byAgent"`
	ByChannel    []Segment     `json:"byChannel"`
	Daily        []DayPoint    `json:"daily"`
	Recent       []SurveyEntry `json:"recent"`
}

// AISection summarises the AI audit trail.
type AISection struct {
	ToolCalls              int                 `json:"toolCalls"`
	ToolErrors             int                 `json:"toolErrors"`
	ToolsBlocked           int                 `json:"toolsBlocked"`
	SecurityBlocks         int                 `json:"securityBlocks"`
	RateLimited            int                 `json:"rateLimited"`
	QueueSheds             int                 `json:"queueSheds"`
	RepliesCorrected       int                 `json:"repliesCorrected"`
	ConfirmationsRequested int                 `json:"confirmationsRequested"`
	ConfirmationsAccepted  int                 `json:"confirmationsAccepted"`
	ConfirmationsDeclined  int                 `json:"confirmationsDeclined"`
	ConfirmationsRejected  int                 `json:"confirmationsRejected"`
	AvgToolLatencyMs       float64             `json:"avgToolLatencyMs"`
	ByTool                 []Bucket            `json:"byTool"`
	ByAgent                []Bucket            `json:"byAgent"`
	Recent                 []domain.AuditEntry `json:"recent"`
}

// Analytics is the payload behind the dashboards.
type Analytics struct {
	GeneratedAt time.Time `json:"generatedAt"`
	WindowDays  int       `json:"windowDays"`

	Conversations struct {
		Total        int `json:"total"`
		Bot          int `json:"bot"`
		WaitingHuman int `json:"waitingHuman"`
		InProgress   int `json:"inProgress"`
		Resolved     int `json:"resolved"`
	} `json:"conversations"`

	// AutomationRate is the share of resolved conversations closed without a
	// person: the headline metric of an AI-first contact centre.
	AutomationRate     float64  `json:"automationRate"`
	HandoffRate        float64  `json:"handoffRate"`
	AvgConfidence      float64  `json:"avgConfidence"`
	AvgHandlingMinutes float64  `json:"avgHandlingMinutes"`
	CrossChannelRate   float64  `json:"crossChannelRate"`
	Intents            []Bucket `json:"intents"`
	Channels           []Bucket `json:"channels"`

	Tickets struct {
		Open       int `json:"open"`
		InProgress int `json:"inProgress"`
		Resolved   int `json:"resolved"`
	} `json:"tickets"`

	Satisfaction Satisfaction `json:"satisfaction"`
	AI           AISection    `json:"ai"`
}

// AnalyticsWindow is how far back the AI audit is read.
const AnalyticsWindow = 7 * 24 * time.Hour

// Analytics computes the dashboard from the operational data. A prototype can
// afford a full scan; production would maintain these aggregates in a
// read model (materialised views or an OLAP store fed by the event bus).
func (s *Service) Analytics(ctx context.Context) (Analytics, error) {
	now := time.Now().UTC()
	conversations, err := s.repo.ListConversations(ctx, ConversationFilter{})
	if err != nil {
		return Analytics{}, err
	}
	tickets, err := s.repo.ListTickets(ctx, "")
	if err != nil {
		return Analytics{}, err
	}
	audit, err := s.repo.ListAudit(ctx, now.Add(-AnalyticsWindow), 5000)
	if err != nil {
		return Analytics{}, err
	}

	result := Analytics{GeneratedAt: now, WindowDays: int(AnalyticsWindow.Hours() / 24)}
	intents := map[string]int{}
	channels := map[string]int{}
	var confidenceSum float64
	var confidenceCount, handled, resolvedByBot, crossChannel int
	var handlingMinutes float64
	var handlingCount int

	for _, conversation := range conversations {
		result.Conversations.Total++
		switch conversation.Status {
		case domain.StatusBot:
			result.Conversations.Bot++
		case domain.StatusWaitingHuman:
			result.Conversations.WaitingHuman++
		case domain.StatusInProgress:
			result.Conversations.InProgress++
		case domain.StatusResolved:
			result.Conversations.Resolved++
			if conversation.AssignedAgent == nil {
				resolvedByBot++
			}
			if conversation.ResolvedAt != nil {
				handlingMinutes += conversation.ResolvedAt.Sub(conversation.CreatedAt).Minutes()
				handlingCount++
			}
		}
		if conversation.AssignedAgent != nil || conversation.Status == domain.StatusWaitingHuman {
			handled++
		}
		if conversation.Intent != "" && conversation.Intent != IntentUnclassified {
			intents[conversation.Intent]++
			confidenceSum += conversation.IntentConfidence
			confidenceCount++
		}
		if len(conversation.Messages) > 0 {
			channels[string(conversation.LastChannel())]++
		}
		if usedChannels(conversation) > 1 {
			crossChannel++
		}
	}

	total := result.Conversations.Total
	result.AutomationRate = ratio(resolvedByBot, result.Conversations.Resolved)
	result.HandoffRate = ratio(handled, total)
	result.CrossChannelRate = ratio(crossChannel, total)
	if confidenceCount > 0 {
		result.AvgConfidence = round2(confidenceSum / float64(confidenceCount))
	}
	if handlingCount > 0 {
		result.AvgHandlingMinutes = round2(handlingMinutes / float64(handlingCount))
	}
	result.Intents = buckets(intents)
	result.Channels = buckets(channels)

	for _, ticket := range tickets {
		switch ticket.Status {
		case domain.TicketOpen:
			result.Tickets.Open++
		case domain.TicketInProgress:
			result.Tickets.InProgress++
		case domain.TicketResolved:
			result.Tickets.Resolved++
		}
	}

	result.Satisfaction = satisfaction(conversations, now)
	result.AI = aiSection(audit)
	return result, nil
}

func usedChannels(conversation domain.Conversation) int {
	seen := map[domain.Channel]bool{}
	for _, message := range conversation.Messages {
		if message.Actor == domain.ActorCustomer {
			seen[message.Channel] = true
		}
	}
	return len(seen)
}

type scoreAcc struct {
	sum, count, satisfied int
}

func (a *scoreAcc) add(score int) {
	a.sum += score
	a.count++
	if score >= 4 {
		a.satisfied++
	}
}

func (a scoreAcc) segment(key string) Segment {
	return Segment{
		Key:       key,
		Responses: a.count,
		Average:   round2(float64(a.sum) / float64(max(a.count, 1))),
		CSAT:      ratio(a.satisfied, a.count),
	}
}

func segments(accs map[string]*scoreAcc) []Segment {
	result := make([]Segment, 0, len(accs))
	for key, acc := range accs {
		result = append(result, acc.segment(key))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Responses != result[j].Responses {
			return result[i].Responses > result[j].Responses
		}
		return result[i].Key < result[j].Key
	})
	return result
}

func satisfaction(conversations []domain.Conversation, now time.Time) Satisfaction {
	var out Satisfaction
	var all scoreAcc
	promoters, detractors, resolved := 0, 0, 0
	byResolver := map[string]*scoreAcc{}
	byIntent := map[string]*scoreAcc{}
	byAgent := map[string]*scoreAcc{}
	byChannel := map[string]*scoreAcc{}

	// Last 7 days, oldest first, so the chart reads left to right.
	daily := map[string]*DayPoint{}
	days := make([]string, 0, 7)
	for offset := 6; offset >= 0; offset-- {
		day := now.AddDate(0, 0, -offset).Format("2006-01-02")
		days = append(days, day)
		daily[day] = &DayPoint{Day: day}
	}
	dayScores := map[string]*scoreAcc{}

	accFor := func(m map[string]*scoreAcc, key string) *scoreAcc {
		if m[key] == nil {
			m[key] = &scoreAcc{}
		}
		return m[key]
	}

	for _, conversation := range conversations {
		if conversation.Status == domain.StatusResolved {
			resolved++
			if conversation.ResolvedAt != nil {
				if point := daily[conversation.ResolvedAt.UTC().Format("2006-01-02")]; point != nil {
					point.Resolved++
				}
			}
		}
		if conversation.SatisfactionScore == nil {
			continue
		}
		score := *conversation.SatisfactionScore
		all.add(score)
		out.Distribution[score-1]++
		switch {
		case score == 5:
			promoters++
		case score <= 3:
			detractors++
		}

		resolver := conversation.ResolvedBy()
		accFor(byResolver, resolver).add(score)
		accFor(byIntent, conversation.Intent).add(score)
		agent := "Assistente Orion (IA)"
		if conversation.AssignedAgent != nil {
			agent = *conversation.AssignedAgent
		}
		accFor(byAgent, agent).add(score)
		channel := string(conversation.LastChannel())
		accFor(byChannel, channel).add(score)

		if conversation.RatedAt != nil {
			day := conversation.RatedAt.UTC().Format("2006-01-02")
			if daily[day] != nil {
				accFor(dayScores, day).add(score)
			}
		}

		comment := ""
		if conversation.SatisfactionComment != nil {
			comment = *conversation.SatisfactionComment
		}
		ratedAt := conversation.UpdatedAt
		if conversation.RatedAt != nil {
			ratedAt = *conversation.RatedAt
		}
		out.Recent = append(out.Recent, SurveyEntry{
			ConversationID: conversation.ID,
			Customer:       firstName(conversation.CustomerName),
			Score:          score,
			Comment:        comment,
			ResolvedBy:     resolver,
			Agent:          agent,
			Intent:         conversation.Intent,
			Channel:        channel,
			RatedAt:        ratedAt,
		})
	}

	out.Responses = all.count
	out.ResponseRate = ratio(all.count, resolved)
	if all.count > 0 {
		out.Average = round2(float64(all.sum) / float64(all.count))
		out.CSAT = ratio(all.satisfied, all.count)
		out.NPSLike = round2(float64(promoters-detractors) / float64(all.count) * 100)
	}
	out.ByResolver = segments(byResolver)
	out.ByIntent = segments(byIntent)
	out.ByAgent = segments(byAgent)
	out.ByChannel = segments(byChannel)
	for _, day := range days {
		point := daily[day]
		if acc := dayScores[day]; acc != nil {
			point.Responses = acc.count
			point.Average = round2(float64(acc.sum) / float64(acc.count))
		}
		out.Daily = append(out.Daily, *point)
	}
	sort.Slice(out.Recent, func(i, j int) bool { return out.Recent[i].RatedAt.After(out.Recent[j].RatedAt) })
	if len(out.Recent) > 10 {
		out.Recent = out.Recent[:10]
	}
	if out.Recent == nil {
		out.Recent = []SurveyEntry{}
	}
	return out
}

func aiSection(audit []domain.AuditEntry) AISection {
	var out AISection
	byTool := map[string]int{}
	byAgent := map[string]int{}
	var latencySum int64
	for _, entry := range audit {
		switch entry.Kind {
		case domain.AuditToolCall:
			switch entry.Status {
			case "ok":
				out.ToolCalls++
				latencySum += entry.LatencyMs
			case "erro":
				out.ToolCalls++
				out.ToolErrors++
			case "bloqueada":
				out.ToolsBlocked++
			}
			if entry.Tool != "" {
				byTool[entry.Tool]++
			}
			if entry.Agent != "" {
				byAgent[entry.Agent]++
			}
		case domain.AuditSecurityBlock:
			out.SecurityBlocks++
		case domain.AuditRateLimited:
			out.RateLimited++
		case domain.AuditQueueShed:
			out.QueueSheds++
		case domain.AuditReplyCorrected:
			out.RepliesCorrected++
		case domain.AuditConfirmRequested:
			out.ConfirmationsRequested++
		case domain.AuditConfirmAccepted:
			out.ConfirmationsAccepted++
		case domain.AuditConfirmDeclined:
			out.ConfirmationsDeclined++
		case domain.AuditConfirmRejected:
			out.ConfirmationsRejected++
		}
	}
	successful := out.ToolCalls - out.ToolErrors
	if successful > 0 {
		out.AvgToolLatencyMs = round2(float64(latencySum) / float64(successful))
	}
	out.ByTool = buckets(byTool)
	out.ByAgent = buckets(byAgent)
	out.Recent = audit
	if len(out.Recent) > 25 {
		out.Recent = out.Recent[:25]
	}
	if out.Recent == nil {
		out.Recent = []domain.AuditEntry{}
	}
	return out
}

func buckets(counts map[string]int) []Bucket {
	result := make([]Bucket, 0, len(counts))
	for key, count := range counts {
		result = append(result, Bucket{Key: key, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Key < result[j].Key
	})
	return result
}

// ratio returns part/total as a percentage with two decimals.
func ratio(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return round2(float64(part) / float64(total) * 100)
}

func round2(value float64) float64 {
	if value < 0 {
		return -round2(-value)
	}
	return float64(int64(value*100+0.5)) / 100
}

func firstName(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "Cliente"
	}
	return fields[0]
}
