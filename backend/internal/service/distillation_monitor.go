package service

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math/bits"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/tidwall/gjson"
)

const (
	DistillationMonitorModeOff     = "off"
	DistillationMonitorModeObserve = "observe"
	DistillationMonitorModeEnforce = "enforce"

	defaultDistillationWindowMinutes           = 15
	defaultDistillationMinRequests             = 100
	defaultDistillationMinOutputTokens         = int64(300_000)
	defaultDistillationObserveThreshold        = 60
	defaultDistillationRiskThreshold           = 85
	defaultDistillationConsecutiveWindows      = 3
	defaultDistillationMinEvidenceClasses      = 3
	defaultDistillationFeatureRetentionDays    = 7
	defaultDistillationAssessmentRetentionDays = 180
	defaultDistillationQueueSize               = 8192
	distillationFeatureTextLimit               = 64 * 1024
	distillationEvaluationInterval             = time.Minute
	distillationSettlementDelay                = 2 * time.Minute
	distillationFeatureWriteTimeout            = 5 * time.Second
	distillationEvaluationTimeout              = 45 * time.Second
	distillationMaxSimilaritySamples           = 256
)

type DistillationMonitorConfig struct {
	Enabled                 bool    `json:"enabled"`
	Mode                    string  `json:"mode"`
	GroupIDs                []int64 `json:"group_ids"`
	WindowMinutes           int     `json:"window_minutes"`
	MinRequests             int     `json:"min_requests"`
	MinOutputTokens         int64   `json:"min_output_tokens"`
	ObserveThreshold        int     `json:"observe_threshold"`
	RiskThreshold           int     `json:"risk_threshold"`
	ConsecutiveWindows      int     `json:"consecutive_windows"`
	MinEvidenceClasses      int     `json:"min_evidence_classes"`
	FeatureRetentionDays    int     `json:"feature_retention_days"`
	AssessmentRetentionDays int     `json:"assessment_retention_days"`
}

func defaultDistillationMonitorConfig() DistillationMonitorConfig {
	return DistillationMonitorConfig{
		Enabled:                 false,
		Mode:                    DistillationMonitorModeObserve,
		GroupIDs:                []int64{},
		WindowMinutes:           defaultDistillationWindowMinutes,
		MinRequests:             defaultDistillationMinRequests,
		MinOutputTokens:         defaultDistillationMinOutputTokens,
		ObserveThreshold:        defaultDistillationObserveThreshold,
		RiskThreshold:           defaultDistillationRiskThreshold,
		ConsecutiveWindows:      defaultDistillationConsecutiveWindows,
		MinEvidenceClasses:      defaultDistillationMinEvidenceClasses,
		FeatureRetentionDays:    defaultDistillationFeatureRetentionDays,
		AssessmentRetentionDays: defaultDistillationAssessmentRetentionDays,
	}
}

func cloneDistillationMonitorConfig(cfg DistillationMonitorConfig) DistillationMonitorConfig {
	cfg.GroupIDs = append([]int64(nil), cfg.GroupIDs...)
	return cfg
}

func (cfg *DistillationMonitorConfig) normalize() {
	if cfg == nil {
		return
	}
	cfg.Mode = strings.ToLower(strings.TrimSpace(cfg.Mode))
	if cfg.Mode == "" {
		cfg.Mode = DistillationMonitorModeObserve
	}
	cfg.GroupIDs = normalizeInt64IDs(cfg.GroupIDs)
	if cfg.WindowMinutes <= 0 {
		cfg.WindowMinutes = defaultDistillationWindowMinutes
	}
	if cfg.MinRequests <= 0 {
		cfg.MinRequests = defaultDistillationMinRequests
	}
	if cfg.MinOutputTokens <= 0 {
		cfg.MinOutputTokens = defaultDistillationMinOutputTokens
	}
	if cfg.ObserveThreshold <= 0 {
		cfg.ObserveThreshold = defaultDistillationObserveThreshold
	}
	if cfg.RiskThreshold <= 0 {
		cfg.RiskThreshold = defaultDistillationRiskThreshold
	}
	if cfg.ConsecutiveWindows <= 0 {
		cfg.ConsecutiveWindows = defaultDistillationConsecutiveWindows
	}
	if cfg.MinEvidenceClasses <= 0 {
		cfg.MinEvidenceClasses = defaultDistillationMinEvidenceClasses
	}
	if cfg.FeatureRetentionDays <= 0 {
		cfg.FeatureRetentionDays = defaultDistillationFeatureRetentionDays
	}
	if cfg.AssessmentRetentionDays <= 0 {
		cfg.AssessmentRetentionDays = defaultDistillationAssessmentRetentionDays
	}
}

