package skills

import (
	"net/url"
	"strings"
)

type repoMetadata struct {
	Key   string
	Label string
	Ref   string
	Valid bool
}

func deriveRepoMetadataFromManifest(m ManagedSkillManifest) repoMetadata {
	if strings.TrimSpace(m.SourceType) != sourceTypeGit {
		return repoMetadata{}
	}
	sourceRef := strings.TrimSpace(m.SourceRef)
	if sourceRef == "" {
		return repoMetadata{}
	}

	ref := canonicalRepoRef(sourceRef)
	if ref == "" {
		return repoMetadata{}
	}

	key, label := deriveRepoKeyAndLabel(ref)
	if key == "" {
		return repoMetadata{}
	}
	if label == "" {
		label = key
	}

	return repoMetadata{
		Key:   key,
		Label: label,
		Ref:   ref,
		Valid: true,
	}
}

func canonicalRepoRef(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if parsed := parseGitSource(trimmed); strings.TrimSpace(parsed.cloneURL) != "" {
		trimmed = strings.TrimSpace(parsed.cloneURL)
	}
	if u, err := url.Parse(trimmed); err == nil && strings.TrimSpace(u.Scheme) != "" && strings.TrimSpace(u.Host) != "" {
		u.RawQuery = ""
		u.Fragment = ""
		return strings.TrimSpace(u.String())
	}
	return trimmed
}

func deriveRepoKeyAndLabel(ref string) (string, string) {
	if owner, repo, ok := parseGitHubOwnerRepo(ref); ok {
		ownerLower := strings.ToLower(strings.TrimSpace(owner))
		repoLower := strings.ToLower(strings.TrimSpace(repo))
		if ownerLower == "" || repoLower == "" {
			return "", ""
		}
		return "github.com/" + ownerLower + "/" + repoLower, ownerLower + "/" + repoLower
	}

	if host, repoPath, ok := parseSCPLikeRepo(ref); ok {
		hostLower := strings.ToLower(strings.TrimSpace(host))
		repoPathLower := strings.ToLower(strings.TrimSpace(repoPath))
		if hostLower != "" && repoPathLower != "" {
			return hostLower + "/" + repoPathLower, pathLabel(repoPathLower)
		}
	}

	if u, err := url.Parse(ref); err == nil && strings.TrimSpace(u.Host) != "" {
		host := strings.ToLower(strings.TrimSpace(u.Host))
		p := strings.Trim(strings.TrimSpace(u.Path), "/")
		p = strings.TrimSuffix(p, ".git")
		if p == "" {
			return host, host
		}
		key := host + "/" + strings.ToLower(p)
		label := pathLabel(p)
		if label == "" {
			label = host + "/" + p
		}
		return key, label
	}

	raw := strings.TrimSpace(ref)
	if raw == "" {
		return "", ""
	}
	label := pathLabel(raw)
	if label == "" {
		label = raw
	}
	return raw, label
}

func parseGitHubOwnerRepo(input string) (owner string, repo string, ok bool) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(input, "/"))
	if trimmed == "" {
		return "", "", false
	}

	rest := ""
	switch {
	case strings.HasPrefix(trimmed, "https://github.com/"):
		rest = strings.TrimPrefix(trimmed, "https://github.com/")
	case strings.HasPrefix(trimmed, "http://github.com/"):
		rest = strings.TrimPrefix(trimmed, "http://github.com/")
	case strings.HasPrefix(trimmed, "github.com/"):
		rest = strings.TrimPrefix(trimmed, "github.com/")
	case strings.HasPrefix(trimmed, "git@github.com:"):
		rest = strings.TrimPrefix(trimmed, "git@github.com:")
	case strings.HasPrefix(trimmed, "ssh://git@github.com/"):
		rest = strings.TrimPrefix(trimmed, "ssh://git@github.com/")
	case looksLikeGitHubShorthand(trimmed):
		rest = trimmed
	default:
		return "", "", false
	}

	parts := strings.Split(strings.TrimPrefix(rest, "/"), "/")
	if len(parts) < 2 {
		return "", "", false
	}
	owner = strings.TrimSpace(parts[0])
	repo = strings.TrimSpace(strings.TrimSuffix(parts[1], ".git"))
	if owner == "" || repo == "" {
		return "", "", false
	}
	return owner, repo, true
}

func pathLabel(p string) string {
	norm := strings.Trim(strings.TrimSpace(strings.ReplaceAll(p, "\\", "/")), "/")
	if norm == "" {
		return ""
	}
	parts := strings.Split(norm, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return parts[0]
}

func parseSCPLikeRepo(input string) (host string, repoPath string, ok bool) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", "", false
	}
	if strings.Contains(s, "://") {
		return "", "", false
	}
	at := strings.LastIndex(s, "@")
	colon := strings.LastIndex(s, ":")
	if at < 0 || colon < 0 || colon <= at {
		return "", "", false
	}
	host = strings.TrimSpace(s[at+1 : colon])
	repoPath = strings.TrimSpace(s[colon+1:])
	repoPath = strings.Trim(strings.ReplaceAll(repoPath, "\\", "/"), "/")
	repoPath = strings.TrimSuffix(repoPath, ".git")
	if host == "" || repoPath == "" {
		return "", "", false
	}
	return host, repoPath, true
}
