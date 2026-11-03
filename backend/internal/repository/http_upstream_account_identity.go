package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// OpenAI account identities must not share an HTTP/TLS connection just because
// they use the same outbound proxy. Keep configured isolation for other profiles
// and for requests without a selected account.
func (s *httpUpstreamService) accountProfileIsolation(profile service.HTTPUpstreamProfile, accountID int64) string {
	isolation := s.getIsolationMode()
	if profile == service.HTTPUpstreamProfileOpenAI && accountID > 0 && isolation == config.ConnectionPoolIsolationProxy {
		return config.ConnectionPoolIsolationAccountProxy
	}
	return isolation
}