func (cfg DistillationMonitorConfig) validate() error {
	switch cfg.Mode {
	case DistillationMonitorModeOff, DistillationMonitorModeObserve, DistillationMonitorModeEnforce:
	default:
		return fmt.Errorf("invalid distillation monitor mode")
	}
	if cfg.WindowMinutes < 5 || cfg.WindowMinutes > 60 {
		return fmt.Errorf("distillation monitor window must be between 5 and 60 minutes")
	}
	if cfg.MinRequests < 10 || cfg.MinRequests > 1_000_000 {
		return fmt.Errorf("distillation monitor minimum requests is out of range")
	}
	if cfg.MinOutputTokens < 1_000 || cfg.MinOutputTokens > 10_000_000_000 {
		return fmt.Errorf("distillation monitor minimum output tokens is out of range")
	}
	if cfg.ObserveThreshold < 1 || cfg.ObserveThreshold > 100 || cfg.RiskThreshold < 1 || cfg.RiskThreshold > 100 || cfg.ObserveThreshold > cfg.RiskThreshold {
		return fmt.Errorf("distillation monitor score thresholds are invalid")
	}
	if cfg.ConsecutiveWindows < 1 || cfg.ConsecutiveWindows > 12 {
		return fmt.Errorf("distillation monitor consecutive windows is out of range")
	}
	if cfg.MinEvidenceClasses < 2 || cfg.MinEvidenceClasses > 5 {
		return fmt.Errorf("distillation monitor evidence class count is out of range")
	}
	if cfg.FeatureRetentionDays < 1 || cfg.FeatureRetentionDays > 30 || cfg.AssessmentRetentionDays < 1 || cfg.AssessmentRetentionDays > 3650 {
		return fmt.Errorf("distillation monitor retention is out of range")
	}
	return nil
}

func (cfg DistillationMonitorConfig) includesGroup(groupID *int64) bool {
	if !cfg.Enabled || cfg.Mode == DistillationMonitorModeOff || groupID == nil || *groupID <= 0 {
		return false
	}
	for _, configured := range cfg.GroupIDs {
		if configured == *groupID {
			return true
		}
	}
	return false
}

type DistillationMonitorRuntimeStatus struct {
	Enabled      bool       `json:"enabled"`
	Mode         string     `json:"mode"`
	QueueLength  int        `json:"queue_length"`
	QueueSize    int        `json:"queue_size"`
	Enqueued     int64      `json:"enqueued"`
	Dropped      int64      `json:"dropped"`
	Processed    int64      `json:"processed"`
	Errors       int64      `json:"errors"`
	LastRunAt    *time.Time `json:"last_run_at,omitempty"`
	LastFlagged  int64      `json:"last_flagged"`
	LastDisabled int64      `json:"last_disabled"`
}

type DistillationRequestFeature struct {
	RequestID      string
	UserID         int64
	APIKeyID       int64
	GroupID        int64
	Model          string
	PromptSimHash  int64
	PromptLength   int
	MessageCount   int
	ExplicitSignal bool
	CreatedAt      time.Time
}

