package api

import (
	"strings"
	"unicode"
)

type schemaSearchIntent struct {
	Tokens    []string
	Verb      string
	Resources []string
}

func matchedSchemaResource(resources []string, operationSet, pathSet, tagSet map[string]struct{}) string {
	if len(resources) == 0 {
		return ""
	}

	for _, resource := range resources {
		if resource == "" {
			continue
		}
		if _, ok := tagSet[resource]; ok {
			return resource
		}
	}
	for _, resource := range resources {
		if resource == "" {
			continue
		}
		if _, ok := pathSet[resource]; ok {
			return resource
		}
	}
	for _, resource := range resources {
		if resource == "" {
			continue
		}
		if _, ok := operationSet[resource]; ok {
			return resource
		}
	}
	return ""
}

func parseSchemaSearchIntent(query string) schemaSearchIntent {
	tokens := normalizeTokens(tokenize(query))
	resources := make([]string, 0, len(tokens))
	verb := ""

	for _, token := range tokens {
		if canonical := canonicalVerb(token); canonical != "" && verb == "" {
			verb = canonical
			continue
		}
		resources = append(resources, token)
	}

	if verb == "" && len(resources) > 0 {
		verb = "get"
	}

	return schemaSearchIntent{
		Tokens:    tokens,
		Verb:      verb,
		Resources: resources,
	}
}

func isNestedSubresourceOperation(resource string, operation schemaOperation) bool {
	if resource == "" {
		return false
	}
	segments := pathSegments(operation.Path)
	for i := 0; i < len(segments)-2; i++ {
		if normalizeToken(segments[i]) != resource || !isPathParameter(segments[i+1]) {
			continue
		}
		if !isPathParameter(segments[i+2]) {
			return true
		}
	}
	return false
}

func pathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func isPathParameter(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

func tokenize(input string) []string {
	fields := strings.FieldsFunc(strings.ToLower(input), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		if field != "" {
			tokens = append(tokens, field)
		}
	}
	return tokens
}

func splitCamelCase(input string) []string {
	if input == "" {
		return nil
	}

	var tokens []string
	var current []rune
	for i, r := range input {
		if i > 0 && unicode.IsUpper(r) && len(current) > 0 {
			tokens = append(tokens, strings.ToLower(string(current)))
			current = current[:0]
		}
		current = append(current, r)
	}
	if len(current) > 0 {
		tokens = append(tokens, strings.ToLower(string(current)))
	}
	for i, token := range tokens {
		tokens[i] = splitOperationToken(token)
	}
	return tokens
}

func splitOperationToken(token string) string {
	switch token {
	case "var", "vars":
		return "variable"
	default:
		return token
	}
}

func normalizeTokens(tokens []string) []string {
	normalized := make([]string, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		normalizedToken := normalizeToken(token)
		if normalizedToken == "" {
			continue
		}
		if _, ok := seen[normalizedToken]; ok {
			continue
		}
		seen[normalizedToken] = struct{}{}
		normalized = append(normalized, normalizedToken)
	}
	return normalized
}

func normalizeToken(token string) string {
	if token == "" {
		return ""
	}

	switch token {
	case "a", "an", "the", "to", "for", "of", "in", "on", "with", "me", "please":
		return ""
	case "add", "new":
		return "create"
	case "show", "fetch", "read", "find":
		return "get"
	case "all", "browse", "collection", "index":
		return "list"
	case "remove", "destroy":
		return "delete"
	case "edit", "set", "modify":
		return "update"
	case "stop", "abort":
		return "cancel"
	case "vars", "var", "variables":
		return "variable"
	}

	if strings.HasSuffix(token, "ies") && len(token) > 3 {
		return token[:len(token)-3] + "y"
	}
	if strings.HasSuffix(token, "s") && len(token) > 3 && !strings.HasSuffix(token, "ss") {
		return token[:len(token)-1]
	}
	return token
}

func canonicalVerb(token string) string {
	switch token {
	case "create", "get", "list", "delete", "cancel", "update":
		return token
	default:
		return ""
	}
}

func tokenSet(tokens []string) map[string]struct{} {
	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		set[token] = struct{}{}
	}
	return set
}
