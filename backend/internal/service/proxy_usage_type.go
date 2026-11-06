package service

import infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

const (
	ProxyUsageGeneral = "general"
	ProxyUsageKiro    = "kiro"
	ProxyUsagePro     = "pro"
)

var ErrProxyUsageTypeInvalid = infraerrors.BadRequest("PROXY_USAGE_TYPE_INVALID", "proxy usage type must be general, kiro or pro")

func NormalizeProxyUsageType(value string) string {
	if value == "" {
		return ProxyUsageGeneral
	}
	return value
}

func ValidProxyUsageType(value string) bool {
	return value == ProxyUsageGeneral || value == ProxyUsageKiro || value == ProxyUsagePro
}