type DistillationCandidate struct {
	UserID             int64
	UserEmail          string
	GroupID            int64
	GroupName          string
	DominantModel      string
	RequestCount       int
	InputTokens        int64
	OutputTokens       int64
	CacheReadTokens    int64
	NonStreamCount     int
	MissingSessions    int
	DistinctSessions   int
	DistinctAPIKeys    int
	DistinctIPs        int
	DistinctUserAgents int
	ExplicitSignals    int
	FirstRequestAt     time.Time
	LastRequestAt      time.Time
}

type DistillationAssessment struct {
	UserID          int64
	GroupID         int64
	WindowStart     time.Time
	WindowEnd       time.Time
	Score           int
	EvidenceClasses int
	Mode            string
	Metrics         map[string]any
	AutoDisabled    bool
}

type DistillationAssessmentRecord struct {
	ID              int64          `json:"id"`
	UserID          int64          `json:"user_id"`
	UserEmail       string         `json:"user_email"`
	UserStatus      string         `json:"user_status"`
	GroupID         int64          `json:"group_id"`
	GroupName       string         `json:"group_name"`
	DominantModel   string         `json:"dominant_model"`
	WindowStart     time.Time      `json:"window_start"`
	WindowEnd       time.Time      `json:"window_end"`
	Score           int            `json:"score"`
	EvidenceClasses int            `json:"evidence_classes"`
	Mode            string         `json:"mode"`
	Metrics         map[string]any `json:"metrics"`
	AutoDisabled    bool           `json:"auto_disabled"`
	CreatedAt       time.Time      `json:"created_at"`
}

type DistillationAssessmentFilter struct {
	Pagination pagination.PaginationParams
}

type DistillationMonitorRepository interface {
	CreateDistillationRequestFeature(ctx context.Context, feature DistillationRequestFeature) error
	ListDistillationCandidates(ctx context.Context, groupIDs []int64, start, end time.Time, minRequests int, minOutputTokens int64) ([]DistillationCandidate, error)
	ListDistillationPromptSimHashes(ctx context.Context, userID, groupID int64, start, end time.Time, limit int) ([]int64, error)
	CreateDistillationAssessment(ctx context.Context, assessment DistillationAssessment) (bool, error)
	ListDistillationAssessments(ctx context.Context, filter DistillationAssessmentFilter) ([]DistillationAssessmentRecord, *pagination.PaginationResult, error)
	CountConsecutiveDistillationAssessments(ctx context.Context, userID, groupID int64, windowEnd time.Time, window time.Duration, threshold, minEvidenceClasses, required int) (int, error)
	MarkDistillationAssessmentDisabled(ctx context.Context, userID, groupID int64, windowStart time.Time) error
	CleanupDistillationMonitor(ctx context.Context, featuresBefore, assessmentsBefore time.Time) error
}

type distillationScore struct {
	Total           int
	EvidenceClasses int
	Volume          int
	Automation      int
	Session         int
	Template        int
	Fanout          int
	Batch           int
	TemplateRatio   float64
}

func (s *ContentModerationService) enqueueDistillationRequest(input ContentModerationCheckInput) {
	if s == nil || s.distillationQueue == nil || input.GroupID == nil || input.UserID <= 0 || input.RequestID == "" {
		return
	}
	feature := buildDistillationRequestFeature(input)
	select {
	case s.distillationQueue <- feature:
		s.distillationEnqueued.Add(1)
	default:
		s.distillationDropped.Add(1)
	}
}

func (s *ContentModerationService) distillationCaptureWorker() {
	repo, ok := s.repo.(DistillationMonitorRepository)
	if !ok {
		return
	}
	for feature := range s.distillationQueue {
		ctx, cancel := context.WithTimeout(context.Background(), distillationFeatureWriteTimeout)
		err := repo.CreateDistillationRequestFeature(ctx, feature)
		cancel()
		if err != nil {
			s.distillationErrors.Add(1)
			slog.Warn("distillation_monitor.feature_write_failed", "request_id", feature.RequestID, "user_id", feature.UserID, "group_id", feature.GroupID, "error", err)
			continue
		}
		s.distillationProcessed.Add(1)
	}
}

