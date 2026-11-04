package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestConversationBindingRepositoryKeepsConcurrentWinner(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountRepository{sql: db}
	candidate := &service.OpenAIConversationBinding{AccountID: 2, CredentialAccountID: 2, IdentityHash: "second"}
	mock.ExpectExec("INSERT INTO openai_conversation_bindings").WithArgs("key", int64(2), int64(2), "second").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT account_id, credential_account_id, identity_hash").WithArgs("key").WillReturnRows(sqlmock.NewRows([]string{"account_id", "credential_account_id", "identity_hash"}).AddRow(1, 1, "first"))
	winner, err := repo.ClaimOpenAIConversationBinding(context.Background(), "key", candidate)
	require.NoError(t, err)
	require.Equal(t, int64(1), winner.AccountID)
	require.Equal(t, "first", winner.IdentityHash)
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestConversationBindingRepositoryDoesNotTreatReadFailureAsMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountRepository{sql: db}
	mock.ExpectQuery("SELECT account_id").WillReturnError(errors.New("connection lost"))
	b, err := repo.GetOpenAIConversationBinding(context.Background(), "key")
	require.Nil(t, b)
	require.ErrorContains(t, err, "connection lost")
	mock.ExpectQuery("SELECT account_id").WillReturnRows(sqlmock.NewRows([]string{"account_id", "credential_account_id", "identity_hash"}))
	b, err = repo.GetOpenAIConversationBinding(context.Background(), "key")
	require.Nil(t, b)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
