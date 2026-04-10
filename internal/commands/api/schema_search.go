package api

import (
	"encoding/json"
	"sort"
	"strings"
)

const maxSchemaSearchResults = 10

type schemaSearchResult struct {
	Operation   schemaOperation
	Confidence  float64
	ResourceKey string
}

type hybridSchemaSearcher struct{}

func (s hybridSchemaSearcher) Search(query string, operations []schemaOperation, limit int) ([]schemaSearchResult, error) {
	candidateLimit := limit * 3
	if candidateLimit < 10 {
		candidateLimit = 10
	}
	if candidateLimit > len(operations) {
		candidateLimit = len(operations)
	}

	results, err := blugeRetrieve(query, operations, candidateLimit)
	if err != nil {
		return nil, err
	}
	return filterSchemaResultsByResource(parseSchemaSearchIntent(query), results, limit), nil
}

func filterSchemaResultsByResource(intent schemaSearchIntent, results []schemaSearchResult, limit int) []schemaSearchResult {
	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}

	for i := range results {
		results[i].ResourceKey = resourceKeyForOperation(intent.Resources, results[i].Operation)
	}

	hasResource := false
	for _, result := range results {
		if result.ResourceKey != "" {
			hasResource = true
			break
		}
	}

	filtered := make([]schemaSearchResult, 0, len(results))
	for _, result := range results {
		if hasResource && result.ResourceKey == "" {
			continue
		}
		filtered = append(filtered, result)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Confidence != filtered[j].Confidence {
			return filtered[i].Confidence > filtered[j].Confidence
		}
		if filtered[i].Operation.OperationID != filtered[j].Operation.OperationID {
			return filtered[i].Operation.OperationID < filtered[j].Operation.OperationID
		}
		if filtered[i].Operation.Method != filtered[j].Operation.Method {
			return filtered[i].Operation.Method < filtered[j].Operation.Method
		}
		return filtered[i].Operation.Path < filtered[j].Operation.Path
	})

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

func resourceKeyForOperation(resources []string, operation schemaOperation) string {
	operationSet := tokenSet(normalizeTokens(splitCamelCase(operation.OperationID)))
	pathSet := tokenSet(normalizeTokens(tokenize(operation.Path)))
	tagSet := tokenSet(normalizeTokens(tokenize(strings.Join(operation.Tags, " "))))
	resource := matchedSchemaResource(resources, operationSet, pathSet, tagSet)
	if resource == "" {
		return ""
	}
	if len(resources) == 1 && isNestedSubresourceOperation(resource, operation) {
		return ""
	}
	return resource
}

func schemaSearchJSONAPIResponse(results []schemaSearchResult) ([]byte, error) {
	data := make([]map[string]any, 0, len(results))
	for _, result := range results {
		data = append(data, map[string]any{
			"attributes": map[string]any{
				"operation-id": result.Operation.OperationID,
				"method":       result.Operation.Method,
				"path":         result.Operation.Path,
				"summary":      result.Operation.Summary,
			},
		})
	}

	payload := map[string]any{"data": data}
	return json.Marshal(payload)
}