func (s *ContentModerationService) distillationEvaluationWorker() {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		<-timer.C
		s.runDistillationEvaluation(time.Now())
		timer.Reset(distillationEvaluationInterval)
	}
}

func (s *ContentModerationService) runDistillationEvaluation(now time.Time) {
	repo, ok := s.repo.(DistillationMonitorRepository)
	if !ok || s.settingRepo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), distillationEvaluationTimeout)
	defer cancel()
	snapshot, err := s.loadRuntimeSnapshot(ctx)
	if err != nil || snapshot == nil || snapshot.config == nil {
		if err != nil {
			s.distillationErrors.Add(1)
		}
		return
	}
	cfg := snapshot.config.DistillationMonitor
	if !cfg.Enabled || cfg.Mode == DistillationMonitorModeOff || len(cfg.GroupIDs) == 0 {
		return
	}
	window := time.Duration(cfg.WindowMinutes) * time.Minute
	end := now.Add(-distillationSettlementDelay).Truncate(window)
	start := end.Add(-window)
	if !start.Before(end) {
		return
	}
	candidates, err := repo.ListDistillationCandidates(ctx, cfg.GroupIDs, start, end, cfg.MinRequests, cfg.MinOutputTokens)
	if err != nil {
		s.distillationErrors.Add(1)
		slog.Warn("distillation_monitor.candidate_query_failed", "error", err)
		return
	}
	flagged, disabled := int64(0), int64(0)
	for _, candidate := range candidates {
		if _, allowed := snapshot.allowlistedUsers[candidate.UserID]; allowed {
			continue
		}
		hashes, hashErr := repo.ListDistillationPromptSimHashes(ctx, candidate.UserID, candidate.GroupID, start, end, distillationMaxSimilaritySamples)
		if hashErr != nil {
			s.distillationErrors.Add(1)
			continue
		}
		score := scoreDistillationCandidate(candidate, hashes, cfg, window)
		assessment := DistillationAssessment{
			UserID: candidate.UserID, GroupID: candidate.GroupID, WindowStart: start, WindowEnd: end,
			Score: score.Total, EvidenceClasses: score.EvidenceClasses, Mode: cfg.Mode,
			Metrics: distillationMetrics(candidate, score),
		}
		created, createErr := repo.CreateDistillationAssessment(ctx, assessment)
		if createErr != nil {
			s.distillationErrors.Add(1)
			continue
		}
		if !created || score.Total < cfg.ObserveThreshold {
			continue
		}
		flagged++
		consecutive := 0
		if score.Total >= cfg.RiskThreshold && score.EvidenceClasses >= cfg.MinEvidenceClasses {
			consecutive, err = repo.CountConsecutiveDistillationAssessments(ctx, candidate.UserID, candidate.GroupID, end, window, cfg.RiskThreshold, cfg.MinEvidenceClasses, cfg.ConsecutiveWindows)
			if err != nil {
				s.distillationErrors.Add(1)
				continue
			}
		}
		autoDisabled := false
		if cfg.Mode == DistillationMonitorModeEnforce && consecutive >= cfg.ConsecutiveWindows {
			autoDisabled = s.disableDistillationUser(ctx, candidate.UserID, score.Total, candidate.GroupID)
			if autoDisabled {
				disabled++
				_ = repo.MarkDistillationAssessmentDisabled(ctx, candidate.UserID, candidate.GroupID, start)
			}
		}
		s.persistDistillationRiskLog(ctx, candidate, score, cfg, start, end, consecutive, autoDisabled)
	}
	if err := repo.CleanupDistillationMonitor(ctx, now.AddDate(0, 0, -cfg.FeatureRetentionDays), now.AddDate(0, 0, -cfg.AssessmentRetentionDays)); err != nil {
		slog.Warn("distillation_monitor.cleanup_failed", "error", err)
	}
	s.distillationLastRunUnix.Store(now.Unix())
	s.distillationLastFlagged.Store(flagged)
	s.distillationLastDisabled.Store(disabled)
}

