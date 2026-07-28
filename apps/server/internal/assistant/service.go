package assistant

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"non24.app/core/agentpolicy"
	"non24.app/core/domain"
	"non24.app/core/scheduling"
	"non24.app/server/internal/provider"
	"non24.app/server/internal/store"
)

const (
	ResultAnswerOnly = "answer_only"
	ResultUnknown    = "unknown"
	ResultRefused    = "refused_medical"
	ResultProposal   = "proposal_pending"

	proposalTTL = 15 * time.Minute
)

type Service struct {
	llm    provider.LLM
	status provider.Status
	store  *store.Store
	now    func() time.Time
}

func New(llm provider.LLM, status provider.Status, st *store.Store) *Service {
	if llm == nil {
		llm = provider.DisabledClient{}
	}
	return &Service{
		llm:    llm,
		status: status,
		store:  st,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Status() provider.Status {
	return s.status
}

func (s *Service) HandleMessage(ctx context.Context, device store.Device, req MessageRequest) (MessageResponse, error) {
	if req.SchemaVersion != SchemaVersion {
		return MessageResponse{}, errors.New("unsupported schema version")
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" || utf8.RuneCountInString(req.Message) > 2000 {
		return MessageResponse{}, errors.New("message is required and must be bounded")
	}
	if req.Context.ZoneID == "" {
		req.Context.ZoneID = "UTC"
	}
	if req.Context.Now.IsZero() {
		req.Context.Now = s.now()
	}
	if agentpolicy.IsMedicalDecisionPrompt(req.Message) {
		return s.answer(ResultRefused, "answer_only", agentpolicy.MedicalRefusal), nil
	}
	if agentpolicy.ContainsMedicalSubject(req.Message) || agentpolicy.ContainsMarkerSubject(req.Message) {
		answer := factualHealthAnswer(req.Message, req.Context)
		if strings.TrimSpace(answer) == "" {
			return s.answer(ResultRefused, "answer_only", agentpolicy.MedicalRefusal), nil
		}
		return s.answer(ResultAnswerOnly, "answer_only", answer), nil
	}
	if err := validatePlanningContext(req.Context); err != nil {
		return MessageResponse{}, err
	}
	if !s.status.Configured {
		return s.answer(ResultAnswerOnly, "answer_only", localFallback(req.Message, s.status)), nil
	}

	action, err := s.callModel(ctx, req, false)
	if errors.Is(err, provider.ErrUsageLimit) {
		return s.answer(ResultAnswerOnly, "answer_only", "The assistant's service hit its usage limit. I can still answer from local scheduling context, and no proposal was created."), nil
	}
	if err != nil {
		action, err = s.callModel(ctx, req, true)
	}
	if errors.Is(err, provider.ErrUsageLimit) {
		return s.answer(ResultAnswerOnly, "answer_only", "The assistant's service hit its usage limit. I can still answer from local scheduling context, and no proposal was created."), nil
	}
	if err != nil {
		return s.answer(ResultUnknown, "answer_only", "I could not turn that into a safe schedule action. No proposal was created."), nil
	}
	// Screen the model's answer on the way out, keyed on what the ANSWER says
	// rather than on what the prompt looked like. The previous form gated this
	// on the prompt containing a medical subject, which made it unreachable:
	// such prompts already returned above, so an unsolicited dosing
	// recommendation from a harmless-looking prompt had nothing to catch it.
	//
	// A hard medication directive is refused unconditionally; softer decision
	// language only counts when the answer is itself about medication or
	// treatment, so ordinary scheduling replies are not falsely refused.
	if agentpolicy.ContainsMedicationDirective(action.Answer) ||
		(agentpolicy.ContainsMedicalSubject(action.Answer) && agentpolicy.IsUnsafeMedicalAnswer(action.Answer)) {
		return s.answer(ResultRefused, "answer_only", agentpolicy.MedicalRefusal), nil
	}
	return s.resolveAction(ctx, device, req, action)
}

func (s *Service) callModel(ctx context.Context, req MessageRequest, compact bool) (modelAction, error) {
	redacted, err := buildRedactedContext(req.Context, compact)
	if err != nil {
		return modelAction{}, err
	}
	resp, err := s.llm.Complete(ctx, provider.Request{
		System:  assistantSystemPrompt(),
		User:    req.Message,
		Context: redacted,
		Schema:  actionSchemaPrompt(),
	})
	if err != nil {
		return modelAction{}, err
	}
	return parseModelAction(resp.Text)
}

func parseModelAction(text string) (modelAction, error) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 16*1024 {
		return modelAction{}, errors.New("empty or oversized model output")
	}
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end < start {
		return modelAction{}, errors.New("model output is not JSON")
	}
	var action modelAction
	dec := json.NewDecoder(strings.NewReader(text[start : end+1]))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&action); err != nil {
		return modelAction{}, err
	}
	if err := validateAction(action); err != nil {
		return modelAction{}, err
	}
	action.Answer = safeAnswer(action.Answer)
	return action, nil
}

