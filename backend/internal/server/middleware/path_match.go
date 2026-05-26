package middleware

import "strings"

func normalizeRequestPath(path string) string {
	trimmed := strings.Trim(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return "/"
	}
	return "/" + trimmed
}

func requestPathHasMountedPrefix(rawPath, rawPrefix string) bool {
	path := normalizeRequestPath(rawPath)
	prefix := normalizeRequestPath(rawPrefix)
	if prefix == "/" {
		return true
	}
	pathSegments := strings.Split(strings.Trim(path, "/"), "/")
	prefixSegments := strings.Split(strings.Trim(prefix, "/"), "/")
	if len(prefixSegments) > len(pathSegments) {
		return false
	}

	for start := 0; start <= len(pathSegments)-len(prefixSegments); start++ {
		matches := true
		for offset, segment := range prefixSegments {
			if pathSegments[start+offset] != segment {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func requestPathEqualsMounted(rawPath, rawEndpoint string) bool {
	path := normalizeRequestPath(rawPath)
	endpoint := normalizeRequestPath(rawEndpoint)
	pathSegments := strings.Split(strings.Trim(path, "/"), "/")
	endpointSegments := strings.Split(strings.Trim(endpoint, "/"), "/")
	if len(endpointSegments) > len(pathSegments) {
		return false
	}
	offset := len(pathSegments) - len(endpointSegments)
	for index, segment := range endpointSegments {
		if pathSegments[offset+index] != segment {
			return false
		}
	}
	return true
}
