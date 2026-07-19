package service

import "strings"

// resolveOpenAIForwardModel resolves account-scoped model mappings for ordinary
// OpenAI forwarding paths.
func resolveOpenAIForwardModel(account *Account, requestedModel string) string {
	if account == nil {
		return requestedModel
	}

	mappedModel, _ := account.ResolveMappedModel(requestedModel)
	return mappedModel
}

// resolveOpenAIMessagesForwardModel additionally applies the exact/group
// dispatch selected for the /v1/messages caller.
func resolveOpenAIMessagesForwardModel(account *Account, requestedModel, dispatchMappedModel string) string {
	dispatchMappedModel = strings.TrimSpace(dispatchMappedModel)
	if account != nil {
		if mappedModel, matched := account.ResolveMappedModel(requestedModel); matched {
			return mappedModel
		}
	}
	if dispatchMappedModel != "" {
		return dispatchMappedModel
	}
	return requestedModel
}

// resolveOpenAICompactForwardModel determines the compact-only upstream model
// for /responses/compact requests. It never affects normal /responses traffic.
// When no compact-specific mapping matches, the input model is returned as-is.
func resolveOpenAICompactForwardModel(account *Account, model string) string {
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" || account == nil {
		return trimmedModel
	}

	mappedModel, matched := account.ResolveCompactMappedModel(trimmedModel)
	if !matched {
		return trimmedModel
	}
	if trimmedMapped := strings.TrimSpace(mappedModel); trimmedMapped != "" {
		return trimmedMapped
	}
	return trimmedModel
}
