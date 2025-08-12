package search

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// QueryParser provides SQL-like query parsing for advanced searches
type QueryParser struct {
	keywords map[string]bool
}

// ParsedQuery represents a parsed SQL-like query
type ParsedQuery struct {
	SelectFields []string                 `json:"select_fields"`
	FromType     string                   `json:"from_type"`     // "files", "commits", "content"
	WhereClause  *WhereClause             `json:"where_clause"`
	OrderBy      []OrderByClause          `json:"order_by"`
	GroupBy      []string                 `json:"group_by"`
	Having       *WhereClause             `json:"having"`
	Limit        int                      `json:"limit"`
	Offset       int                      `json:"offset"`
	Aggregations []AggregationFunction    `json:"aggregations"`
}

// WhereClause represents a WHERE condition
type WhereClause struct {
	Conditions []Condition  `json:"conditions"`
	Logic      string       `json:"logic"` // "AND" or "OR"
}

// Condition represents a single condition
type Condition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // "=", "!=", "LIKE", "IN", ">", "<", ">=", "<=", "CONTAINS"
	Value    interface{} `json:"value"`
	Not      bool        `json:"not"`
}

// OrderByClause represents an ORDER BY clause
type OrderByClause struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // "ASC" or "DESC"
}

// AggregationFunction represents aggregation functions
type AggregationFunction struct {
	Function string `json:"function"` // "COUNT", "SUM", "AVG", "MIN", "MAX"
	Field    string `json:"field"`
	Alias    string `json:"alias,omitempty"`
}

// QueryExecutionResult represents the result of executing a parsed query
type QueryExecutionResult struct {
	Rows         []map[string]interface{} `json:"rows"`
	Aggregations map[string]interface{}   `json:"aggregations,omitempty"`
	Total        int                      `json:"total"`
	QueryTime    time.Duration            `json:"query_time"`
	ExecutedSQL  string                   `json:"executed_sql"`
}

// NewQueryParser creates a new SQL query parser
func NewQueryParser() *QueryParser {
	return &QueryParser{
		keywords: map[string]bool{
			"SELECT": true, "FROM": true, "WHERE": true, "ORDER": true, "BY": true,
			"GROUP": true, "HAVING": true, "LIMIT": true, "OFFSET": true, "AND": true,
			"OR": true, "NOT": true, "LIKE": true, "IN": true, "AS": true,
			"COUNT": true, "SUM": true, "AVG": true, "MIN": true, "MAX": true,
			"ASC": true, "DESC": true, "CONTAINS": true,
		},
	}
}

// ParseQuery parses a SQL-like query string
func (qp *QueryParser) ParseQuery(query string) (*ParsedQuery, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}
	
	parsed := &ParsedQuery{
		SelectFields: []string{},
		OrderBy:      []OrderByClause{},
		GroupBy:      []string{},
		Aggregations: []AggregationFunction{},
	}
	
	// Tokenize the query
	tokens := qp.tokenize(query)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("no tokens found")
	}
	
	// Parse each clause
	i := 0
	for i < len(tokens) {
		token := strings.ToUpper(tokens[i])
		
		switch token {
		case "SELECT":
			var err error
			i, err = qp.parseSelect(tokens, i, parsed)
			if err != nil {
				return nil, err
			}
		case "FROM":
			var err error
			i, err = qp.parseFrom(tokens, i, parsed)
			if err != nil {
				return nil, err
			}
		case "WHERE":
			var err error
			i, err = qp.parseWhere(tokens, i, parsed)
			if err != nil {
				return nil, err
			}
		case "ORDER":
			var err error
			i, err = qp.parseOrderBy(tokens, i, parsed)
			if err != nil {
				return nil, err
			}
		case "GROUP":
			var err error
			i, err = qp.parseGroupBy(tokens, i, parsed)
			if err != nil {
				return nil, err
			}
		case "HAVING":
			var err error
			i, err = qp.parseHaving(tokens, i, parsed)
			if err != nil {
				return nil, err
			}
		case "LIMIT":
			var err error
			i, err = qp.parseLimit(tokens, i, parsed)
			if err != nil {
				return nil, err
			}
		case "OFFSET":
			var err error
			i, err = qp.parseOffset(tokens, i, parsed)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unexpected token: %s", token)
		}
	}
	
	// Validate the parsed query
	if err := qp.validateQuery(parsed); err != nil {
		return nil, err
	}
	
	return parsed, nil
}

