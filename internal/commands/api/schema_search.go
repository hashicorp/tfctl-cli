// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package api

import (
	"context"
	"sort"
	"strings"

	"github.com/hashicorp/tfcloud/internal/pkg/openapi"
)

const maxSchemaSearchResults = 10

type schemaSearchResult struct {
	Operation   *openapi.Operation
	Confidence  float64
	ResourceKey string
}

type hybridSchemaSearcher struct{}

func (s hybridSchemaSearcher) Search(ctx context.Context, query string, operations []*openapi.Operation, limit int) ([]schemaSearchResult, error) {
	candidateLimit := limit * 3
	if candidateLimit < 10 {
		candidateLimit = 10
	}
	if candidateLimit > len(operations) {
		candidateLimit = len(operations)
	}

	results, err := blugeRetrieve(ctx, query, operations, candidateLimit)
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

func resourceKeyForOperation(resources []string, operation *openapi.Operation) string {
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