func (s *ContentModerationService) disableDistillationUser(ctx context.Context, userID int64, score int, groupID int64) bool {
	if s.userRepo == nil || userID <= 0 {
		return false
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil || user.IsAdmin() || user.Status == StatusDisabled {
		return false
	}
	user.Status = StatusDisabled
	if err := s.userRepo.Update(ctx, user, UserUpdateFields{Status: true}); err != nil {
		slog.Warn("distillation_monitor.disable_user_failed", "user_id", userID, "group_id", groupID, "score", score, "error", err)
		return false
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	slog.Warn("distillation_monitor.user_disabled", "user_id", userID, "group_id", groupID, "score", score)
	return true
}

func (s *ContentModerationService) persistDistillationRiskLog(ctx context.Context, candidate DistillationCandidate, score distillationScore, cfg DistillationMonitorConfig, start, end time.Time, consecutive int, autoDisabled bool) {
	if s.repo == nil {
		return
	}
	userID, groupID := candidate.UserID, candidate.GroupID
	action := "distillation_flag"
	if autoDisabled {
		action = "distillation_disable"
	}
	matchedKeyword := ""
	if candidate.ExplicitSignals > 0 {
		matchedKeyword = "distillation_intent"
	}
	log := &ContentModerationLog{
		RequestID: fmt.Sprintf("distill-%d-%d-%d", candidate.UserID, candidate.GroupID, start.Unix()),
		UserID:    &userID, UserEmail: candidate.UserEmail, GroupID: &groupID, GroupName: candidate.GroupName,
		Endpoint: "behavior_window", Provider: "distillation_monitor", Model: candidate.DominantModel,
		Mode: "distillation_" + cfg.Mode, Action: action, Flagged: true,
		HighestCategory: "model_distillation", HighestScore: float64(score.Total) / 100,
		MatchedKeyword: matchedKeyword,
		CategoryScores: map[string]float64{
			"volume": float64(score.Volume) / 25, "automation": float64(score.Automation) / 30,
			"session_independence": float64(score.Session) / 20, "template_batch": float64(score.Template) / 25,
			"identity_fanout": float64(score.Fanout) / 10,
		},
		ThresholdSnapshot: map[string]float64{"observe": float64(cfg.ObserveThreshold) / 100, "disable": float64(cfg.RiskThreshold) / 100},
		InputExcerpt: fmt.Sprintf("window=%s..%s requests=%d output_tokens=%d non_stream=%.1f%% template_similarity=%.1f%% evidence=%d consecutive=%d",
			start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), candidate.RequestCount, candidate.OutputTokens,
			ratio(candidate.NonStreamCount, candidate.RequestCount)*100, score.TemplateRatio*100, score.EvidenceClasses, consecutive),
		ViolationCount: consecutive, AutoBanned: autoDisabled,
	}
	if err := s.repo.CreateLog(ctx, log); err != nil {
		slog.Warn("distillation_monitor.log_failed", "user_id", candidate.UserID, "group_id", candidate.GroupID, "error", err)
	}
}

func (s *ContentModerationService) distillationRuntimeStatus(cfg DistillationMonitorConfig) DistillationMonitorRuntimeStatus {
	status := DistillationMonitorRuntimeStatus{
		Enabled: cfg.Enabled, Mode: cfg.Mode, QueueSize: defaultDistillationQueueSize,
		Enqueued: s.distillationEnqueued.Load(), Dropped: s.distillationDropped.Load(), Processed: s.distillationProcessed.Load(),
		Errors: s.distillationErrors.Load(), LastFlagged: s.distillationLastFlagged.Load(), LastDisabled: s.distillationLastDisabled.Load(),
	}
	if s.distillationQueue != nil {
		status.QueueLength = len(s.distillationQueue)
	}
	if unix := s.distillationLastRunUnix.Load(); unix > 0 {
		value := time.Unix(unix, 0)
		status.LastRunAt = &value
	}
	return status
}

func (s *ContentModerationService) ListDistillationAssessments(ctx context.Context, filter DistillationAssessmentFilter) ([]DistillationAssessmentRecord, *pagination.PaginationResult, error) {
	repo, ok := s.repo.(DistillationMonitorRepository)
	if !ok {
		return nil, nil, fmt.Errorf("distillation monitor repository unavailable")
	}
	if filter.Pagination.Page <= 0 {
		filter.Pagination.Page = 1
	}
	if filter.Pagination.PageSize <= 0 {
		filter.Pagination.PageSize = 20
	}
	if filter.Pagination.PageSize > 100 {
		filter.Pagination.PageSize = 100
	}
	filter.Pagination.SortOrder = pagination.SortOrderDesc
	return repo.ListDistillationAssessments(ctx, filter)
}

func buildDistillationRequestFeature(input ContentModerationCheckInput) DistillationRequestFeature {
	text, messageCount := distillationRequestText(input.Protocol, input.Body)
	return DistillationRequestFeature{
		RequestID: strings.TrimSpace(input.RequestID), UserID: input.UserID, APIKeyID: input.APIKeyID,
		GroupID: contentModerationLogGroupID(input.GroupID), Model: strings.TrimSpace(input.Model),
		PromptSimHash: int64(distillationSimHash(text)), PromptLength: utf8.RuneCountInString(text),
		MessageCount: messageCount, ExplicitSignal: hasExplicitDistillationSignal(text), CreatedAt: time.Now(),
	}
}

func distillationRequestText(protocol string, body []byte) (string, int) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return "", 0
	}
	text := ExtractContentModerationInput(protocol, body).Text
	if protocol == ContentModerationProtocolOpenAIResponses {
		text = distillationResponsesText(gjson.ParseBytes(body), text)
	}
	if len(text) > distillationFeatureTextLimit {
		text = text[:distillationFeatureTextLimit]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	root := gjson.ParseBytes(body)
	messageCount := maxDistillationInt(distillationItemCount(root.Get("messages")), distillationItemCount(root.Get("input")))
	messageCount = maxDistillationInt(messageCount, distillationItemCount(root.Get("contents")))
	return text, messageCount
}