// tokenize breaks the query into tokens
func (qp *QueryParser) tokenize(query string) []string {
	// Handle quoted strings
	var tokens []string
	var current strings.Builder
	inQuotes := false
	quoteChar := byte(0)
	
	for i := 0; i < len(query); i++ {
		ch := query[i]
		
		if !inQuotes && (ch == '\'' || ch == '"') {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			inQuotes = true
			quoteChar = ch
		} else if inQuotes && ch == quoteChar {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			inQuotes = false
			quoteChar = 0
		} else if !inQuotes && (ch == ' ' || ch == '\t' || ch == '\n' || ch == ',' || ch == '(' || ch == ')') {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			if ch == ',' || ch == '(' || ch == ')' {
				tokens = append(tokens, string(ch))
			}
		} else {
			current.WriteByte(ch)
		}
	}
	
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	
	// Filter empty tokens
	var filtered []string
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token != "" {
			filtered = append(filtered, token)
		}
	}
	
	return filtered
}

// parseSelect parses the SELECT clause
func (qp *QueryParser) parseSelect(tokens []string, start int, parsed *ParsedQuery) (int, error) {
	i := start + 1 // Skip "SELECT"
	
	for i < len(tokens) {
		token := tokens[i]
		upperToken := strings.ToUpper(token)
		
		if upperToken == "FROM" {
			break
		}
		
		if token == "," {
			i++
			continue
		}
		
		// Check for aggregation functions
		if qp.isAggregationFunction(upperToken) && i+1 < len(tokens) && tokens[i+1] == "(" {
			agg, nextI, err := qp.parseAggregation(tokens, i)
			if err != nil {
				return i, err
			}
			parsed.Aggregations = append(parsed.Aggregations, *agg)
			i = nextI
		} else {
			// Regular field
			parsed.SelectFields = append(parsed.SelectFields, token)
			i++
		}
		
		// Handle AS alias (for future enhancement)
		if i < len(tokens) && strings.ToUpper(tokens[i]) == "AS" {
			i += 2 // Skip AS and alias
		}
	}
	
	if len(parsed.SelectFields) == 0 && len(parsed.Aggregations) == 0 {
		return i, fmt.Errorf("no fields selected")
	}
	
	return i, nil
}

// parseAggregation parses aggregation functions like COUNT(field)
func (qp *QueryParser) parseAggregation(tokens []string, start int) (*AggregationFunction, int, error) {
	if start+2 >= len(tokens) {
		return nil, start, fmt.Errorf("incomplete aggregation function")
	}
	
	function := strings.ToUpper(tokens[start])
	if tokens[start+1] != "(" {
		return nil, start, fmt.Errorf("expected '(' after %s", function)
	}
	
	// Find the closing parenthesis
	field := ""
	i := start + 2
	for i < len(tokens) && tokens[i] != ")" {
		if field != "" {
			field += " "
		}
		field += tokens[i]
		i++
	}
	
	if i >= len(tokens) {
		return nil, start, fmt.Errorf("missing closing ')' for %s", function)
	}
	
	agg := &AggregationFunction{
		Function: function,
		Field:    field,
	}
	
	return agg, i + 1, nil
}

// parseFrom parses the FROM clause
func (qp *QueryParser) parseFrom(tokens []string, start int, parsed *ParsedQuery) (int, error) {
	if start+1 >= len(tokens) {
		return start, fmt.Errorf("missing table name after FROM")
	}
	
	parsed.FromType = strings.ToLower(tokens[start+1])
	
	// Validate table name
	validTables := []string{"files", "commits", "content", "blobs"}
	valid := false
	for _, table := range validTables {
		if parsed.FromType == table {
			valid = true
			break
		}
	}
	
	if !valid {
		return start, fmt.Errorf("invalid table name: %s. Valid tables: %s", 
			parsed.FromType, strings.Join(validTables, ", "))
	}
	
	return start + 2, nil
}