func validateAction(action modelAction) error {
	if utf8.RuneCountInString(action.Answer) > 2000 {
		return errors.New("assistant answer is too long")
	}
	if action.SchemaVersion != SchemaVersion {
		return errors.New("unsupported action schema version")
	}
	switch action.RecommendedAction {
	case "answer_only":
		if action.Target != nil {
			return errors.New("answer_only must not include a target")
		}
	case "propose_move_task", "propose_place_task", "propose_reminder_shift":
		if action.Target == nil || !contextIdentifierPattern.MatchString(action.Target.TaskID) {
			return errors.New("proposal action requires a task target")
		}
		if action.Target.ReminderID != "" && !contextIdentifierPattern.MatchString(action.Target.ReminderID) {
			return errors.New("proposal reminder id is invalid")
		}
		if action.Target.DurationMinutes < 0 || action.Target.DurationMinutes > 1440 {
			return errors.New("proposal duration is outside the allowed range")
		}
		if action.Target.PreferredAfterWakeMinutes != nil && (*action.Target.PreferredAfterWakeMinutes < 0 || *action.Target.PreferredAfterWakeMinutes > 1440) {
			return errors.New("proposal wake offset is outside the allowed range")
		}
		if (action.Target.EarliestStartAt != nil && action.Target.EarliestStartAt.IsZero()) || (action.Target.LatestFinishAt != nil && action.Target.LatestFinishAt.IsZero()) {
			return errors.New("proposal timing bounds are invalid")
		}
		if action.Target.EarliestStartAt != nil && action.Target.LatestFinishAt != nil && !action.Target.EarliestStartAt.Before(*action.Target.LatestFinishAt) {
			return errors.New("proposal finish must be after its start")
		}
	default:
		return errors.New("unknown recommended action")
	}
	return nil
}

func (s *Service) HandleDirectProposal(ctx context.Context, device store.Device, req DirectProposalRequest) (MessageResponse, error) {
	if req.SchemaVersion != SchemaVersion {
		return MessageResponse{}, errors.New("unsupported schema version")
	}
	action := modelAction{
		SchemaVersion:     req.SchemaVersion,
		RecommendedAction: req.RecommendedAction,
		Target:            req.Target,
	}
	if err := validateAction(action); err != nil {
		return MessageResponse{}, err
	}
	if action.RecommendedAction == "answer_only" {
		return MessageResponse{}, errors.New("direct proposal requires a propose action")
	}
	if req.Context.ZoneID == "" {
		req.Context.ZoneID = "UTC"
	}
	if req.Context.Now.IsZero() {
		req.Context.Now = s.now()
	}
	if err := validatePlanningContext(req.Context); err != nil {
		return MessageResponse{}, err
	}
	return s.createPendingProposal(ctx, device, req.Context, action, "agent")
}

func (s *Service) resolveAction(ctx context.Context, device store.Device, req MessageRequest, action modelAction) (MessageResponse, error) {
	if action.RecommendedAction == "answer_only" {
		answer := action.Answer
		if answer == "" {
			answer = "I can answer from the local scheduling context. No proposal was created."
		}
		return s.answer(ResultAnswerOnly, "answer_only", answer), nil
	}
	proposal, err := s.resolveProposal(req.Context, action)
	if err != nil {
		answer := action.Answer
		if answer == "" {
			answer = "I could not resolve that into a safe schedule proposal. No proposal was created."
		}
		return s.answer(ResultUnknown, action.RecommendedAction, answer), nil
	}
	return s.storePendingProposal(ctx, device, action, proposal, "assistant")
}

func (s *Service) createPendingProposal(ctx context.Context, device store.Device, input PlanningContext, action modelAction, source string) (MessageResponse, error) {
	proposal, err := s.resolveProposal(input, action)
	if err != nil {
		return MessageResponse{}, err
	}
	return s.storePendingProposal(ctx, device, action, proposal, source)
}