func distillationResponsesText(root gjson.Result, fallback string) string {
	parts := make([]string, 0, 32)
	remaining := distillationFeatureTextLimit
	collectDistillationResponseValue(root.Get("input"), &parts, &remaining, 0)
	appendDistillationResponseText(&parts, root.Get("instructions").String(), &remaining)
	if len(parts) == 0 {
		return fallback
	}
	return normalizeContentModerationText(strings.Join(parts, "\n"))
}

func appendDistillationResponseText(parts *[]string, value string, remaining *int) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "data:image/") || *remaining <= 0 {
		return
	}
	if len(value) > *remaining {
		value = value[:*remaining]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	if value == "" {
		return
	}
	*parts = append(*parts, value)
	*remaining -= len(value)
}

func collectDistillationResponseValue(value gjson.Result, parts *[]string, remaining *int, depth int) {
	if !value.Exists() || depth > 12 || *remaining <= 0 {
		return
	}
	switch {
	case value.Type == gjson.String:
		appendDistillationResponseText(parts, value.String(), remaining)
	case value.IsArray():
		items := value.Array()
		for index := len(items) - 1; index >= 0 && *remaining > 0; index-- {
			collectDistillationResponseValue(items[index], parts, remaining, depth+1)
		}
	case value.IsObject():
		typeName := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		role := strings.ToLower(strings.TrimSpace(value.Get("role").String()))
		if typeName == "" && role == "" && depth > 0 {
			value.ForEach(func(_, child gjson.Result) bool {
				collectDistillationResponseValue(child, parts, remaining, depth+1)
				return *remaining > 0
			})
			return
		}
		allowed := role == "user" || role == "developer" || role == "system"
		switch typeName {
		case "input_text", "text", "message", "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output", "computer_call_output":
			allowed = true
		}
		if !allowed && depth > 0 {
			return
		}
		for _, key := range []string{"content", "text", "arguments", "input", "output", "summary"} {
			collectDistillationResponseValue(value.Get(key), parts, remaining, depth+1)
		}
	}
}

