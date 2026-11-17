package mcpserver

import (
	"encoding/json"
	"math"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/domain"
)

// toInt converts a decoded JSON number to an int, rejecting fractions.
func toInt(key string, v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return floatToInt(key, n)
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, domain.Invalid("%s must be a whole number, got %s", key, n)
		}
		return int(i), nil
	default:
		return 0, domain.Invalid("%s must be a number", key)
	}
}

func floatToInt(key string, n float64) (int, error) {
	if n != math.Trunc(n) || math.Abs(n) > math.MaxInt32 {
		return 0, domain.Invalid("%s must be a whole number, got %v", key, n)
	}
	return int(n), nil
}

func reqInt(req mcp.CallToolRequest, key string) (int, error) {
	v, ok := req.GetArguments()[key]
	if !ok || v == nil {
		return 0, domain.Invalid("%s is required", key)
	}
	return toInt(key, v)
}

func optInt(req mcp.CallToolRequest, key string) (*int, error) {
	v, ok := req.GetArguments()[key]
	if !ok || v == nil {
		return nil, nil //nolint:nilnil // (nil, nil) means "argument not supplied"; callers treat it as no filter
	}
	n, err := toInt(key, v)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func optBool(req mcp.CallToolRequest, key string) (*bool, error) {
	v, ok := req.GetArguments()[key]
	if !ok || v == nil {
		return nil, nil //nolint:nilnil // (nil, nil) means "argument not supplied"; callers treat it as no filter
	}
	b, isBool := v.(bool)
	if !isBool {
		return nil, domain.Invalid("%s must be true or false", key)
	}
	return &b, nil
}

// listOptions reads the shared page/limit arguments. Zero means "default".
func listOptions(req mcp.CallToolRequest) (domain.ListOptions, error) {
	var o domain.ListOptions
	if p, err := optInt(req, "page"); err != nil {
		return o, err
	} else if p != nil {
		o.Page = *p
	}
	if l, err := optInt(req, "limit"); err != nil {
		return o, err
	} else if l != nil {
		o.Limit = *l
	}
	return o, nil
}