func (s *Service) storePendingProposal(ctx context.Context, device store.Device, action modelAction, proposal scheduling.Proposal, source string) (MessageResponse, error) {
	proposalID, err := newID("proposal")
	if err != nil {
		return MessageResponse{}, err
	}
	createdAt := s.now()
	expiresAt := createdAt.Add(proposalTTL)
	payload, err := json.Marshal(storedProposalPayload{
		ProposalID: proposalID,
		ActionID:   action.RecommendedAction,
		ScheduleProposals: scheduleProposalsPayload{
			SchemaVersion:    SchemaVersion,
			RequestID:        proposalID,
			GeneratedAt:      createdAt,
			AlgorithmVersion: "assistant-scheduler-v1",
			Proposals: []scheduleProposalPayload{{
				ProposalID:       proposalID,
				TaskID:           string(proposal.TaskID),
				StartAt:          proposal.Window.Start.UTC,
				EndAt:            proposal.Window.End.UTC,
				ZoneID:           proposal.Window.Start.ZoneID,
				Confidence:       confidenceFromDomain(proposal.Confidence),
				ExplanationCodes: proposal.ExplanationCodes,
			}},
			Unplaced: []unplacedPayload{},
		},
		Answer:    action.Answer,
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return MessageResponse{}, err
	}
	audit, err := json.Marshal(map[string]string{"source": source, "event": "proposal_created"})
	if err != nil {
		return MessageResponse{}, err
	}
	record, err := s.store.CreateProposal(ctx, store.ProposalInput{
		ID:        proposalID,
		ActionID:  action.RecommendedAction,
		DeviceID:  device.ID,
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
		Payload:   payload,
		Audit:     audit,
	})
	if err != nil {
		return MessageResponse{}, err
	}
	answer := action.Answer
	if source != "assistant" {
		answer = "I queued a schedule proposal for approval. It awaits human approval."
	} else if answer == "" {
		answer = "I queued a schedule proposal for approval. It awaits human approval."
	}
	return MessageResponse{
		SchemaVersion: SchemaVersion,
		Backend:       s.status,
		Result:        ResultProposal,
		Action:        action.RecommendedAction,
		Answer:        answer,
		Proposals: []ProposalSummary{{
			ProposalID:    record.ID,
			Status:        record.Status,
			DecisionToken: record.DecisionToken,
			Payload:       record.Payload,
		}},
	}, nil
}

func (s *Service) resolveProposal(input PlanningContext, action modelAction) (scheduling.Proposal, error) {
	task, ok := taskByID(input.Tasks, action.Target.TaskID)
	if !ok {
		return scheduling.Proposal{}, errors.New("task not found")
	}
	if action.Target.DurationMinutes > 0 {
		task.DurationMinutes = action.Target.DurationMinutes
	}
	if action.Target.EarliestStartAt != nil {
		task.EarliestStartAt = action.Target.EarliestStartAt
	}
	if action.Target.LatestFinishAt != nil {
		task.LatestFinishAt = action.Target.LatestFinishAt
	}
	if action.Target.PreferredAfterWakeMinutes != nil {
		task.PreferredAfterWakeMinutes = action.Target.PreferredAfterWakeMinutes
	}
	domainTask, err := toDomainTask(task, input.ZoneID)
	if err != nil {
		return scheduling.Proposal{}, err
	}
	availability, err := toDomainAvailability(input.Availability, input.EstimateID, input.ZoneID)
	if err != nil {
		return scheduling.Proposal{}, err
	}
	events, err := toDomainEvents(input.FixedEvents, input.ZoneID)
	if err != nil {
		return scheduling.Proposal{}, err
	}
	return scheduling.Scheduler{}.Propose(scheduling.Request{
		Task:         domainTask,
		Availability: availability,
		Events:       events,
		Now:          input.Now.UTC(),
	})
}

func (s *Service) answer(result, action, answer string) MessageResponse {
	return MessageResponse{
		SchemaVersion: SchemaVersion,
		Backend:       s.status,
		Result:        result,
		Action:        action,
		Answer:        answer,
		Proposals:     []ProposalSummary{},
	}
}

func taskByID(tasks []TaskContext, id string) (TaskContext, bool) {
	for _, task := range tasks {
		if task.TaskID == id {
			return task, true
		}
	}
	return TaskContext{}, false
}

func toDomainTask(task TaskContext, zoneID string) (domain.FlexibleTask, error) {
	if task.TaskID == "" || task.DurationMinutes <= 0 {
		return domain.FlexibleTask{}, errors.New("task id and duration are required")
	}
	constraint := domain.TaskConstraint{
		BusinessHours:          task.BusinessHours,
		BusinessStartLocal:     task.BusinessStartLocal,
		BusinessEndLocal:       task.BusinessEndLocal,
		MinimumConfidence:      confidenceLevel(task.MinimumConfidence),
		AllowAutomaticMovement: false,
		RequiresApproval:       true,
	}
	if task.EarliestStartAt != nil {
		instant, err := domain.NewZonedInstant(*task.EarliestStartAt, zoneID)
		if err != nil {
			return domain.FlexibleTask{}, err
		}
		constraint.EarliestStart = &instant
	}
	if task.LatestFinishAt != nil {
		instant, err := domain.NewZonedInstant(*task.LatestFinishAt, zoneID)
		if err != nil {
			return domain.FlexibleTask{}, err
		}
		constraint.LatestFinish = &instant
	}
	if task.PreferredAfterWakeMinutes != nil {
		duration := time.Duration(*task.PreferredAfterWakeMinutes) * time.Minute
		constraint.PreferredAfterWake = &duration
	}
	return domain.FlexibleTask{
		ID:                domain.FlexibleTaskID(task.TaskID),
		Title:             "Flexible task",
		EstimatedDuration: time.Duration(task.DurationMinutes) * time.Minute,
		Constraint:        constraint,
	}, nil
}

func toDomainAvailability(values []AvailabilityContext, estimateID, fallbackZone string) ([]domain.AvailabilityWindow, error) {
	var result []domain.AvailabilityWindow
	for i, value := range values {
		zone := value.ZoneID
		if zone == "" {
			zone = fallbackZone
		}
		start, err := domain.NewZonedInstant(value.StartAt, zone)
		if err != nil {
			return nil, err
		}
		end, err := domain.NewZonedInstant(value.EndAt, zone)
		if err != nil {
			return nil, err
		}
		window := domain.TimeRange{Start: start, End: end}
		if err := window.Validate(); err != nil {
			return nil, err
		}
		result = append(result, domain.AvailabilityWindow{
			ID:         domain.AvailabilityWindowID(fmt.Sprintf("availability_%02d", i+1)),
			Kind:       availabilityKind(value.Kind),
			Interval:   window,
			Confidence: domain.InferenceConfidence{Level: confidenceLevel(value.Confidence), Reasons: []string{"assistant planning context"}},
			EstimateID: domain.PhaseEstimateID(estimateID),
		})
	}
	return result, nil
}

func toDomainEvents(values []FixedEventContext, fallbackZone string) ([]domain.CalendarEvent, error) {
	var result []domain.CalendarEvent
	for _, value := range values {
		zone := value.ZoneID
		if zone == "" {
			zone = fallbackZone
		}
		start, err := domain.NewZonedInstant(value.StartAt, zone)
		if err != nil {
			return nil, err
		}
		end, err := domain.NewZonedInstant(value.EndAt, zone)
		if err != nil {
			return nil, err
		}
		window := domain.TimeRange{Start: start, End: end}
		if err := window.Validate(); err != nil {
			return nil, err
		}
		result = append(result, domain.CalendarEvent{
			ID:       domain.CalendarEventID(value.EventID),
			Title:    "Fixed event",
			Interval: window,
			Fixed:    true,
		})
	}
	return result, nil
}

func availabilityKind(value string) domain.AvailabilityKind {
	switch value {
	case string(domain.AvailabilityFunctional):
		return domain.AvailabilityFunctional
	case string(domain.AvailabilityFree):
		return domain.AvailabilityFree
	default:
		return domain.AvailabilityPredictedWake
	}
}

func confidenceLevel(value string) domain.ConfidenceLevel {
	switch value {
	case string(domain.ConfidenceHigh):
		return domain.ConfidenceHigh
	case string(domain.ConfidenceLow):
		return domain.ConfidenceLow
	default:
		return domain.ConfidenceMedium
	}
}

func confidenceFromDomain(value domain.InferenceConfidence) confidencePayload {
	reasons := append([]string(nil), value.Reasons...)
	if len(reasons) == 0 {
		reasons = []string{"scheduler returned this confidence level"}
	}
	return confidencePayload{Level: string(value.Level), Reasons: reasons}
}

func newID(prefix string) (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
}

func localFallback(message string, status provider.Status) string {
	if strings.Contains(strings.ToLower(message), "where") && strings.Contains(strings.ToLower(message), "data") {
		return "No LLM provider is configured. The assistant is answering from this self-hosted instance only, and no provider call was made."
	}
	if status.Provider == "" || status.Provider == string(provider.Disabled) {
		return "No LLM provider is configured. I can answer only from local scheduling context, and no proposal was created."
	}
	return "The assistant provider is not available. I can answer only from local scheduling context, and no proposal was created."
}

func safeAnswer(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) > 2000 {
		value = string(runes[:2000])
	}
	replacements := map[string]string{
		"DLMO":             "estimated sleep-wake phase",
		"dlmo":             "estimated sleep-wake phase",
		"circadian phase":  "estimated sleep-wake phase",
		"Circadian phase":  "estimated sleep-wake phase",
		"exact phase":      "estimated sleep-wake phase",
		"treatment advice": "planning help",
	}
	for old, replacement := range replacements {
		value = strings.ReplaceAll(value, old, replacement)
	}
	return value
}

func (r MessageResponse) MarshalJSON() ([]byte, error) {
	type alias MessageResponse
	if r.Proposals == nil {
		r.Proposals = []ProposalSummary{}
	}
	return json.Marshal(alias(r))
}

func redactionContainsForbidden(data []byte, forbidden ...string) bool {
	lower := bytes.ToLower(data)
	for _, item := range forbidden {
		if bytes.Contains(lower, bytes.ToLower([]byte(item))) {
			return true
		}
	}
	return false
}