func distillationItemCount(value gjson.Result) int {
	if value.IsArray() {
		return len(value.Array())
	}
	if value.Exists() {
		return 1
	}
	return 0
}

func distillationSimHash(text string) uint64 {
	tokens := distillationTokens(text)
	if len(tokens) == 0 {
		return 0
	}
	weights := [64]int{}
	for i, token := range tokens {
		term := token
		if i > 0 {
			term = tokens[i-1] + "\x00" + token
		}
		h := fnv.New64a()
		_, _ = h.Write([]byte(term))
		value := h.Sum64()
		for bit := 0; bit < 64; bit++ {
			if value&(uint64(1)<<bit) != 0 {
				weights[bit]++
			} else {
				weights[bit]--
			}
		}
	}
	var result uint64
	for bit, weight := range weights {
		if weight >= 0 {
			result |= uint64(1) << bit
		}
	}
	return result
}

func distillationTokens(text string) []string {
	text = strings.ToLower(text)
	tokens := make([]string, 0, 128)
	var builder strings.Builder
	flush := func() {
		if builder.Len() == 0 {
			return
		}
		token := builder.String()
		builder.Reset()
		if len(token) > 48 || mostlyDigits(token) {
			token = "<id>"
		}
		tokens = append(tokens, token)
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			builder.WriteRune(r)
			continue
		}
		flush()
		if len(tokens) >= 4096 {
			break
		}
	}
	flush()
	return tokens
}

func mostlyDigits(value string) bool {
	if value == "" {
		return false
	}
	digits, total := 0, 0
	for _, r := range value {
		total++
		if unicode.IsDigit(r) {
			digits++
		}
	}
	return total >= 6 && digits*2 >= total
}

func hasExplicitDistillationSignal(text string) bool {
	value := strings.ToLower(text)
	for _, signal := range []string{
		"model distillation", "distill the model", "student model", "teacher model", "training dataset", "synthetic dataset", "generate jsonl",
		"模型蒸馏", "蒸馏模型", "学生模型", "教师模型", "生成训练集", "合成数据集", "生成jsonl", "生成 jsonl",
	} {
		if strings.Contains(value, signal) {
			return true
		}
	}
	return false
}

