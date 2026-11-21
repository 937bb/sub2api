package service

import "context"

// BindStickySessionAfterSelection preserves temporary capacity spillover when
// admission happens after selection, including after an account queue wait.
func (s *OpenAIGatewayService) BindStickySessionAfterSelection(ctx context.Context, groupID *int64, sessionHash string, selection *AccountSelectionResult) error {
	if selection == nil || selection.Account == nil || selection.preserveStickyBinding {
		return nil
	}
	return s.BindStickySessionAfterProfitAdmission(ctx, groupID, sessionHash, selection.Account.ID)
}
