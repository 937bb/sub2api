package repository

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *accountRepository) GetOpenAIConversationBinding(ctx context.Context, key string) (*service.OpenAIConversationBinding, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT account_id, credential_account_id, identity_hash FROM openai_conversation_bindings WHERE binding_key = $1`, key)
	if err != nil {
		return nil, fmt.Errorf("read conversation binding: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	b := &service.OpenAIConversationBinding{}
	if err := rows.Scan(&b.AccountID, &b.CredentialAccountID, &b.IdentityHash); err != nil {
		return nil, err
	}
	return b, rows.Err()
}

// The unique key elects one owner across HTTP/WS processes. The subsequent SELECT
// has a fresh READ COMMITTED snapshot, including a concurrent winning insert.
func (r *accountRepository) ClaimOpenAIConversationBinding(ctx context.Context, key string, b *service.OpenAIConversationBinding) (*service.OpenAIConversationBinding, error) {
	_, err := r.sql.ExecContext(ctx, `INSERT INTO openai_conversation_bindings (binding_key, account_id, credential_account_id, identity_hash) VALUES ($1, $2, $3, $4) ON CONFLICT (binding_key) DO NOTHING`, key, b.AccountID, b.CredentialAccountID, b.IdentityHash)
	if err != nil {
		return nil, fmt.Errorf("claim conversation binding: %w", err)
	}
	return r.GetOpenAIConversationBinding(ctx, key)
}
