package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type turnStateAdminRepoMock struct {
	*opsRepoMock
	batchRecords  []*OpenAICodexTurnStateRecord
	deleteIDs     []int64
	deleteHashes  []string
	listResult    *OpenAICodexTurnStateList
	summaryResult *OpenAICodexTurnStateSummary
}

func newTurnStateAdminRepoMock() *turnStateAdminRepoMock {
	return &turnStateAdminRepoMock{opsRepoMock: &opsRepoMock{}}
}

func (m *turnStateAdminRepoMock) UpsertOpenAICodexTurnState(context.Context, *OpenAICodexTurnStateRecord) error {
	return nil
}

func (m *turnStateAdminRepoMock) LoadActiveOpenAICodexTurnStates(context.Context, time.Time) ([]*OpenAICodexTurnStateRecord, error) {
	return nil, nil
}

func (m *turnStateAdminRepoMock) BatchUpsertOpenAICodexTurnStates(_ context.Context, records []*OpenAICodexTurnStateRecord) error {
	m.batchRecords = records
	return nil
}

func (m *turnStateAdminRepoMock) DeleteOpenAICodexTurnStates(_ context.Context, ids []int64) ([]string, error) {
	m.deleteIDs = append([]int64(nil), ids...)
	return append([]string(nil), m.deleteHashes...), nil
}

func (m *turnStateAdminRepoMock) ListOpenAICodexTurnStates(context.Context, *OpenAICodexTurnStateFilter) (*OpenAICodexTurnStateList, error) {
	return m.listResult, nil
}

func (m *turnStateAdminRepoMock) GetOpenAICodexTurnStateSummary(context.Context, time.Time) (*OpenAICodexTurnStateSummary, error) {
	return m.summaryResult, nil
}

func TestOpsServiceAddOpenAICodexTurnStates_DeduplicatesAndUpdatesPool(t *testing.T) {
	repo := newTurnStateAdminRepoMock()
	gateway := &OpenAIGatewayService{}
	svc := &OpsService{opsRepo: repo, openAIGatewayService: gateway}
	issuedAt := time.Now().UTC().Truncate(time.Second)
	short := testOpenAICodexTurnState(292, issuedAt, 'a')
	longest := testOpenAICodexTurnState(312, issuedAt, 'b')

	added, err := svc.AddOpenAICodexTurnStates(context.Background(), []string{
		" " + short + " ",
		longest,
		short,
		"",
	})
	require.NoError(t, err)
	require.Equal(t, 2, added)
	require.Len(t, repo.batchRecords, 2)

	_, reusable := gateway.getOpenAICodexTurnStatePool().preferredForBucket(77, "gpt-5.5")
	require.False(t, reusable)
	require.False(t, gateway.getOpenAICodexTurnStatePool().hasSampledAccount(77))
	for _, record := range repo.batchRecords {
		require.Equal(t, "manual", record.SourceTransport)
		require.Nil(t, record.SourceAccountID)
		require.Equal(t, issuedAt, record.IssuedAt)
		require.Equal(t, issuedAt.Add(openAICodexTurnStateTTL), record.ExpiresAt)
	}
}

func TestOpsServiceAddOpenAICodexTurnStates_RejectsInvalidHeaderValue(t *testing.T) {
	repo := newTurnStateAdminRepoMock()
	svc := &OpsService{opsRepo: repo}

	added, err := svc.AddOpenAICodexTurnStates(context.Background(), []string{"valid-state", "invalid\r\nvalue"})
	require.ErrorContains(t, err, "invalid header")
	require.Zero(t, added)
	require.Empty(t, repo.batchRecords)
}

func TestOpsServiceDeleteOpenAICodexTurnStates_EvictsRuntimeSelection(t *testing.T) {
	repo := newTurnStateAdminRepoMock()
	gateway := &OpenAIGatewayService{}
	svc := &OpsService{opsRepo: repo, openAIGatewayService: gateway}
	pool := gateway.getOpenAICodexTurnStatePool()
	issuedAt := time.Now().UTC().Truncate(time.Second)
	fallback := testOpenAICodexTurnState(292, issuedAt, 'a')
	longest := testOpenAICodexTurnState(312, issuedAt, 'b')
	pool.observe(fallback, nil, "", "", "http")
	pool.observe(longest, nil, "", "", "http")
	repo.deleteHashes = []string{hashOpenAICodexTurnState(longest)}

	deleted, err := svc.DeleteOpenAICodexTurnStates(context.Background(), []int64{0, 7, 7, -1})
	require.NoError(t, err)
	require.Equal(t, 1, deleted)
	require.Equal(t, []int64{7}, repo.deleteIDs)

	require.Len(t, pool.entries, 1)
	for _, record := range pool.entries {
		require.Equal(t, fallback, record.StateValue)
	}
}

func TestOpsServiceCodexTurnStateReads_RedactRawValues(t *testing.T) {
	repo := newTurnStateAdminRepoMock()
	repo.listResult = &OpenAICodexTurnStateList{
		Items: []*OpenAICodexTurnStateRecord{{StateValue: "sensitive-list-value"}},
		Total: 1,
	}
	repo.summaryResult = &OpenAICodexTurnStateSummary{
		LongestActive: &OpenAICodexTurnStateRecord{StateValue: "sensitive-summary-value"},
	}
	svc := &OpsService{opsRepo: repo}

	list, err := svc.ListOpenAICodexTurnStates(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, list.Items[0].StateValue)
	require.NotEmpty(t, list.Items[0].MaskedValue)

	summary, err := svc.GetOpenAICodexTurnStateSummary(context.Background())
	require.NoError(t, err)
	require.Empty(t, summary.LongestActive.StateValue)
	require.NotEmpty(t, summary.LongestActive.MaskedValue)
	require.Equal(t, int64(openAICodexTurnStateTTL/time.Second), summary.ReuseTTLSeconds)
}