// parseWhere parses the WHERE clause
func (qp *QueryParser) parseWhere(tokens []string, start int, parsed *ParsedQuery) (int, error) {
	whereClause := &WhereClause{
		Conditions: []Condition{},
		Logic:      "AND", // Default logic
	}
	
	i := start + 1 // Skip "WHERE"
	
	for i < len(tokens) {
		token := strings.ToUpper(tokens[i])
		
		// Stop at other clauses
		if qp.keywords[token] && token != "AND" && token != "OR" && token != "NOT" {
			break
		}
		
		if token == "AND" || token == "OR" {
			whereClause.Logic = token
			i++
			continue
		}
		
		// Parse condition
		condition, nextI, err := qp.parseCondition(tokens, i)
		if err != nil {
			return i, err
		}
		
		whereClause.Conditions = append(whereClause.Conditions, *condition)
		i = nextI
	}
	
	parsed.WhereClause = whereClause
	return i, nil
}

// parseCondition parses a single condition
func (qp *QueryParser) parseCondition(tokens []string, start int) (*Condition, int, error) {
	if start+2 >= len(tokens) {
		return nil, start, fmt.Errorf("incomplete condition")
	}
	
	condition := &Condition{}
	i := start
	
	// Handle NOT
	if strings.ToUpper(tokens[i]) == "NOT" {
		condition.Not = true
		i++
		if i >= len(tokens) {
			return nil, start, fmt.Errorf("incomplete NOT condition")
		}
	}
	
	// Field name
	condition.Field = tokens[i]
	i++
	
	// Operator
	if i >= len(tokens) {
		return nil, start, fmt.Errorf("missing operator")
	}
	condition.Operator = strings.ToUpper(tokens[i])
	i++
	
	// Value
	if i >= len(tokens) {
		return nil, start, fmt.Errorf("missing value")
	}
	
	// Parse value (could be string, number, or list for IN operator)
	if condition.Operator == "IN" {
		values, nextI, err := qp.parseValueList(tokens, i)
		if err != nil {
			return nil, start, err
		}
		condition.Value = values
		i = nextI
	} else {
		condition.Value = qp.parseValue(tokens[i])
		i++
	}
	
	return condition, i, nil
}

// parseValueList parses a list of values for IN operator
func (qp *QueryParser) parseValueList(tokens []string, start int) ([]interface{}, int, error) {
	if start >= len(tokens) || tokens[start] != "(" {
		return nil, start, fmt.Errorf("expected '(' for IN list")
	}
	
	var values []interface{}
	i := start + 1
	
	for i < len(tokens) && tokens[i] != ")" {
		if tokens[i] == "," {
			i++
			continue
		}
		values = append(values, qp.parseValue(tokens[i]))
		i++
	}
	
	if i >= len(tokens) {
		return nil, start, fmt.Errorf("missing closing ')' for IN list")
	}
	
	return values, i + 1, nil
}

// parseValue parses a single value, attempting to determine its type
func (qp *QueryParser) parseValue(token string) interface{} {
	// Try to parse as number
	if intVal, err := strconv.Atoi(token); err == nil {
		return intVal
	}
	
	if floatVal, err := strconv.ParseFloat(token, 64); err == nil {
		return floatVal
	}
	
	// Try to parse as boolean
	if strings.ToLower(token) == "true" {
		return true
	}
	if strings.ToLower(token) == "false" {
		return false
	}
	
	// Default to string
	return token
}

// parseOrderBy parses ORDER BY clause
func (qp *QueryParser) parseOrderBy(tokens []string, start int, parsed *ParsedQuery) (int, error) {
	if start+1 >= len(tokens) || strings.ToUpper(tokens[start+1]) != "BY" {
		return start, fmt.Errorf("expected BY after ORDER")
	}
	
	i := start + 2 // Skip "ORDER BY"
	
	for i < len(tokens) {
		token := strings.ToUpper(tokens[i])
		
		// Stop at other clauses
		if qp.keywords[token] && token != "ASC" && token != "DESC" {
			break
		}
		
		if token == "," {
			i++
			continue
		}
		
		// Field name
		field := tokens[i]
		direction := "ASC" // Default
		i++
		
		// Check for ASC/DESC
		if i < len(tokens) {
			nextToken := strings.ToUpper(tokens[i])
			if nextToken == "ASC" || nextToken == "DESC" {
				direction = nextToken
				i++
			}
		}
		
		parsed.OrderBy = append(parsed.OrderBy, OrderByClause{
			Field:     field,
			Direction: direction,
		})
	}
	
	return i, nil
}

