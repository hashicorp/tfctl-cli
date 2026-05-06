// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package api

import (
	"context"
	"strconv"
	"strings"

	"github.com/blugelabs/bluge"

	"github.com/hashicorp/tfcloud/internal/pkg/openapi"
)

func blugeRetrieve(ctx context.Context, query string, operations []*openapi.Operation, limit int) ([]schemaSearchResult, error) {
	if limit <= 0 || limit > len(operations) {
		limit = len(operations)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	writer, err := bluge.OpenWriter(bluge.InMemoryOnlyConfig())
	if err != nil {
		return nil, err
	}
	defer writer.Close()

	byID := make(map[string]*openapi.Operation, len(operations))
	for i, operation := range operations {
		id := strconv.Itoa(i)
		byID[id] = operation
		if err := writer.Update(bluge.Identifier(id), blugeDocumentForOperation(id, operation)); err != nil {
			return nil, err
		}
	}

	reader, err := writer.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	searchQuery := blugeQueryForIntent(parseSchemaSearchIntent(query))
	request := bluge.NewTopNSearch(limit, searchQuery)
	iterator, err := reader.Search(ctx, request)
	if err != nil {
		return nil, err
	}

	results := make([]schemaSearchResult, 0, limit)
	match, err := iterator.Next()
	for err == nil && match != nil {
		id := ""
		err = match.VisitStoredFields(func(field string, value []byte) bool {
			if field == "_id" {
				id = string(value)
			}
			return true
		})
		if err != nil {
			return nil, err
		}
		operation, ok := byID[id]
		if ok {
			results = append(results, schemaSearchResult{
				Operation:  operation,
				Confidence: match.Score,
			})
		}
		match, err = iterator.Next()
	}
	if err != nil {
		return nil, err
	}
	return results, nil
}

func blugeDocumentForOperation(id string, operation *openapi.Operation) *bluge.Document {
	tokens := strings.Join(normalizeTokens(splitCamelCase(operation.OperationID)), " ")
	pathTokens := strings.Join(normalizeTokens(tokenize(operation.Path)), " ")
	tagTokens := strings.Join(normalizeTokens(tokenize(strings.Join(operation.Tags, " "))), " ")
	doc := bluge.NewDocument(id).
		AddField(bluge.NewTextField("operation_id", operation.OperationID).StoreValue()).
		AddField(bluge.NewTextField("operation_tokens", tokens)).
		AddField(bluge.NewTextField("summary", operation.Summary)).
		AddField(bluge.NewTextField("path_tokens", pathTokens)).
		AddField(bluge.NewTextField("tag_tokens", tagTokens)).
		AddField(bluge.NewKeywordField("method", strings.ToUpper(operation.Method))).
		AddField(bluge.NewStoredOnlyField("path", []byte(operation.Path)))
	return doc
}

func blugeQueryForIntent(intent schemaSearchIntent) bluge.Query {
	clauses := make([]bluge.Query, 0, len(intent.Tokens)+4)
	for _, token := range intent.Tokens {
		clauses = append(clauses,
			bluge.NewMatchQuery(token).SetField("operation_id"),
			bluge.NewMatchQuery(token).SetField("operation_tokens"),
			bluge.NewMatchQuery(token).SetField("summary"),
			bluge.NewMatchQuery(token).SetField("path_tokens"),
			bluge.NewMatchQuery(token).SetField("tag_tokens"),
		)
	}

	if intent.Verb != "" {
		if method := preferredMethodForVerb(intent.Verb); method != "" {
			clauses = append(clauses, bluge.NewTermQuery(method).SetField("method"))
		}
	}

	if len(clauses) == 0 {
		return bluge.NewMatchAllQuery()
	}

	query := bluge.NewBooleanQuery()
	for _, clause := range clauses {
		query.AddShould(clause)
	}
	query.SetMinShould(1)
	return query
}

func preferredMethodForVerb(verb string) string {
	switch verb {
	case "create", "cancel":
		return "POST"
	case "get", "list":
		return "GET"
	case "delete":
		return "DELETE"
	case "update":
		return "PATCH"
	default:
		return ""
	}
}