func scoreDistillationCandidate(candidate DistillationCandidate, hashes []int64, cfg DistillationMonitorConfig, window time.Duration) distillationScore {
	result := distillationScore{}
	outputRatio := float64(candidate.OutputTokens) / float64(maxDistillationInt64(cfg.MinOutputTokens, 1))
	switch {
	case outputRatio >= 3:
		result.Volume = 25
	case outputRatio >= 2:
		result.Volume = 20
	default:
		result.Volume = 15
	}

	requestRatio := float64(candidate.RequestCount) / float64(maxDistillationInt(cfg.MinRequests, 1))
	switch {
	case requestRatio >= 4:
		result.Automation += 8
	case requestRatio >= 2:
		result.Automation += 5
	default:
		result.Automation += 3
	}
	if ratio(candidate.NonStreamCount, candidate.RequestCount) >= 0.9 {
		result.Automation += 8
	}
	if !candidate.FirstRequestAt.IsZero() && candidate.LastRequestAt.Sub(candidate.FirstRequestAt) >= time.Duration(float64(window)*0.8) {
		result.Automation += 4
	}

	missingSessionRatio := ratio(candidate.MissingSessions, candidate.RequestCount)
	if missingSessionRatio >= 0.8 {
		result.Session += 8
	}
	presentSessions := candidate.RequestCount - candidate.MissingSessions
	if presentSessions > 0 && ratio(candidate.DistinctSessions, presentSessions) >= 0.8 {
		result.Session += 6
	}
	cacheDenominator := candidate.InputTokens + candidate.CacheReadTokens
	if cacheDenominator > 0 && float64(candidate.CacheReadTokens)/float64(cacheDenominator) <= 0.05 {
		result.Session += 6
	}

	nonStreamRatio := ratio(candidate.NonStreamCount, candidate.RequestCount)
	if requestRatio >= 10 && outputRatio >= 2 && nonStreamRatio >= 0.75 && missingSessionRatio >= 0.75 {
		result.Batch = 10
		result.Automation += result.Batch
	}

	result.TemplateRatio = maxDistillationSimilarityRatio(hashes)
	switch {
	case result.TemplateRatio >= 0.8:
		result.Template += 15
	case result.TemplateRatio >= 0.6:
		result.Template += 12
	case result.TemplateRatio >= 0.4:
		result.Template += 8
	}
	explicitRatio := ratio(candidate.ExplicitSignals, candidate.RequestCount)
	switch {
	case explicitRatio >= 0.2:
		result.Template += 10
	case explicitRatio >= 0.05:
		result.Template += 6
	case candidate.ExplicitSignals > 0:
		result.Template += 3
	}

	if candidate.DistinctAPIKeys >= 4 {
		result.Fanout += 4
	} else if candidate.DistinctAPIKeys >= 2 {
		result.Fanout += 2
	}
	if candidate.DistinctIPs >= 3 {
		result.Fanout += 3
	} else if candidate.DistinctIPs >= 2 {
		result.Fanout += 2
	}
	if candidate.DistinctUserAgents >= 3 {
		result.Fanout += 3
	} else if candidate.DistinctUserAgents >= 2 {
		result.Fanout += 2
	}

	result.Total = minDistillationInt(100, result.Volume+result.Automation+result.Session+result.Template+result.Fanout)
	for _, component := range []struct {
		score int
		floor int
	}{{result.Volume, 20}, {result.Automation, 10}, {result.Session, 10}, {result.Template, 8}, {result.Fanout, 4}} {
		if component.score >= component.floor {
			result.EvidenceClasses++
		}
	}
	return result
}

func maxDistillationSimilarityRatio(values []int64) float64 {
	filtered := make([]uint64, 0, len(values))
	for _, value := range values {
		if value != 0 {
			filtered = append(filtered, uint64(value))
		}
	}
	if len(filtered) < 2 {
		return 0
	}
	maxCluster := 1
	for i, left := range filtered {
		cluster := 1
		for j := i + 1; j < len(filtered); j++ {
			if bits.OnesCount64(left^filtered[j]) <= 6 {
				cluster++
			}
		}
		if cluster > maxCluster {
			maxCluster = cluster
		}
	}
	return float64(maxCluster) / float64(len(filtered))
}

func distillationMetrics(candidate DistillationCandidate, score distillationScore) map[string]any {
	return map[string]any{
		"dominant_model": candidate.DominantModel,
		"requests":       candidate.RequestCount, "input_tokens": candidate.InputTokens, "output_tokens": candidate.OutputTokens,
		"cache_read_tokens": candidate.CacheReadTokens, "non_stream_ratio": ratio(candidate.NonStreamCount, candidate.RequestCount),
		"missing_session_ratio": ratio(candidate.MissingSessions, candidate.RequestCount), "distinct_sessions": candidate.DistinctSessions,
		"distinct_api_keys": candidate.DistinctAPIKeys, "distinct_ips": candidate.DistinctIPs, "distinct_user_agents": candidate.DistinctUserAgents,
		"explicit_signals": candidate.ExplicitSignals, "template_similarity": score.TemplateRatio,
		"score_volume": score.Volume, "score_automation": score.Automation, "score_session": score.Session,
		"score_template": score.Template, "score_fanout": score.Fanout, "score_batch": score.Batch,
	}
}

func ratio(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func maxDistillationInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func minDistillationInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxDistillationInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