// parseGroupBy parses GROUP BY clause
func (qp *QueryParser) parseGroupBy(tokens []string, start int, parsed *ParsedQuery) (int, error) {
	if start+1 >= len(tokens) || strings.ToUpper(tokens[start+1]) != "BY" {
		return start, fmt.Errorf("expected BY after GROUP")
	}
	
	i := start + 2 // Skip "GROUP BY"
	
	for i < len(tokens) {
		token := strings.ToUpper(tokens[i])
		
		// Stop at other clauses
		if qp.keywords[token] {
			break
		}
		
		if token == "," {
			i++
			continue
		}
		
		parsed.GroupBy = append(parsed.GroupBy, tokens[i])
		i++
	}
	
	return i, nil
}

// parseHaving parses HAVING clause (similar to WHERE)
func (qp *QueryParser) parseHaving(tokens []string, start int, parsed *ParsedQuery) (int, error) {
	havingClause := &WhereClause{
		Conditions: []Condition{},
		Logic:      "AND",
	}
	
	i := start + 1 // Skip "HAVING"
	
	for i < len(tokens) {
		token := strings.ToUpper(tokens[i])
		
		// Stop at other clauses
		if qp.keywords[token] && token != "AND" && token != "OR" && token != "NOT" {
			break
		}
		
		if token == "AND" || token == "OR" {
			havingClause.Logic = token
			i++
			continue
		}
		
		condition, nextI, err := qp.parseCondition(tokens, i)
		if err != nil {
			return i, err
		}
		
		havingClause.Conditions = append(havingClause.Conditions, *condition)
		i = nextI
	}
	
	parsed.Having = havingClause
	return i, nil
}

// parseLimit parses LIMIT clause
func (qp *QueryParser) parseLimit(tokens []string, start int, parsed *ParsedQuery) (int, error) {
	if start+1 >= len(tokens) {
		return start, fmt.Errorf("missing value after LIMIT")
	}
	
	limit, err := strconv.Atoi(tokens[start+1])
	if err != nil {
		return start, fmt.Errorf("invalid LIMIT value: %s", tokens[start+1])
	}
	
	parsed.Limit = limit
	return start + 2, nil
}

// parseOffset parses OFFSET clause
func (qp *QueryParser) parseOffset(tokens []string, start int, parsed *ParsedQuery) (int, error) {
	if start+1 >= len(tokens) {
		return start, fmt.Errorf("missing value after OFFSET")
	}
	
	offset, err := strconv.Atoi(tokens[start+1])
	if err != nil {
		return start, fmt.Errorf("invalid OFFSET value: %s", tokens[start+1])
	}
	
	parsed.Offset = offset
	return start + 2, nil
}

// isAggregationFunction checks if a token is an aggregation function
func (qp *QueryParser) isAggregationFunction(token string) bool {
	aggregations := []string{"COUNT", "SUM", "AVG", "MIN", "MAX"}
	for _, agg := range aggregations {
		if token == agg {
			return true
		}
	}
	return false
}

// validateQuery validates the parsed query
func (qp *QueryParser) validateQuery(parsed *ParsedQuery) error {
	if parsed.FromType == "" {
		return fmt.Errorf("missing FROM clause")
	}
	
	if len(parsed.SelectFields) == 0 && len(parsed.Aggregations) == 0 {
		return fmt.Errorf("no fields selected")
	}
	
	// Validate GROUP BY usage
	if len(parsed.GroupBy) > 0 && len(parsed.Aggregations) == 0 {
		return fmt.Errorf("GROUP BY requires aggregation functions")
	}
	
	return nil
}

// Example queries this parser can handle:
/*
SELECT path, size FROM files WHERE size > 1000 ORDER BY size DESC LIMIT 10
SELECT COUNT(*) FROM commits WHERE author = 'john.doe'
SELECT path FROM files WHERE path LIKE '*.go' AND size > 500
SELECT author, COUNT(*) FROM commits GROUP BY author ORDER BY COUNT(*) DESC
SELECT * FROM content WHERE content CONTAINS 'error' OR content CONTAINS 'exception'
SELECT path, last_modified FROM files WHERE last_modified > '2024-01-01' ORDER BY last_modified DESC
SELECT SUM(size), AVG(size) FROM files WHERE path LIKE '*.js'
*/