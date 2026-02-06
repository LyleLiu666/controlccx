package providers

import (
	"errors"
	"strings"

	"controlccx/internal/auth"
)

func (s *Store) Activate(target string, profileID string, authStore *auth.Store) (Profile, error) {
	if s == nil {
		return Profile{}, errors.New("providers: store is nil")
	}
	target = strings.ToLower(strings.TrimSpace(target))
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return Profile{}, errors.New("providers: activate: profile id is required")
	}

	p, ok := s.Get(profileID)
	if !ok {
		return Profile{}, errors.New("providers: activate: profile not found")
	}

	switch target {
	case "claude":
		if authStore == nil {
			return Profile{}, errors.New("providers: activate: auth store is required")
		}
		baseURL := p.Targets.Claude.BaseURL
		apiKey := p.Targets.Claude.APIKey
		authToken := p.Targets.Claude.AuthToken
		model := p.Targets.Claude.Model
		smallFastModel := p.Targets.Claude.SmallFastModel
		if _, err := authStore.ApplyPatch(auth.Patch{
			AnthropicBaseURL:        &baseURL,
			AnthropicAPIKey:         &apiKey,
			AnthropicAuthToken:      &authToken,
			AnthropicModel:          &model,
			AnthropicSmallFastModel: &smallFastModel,
		}); err != nil {
			return Profile{}, err
		}
		if err := s.SetActive("claude", profileID); err != nil {
			return Profile{}, err
		}
		return p, nil
	case "codex":
		if authStore == nil {
			return Profile{}, errors.New("providers: activate: auth store is required")
		}
		apiKey := p.Targets.Codex.APIKey
		model := p.Targets.Codex.Model
		effort := p.Targets.Codex.ReasoningEffort
		if _, err := authStore.ApplyPatch(auth.Patch{
			OpenAIAPIKey:         &apiKey,
			CodexModel:           &model,
			CodexReasoningEffort: &effort,
		}); err != nil {
			return Profile{}, err
		}
		if err := s.SetActive("codex", profileID); err != nil {
			return Profile{}, err
		}
		return p, nil
	case "secretary":
		if err := s.SetActive("secretary", profileID); err != nil {
			return Profile{}, err
		}
		return p, nil
	default:
		return Profile{}, errors.New("providers: activate: unknown target")
	}
}
