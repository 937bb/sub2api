package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKiroOpenAIGroupCacheReachesBilling(t *testing.T) {
	for _, protocol := range []string{"messages", "chat", "responses"} {
		for _, stream := range []bool{false, true} {
			for _, ttl := range []int{300, 3600} {
				t.Run(fmt.Sprintf("%s/stream=%v/ttl=%d", protocol, stream, ttl), func(t *testing.T) {
					groupID := int64(7)
					up := &kiroCompatUpstream{t: t}
					cache := &kiroMemoryCache{values: map[string][]byte{}}
					gateway := &GatewayService{httpUpstream: up, cfg: &config.Config{}, cache: cache, groupRepo: &kiroGroupRepo{group: &Group{ID: groupID, KiroResponseCache: KiroCachePolicy{Enabled: true, TargetPercent: 100, TTLSeconds: ttl, MaxEntries: 128}}}}
					account := fixedKiroAccount()
					account.Credentials["access_token"] = t.Name()
					account.Credentials["expires_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
					for _, hit := range []bool{false, true} {
						raw := []byte(fmt.Sprintf(`{"model":"claude-haiku-4.5","max_tokens":1024,"stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream))
						if protocol == "responses" {
							raw = []byte(fmt.Sprintf(`{"model":"claude-haiku-4.5","stream":%t,"input":"hi"}`, stream))
						}
						recorder := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(recorder)
						c.Request = httptest.NewRequest("POST", "/test", strings.NewReader(string(raw)))
						c.Set("api_key", &APIKey{ID: 1, UserID: 2, GroupID: &groupID})
						logs := &openAIRecordUsageLogRepoStub{inserted: true}
						users := &openAIRecordUsageUserRepoStub{}
						svc := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, nil)
						svc.kiroGateway = gateway
						result, err := svc.forwardKiroCompatible(context.Background(), c, account, raw, protocol)
						require.NoError(t, err)
						require.Equal(t, 11, result.Usage.InputTokens)
						require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{Result: result, APIKey: &APIKey{ID: 1}, User: &User{ID: 2}, Account: account}))
						row := logs.lastLog
						require.NotNil(t, row)
						require.Zero(t, row.InputTokens)
						require.Equal(t, 1, users.deductCalls)
						require.InDelta(t, row.ActualCost, users.lastAmount, 1e-12)
						require.Positive(t, row.ActualCost)
						if hit {
							require.Equal(t, 11, row.CacheReadTokens)
							require.Zero(t, row.CacheCreationTokens)
							require.Zero(t, row.OutputTokens)
							require.Positive(t, row.CacheReadCost)
						} else {
							require.Equal(t, 11, row.CacheCreationTokens)
							require.Zero(t, row.CacheReadTokens)
							require.Equal(t, 2, row.OutputTokens)
							require.Positive(t, row.CacheCreationCost)
							if ttl == 300 {
								require.Equal(t, 11, row.CacheCreation5mTokens)
							} else {
								require.Equal(t, 11, row.CacheCreation1hTokens)
							}
						}
					}
				})
			}
		}
	}
}
