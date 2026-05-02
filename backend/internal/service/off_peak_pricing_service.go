package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// OffPeakPricingRule 分时费率规则
type OffPeakPricingRule struct {
	StartHour  int     `json:"start_hour"`  // 开始小时 (0-23)
	EndHour    int     `json:"end_hour"`    // 结束小时 (0-23)，支持跨天（如 22→7）
	Multiplier float64 `json:"multiplier"`  // 费率倍数（如 0.8 = 八折）
	Label      string  `json:"label"`       // 标签（如 "夜间优惠"）
	GroupIDs   []int64 `json:"group_ids"`   // 适用分组 ID 列表（空=所有分组）
}

// OffPeakPricingService 分时费率服务（带缓存，避免每次请求解析 JSON）
type OffPeakPricingService struct {
	settingService *SettingService

	mu          sync.RWMutex
	cachedRules []OffPeakPricingRule
	cacheTime   time.Time
	enabled     atomic.Bool
	enabledTime time.Time
}

const offPeakCacheTTL = 30 * time.Second

// NewOffPeakPricingService creates an OffPeakPricingService
func NewOffPeakPricingService(settingService *SettingService) *OffPeakPricingService {
	return &OffPeakPricingService{
		settingService: settingService,
	}
}

// ApplyMultiplier 根据当前时间和分组判断是否命中分时规则，返回调整后的 multiplier。
// groupID 为 nil 表示无分组（仅匹配 group_ids 为空的规则）。
// 如果未开启或未命中规则，返回原始 multiplier 不变。
func (s *OffPeakPricingService) ApplyMultiplier(ctx context.Context, baseMultiplier float64, groupID *int64) float64 {
	if !s.isEnabled(ctx) {
		return baseMultiplier
	}

	rules := s.getRules(ctx)
	if len(rules) == 0 {
		return baseMultiplier
	}

	now := time.Now()
	hour := now.Hour()

	for _, rule := range rules {
		if rule.Multiplier <= 0 {
			continue
		}
		if !matchHourRange(hour, rule.StartHour, rule.EndHour) {
			continue
		}
		if !ruleMatchesGroup(rule.GroupIDs, groupID) {
			continue
		}
		return baseMultiplier * rule.Multiplier
	}

	return baseMultiplier
}

// GetCurrentRule 获取当前生效的规则（供前端展示用）
func (s *OffPeakPricingService) GetCurrentRule(ctx context.Context, groupID *int64) *OffPeakPricingRule {
	if !s.isEnabled(ctx) {
		return nil
	}

	rules := s.getRules(ctx)
	hour := time.Now().Hour()

	for _, rule := range rules {
		if rule.Multiplier <= 0 {
			continue
		}
		if matchHourRange(hour, rule.StartHour, rule.EndHour) && ruleMatchesGroup(rule.GroupIDs, groupID) {
			return &rule
		}
	}
	return nil
}

// ruleMatchesGroup 判断规则是否适用于指定分组。
// group_ids 为空 → 适用所有分组；非空 → groupID 必须在列表中。
func ruleMatchesGroup(ruleGroupIDs []int64, groupID *int64) bool {
	if len(ruleGroupIDs) == 0 {
		return true // 未指定分组 = 所有分组
	}
	if groupID == nil {
		return false // 规则限定了分组，但请求无分组
	}
	for _, id := range ruleGroupIDs {
		if id == *groupID {
			return true
		}
	}
	return false
}

// matchHourRange 判断 hour 是否在 [start, end) 范围内，支持跨天
func matchHourRange(hour, start, end int) bool {
	if start < end {
		// 同天：如 9→18
		return hour >= start && hour < end
	}
	// 跨天：如 22→7（即 22,23,0,1,2,3,4,5,6）
	return hour >= start || hour < end
}

func (s *OffPeakPricingService) isEnabled(ctx context.Context) bool {
	now := time.Now()
	if now.Sub(s.enabledTime) < offPeakCacheTTL {
		return s.enabled.Load()
	}

	val := s.settingService.GetBoolSetting(ctx, SettingKeyOffPeakPricingEnabled, false)
	s.enabled.Store(val)
	s.enabledTime = now
	return val
}

func (s *OffPeakPricingService) getRules(ctx context.Context) []OffPeakPricingRule {
	now := time.Now()

	s.mu.RLock()
	if now.Sub(s.cacheTime) < offPeakCacheTTL && s.cachedRules != nil {
		rules := s.cachedRules
		s.mu.RUnlock()
		return rules
	}
	s.mu.RUnlock()

	rulesJSON := s.settingService.GetStringSetting(ctx, SettingKeyOffPeakPricingRules, "[]")
	var rules []OffPeakPricingRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		slog.Error("off_peak_pricing: parse rules failed", "error", err)
		return nil
	}

	s.mu.Lock()
	s.cachedRules = rules
	s.cacheTime = now
	s.mu.Unlock()

	return rules
}
