// Copyright (c) 2026 Nokia. All rights reserved.

// Package dolt validates configured Dolt boundary operations.
package dolt

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

type OperationKind string

const (
	KindProvision OperationKind = "provision"
	KindQuery     OperationKind = "query"
	KindWrite     OperationKind = "write"
)

type StatementKind string

const (
	StatementRead   StatementKind = "read"
	StatementWrite  StatementKind = "write"
	StatementSchema StatementKind = "schema"
)

type OperationConfig struct {
	ConnectionRef    string                 `json:"connection_ref"`
	Database         string                 `json:"database"`
	Operation        string                 `json:"operation"`
	Kind             OperationKind          `json:"kind"`
	Statement        string                 `json:"statement"`
	SchemaStatements []string               `json:"schema_statements"`
	ParameterSchema  map[string]interface{} `json:"parameter_schema"`
	MaxRows          int                    `json:"max_rows"`
	MaxBytes         int                    `json:"max_bytes"`
	Timeout          string                 `json:"timeout"`
	CommitMessage    string                 `json:"commit_message"`
	CommitOnNoChange bool                   `json:"commit_on_no_change"`
}

type PreparedConfig struct {
	OperationConfig
	TimeoutDuration time.Duration
	Parameters      *ParameterValidator
	SQL             PreparedStatement
	CommitTemplate  *CommitTemplate
}

var literalIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func DecodeConfig(def catalog.ToolDef) (*PreparedConfig, error) {
	var cfg OperationConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	prepared, err := PrepareConfig(def.Name, cfg)
	if err != nil {
		return nil, fmt.Errorf("tool %q config: %w", def.Name, err)
	}
	return prepared, nil
}

func PrepareConfig(toolName string, cfg OperationConfig) (*PreparedConfig, error) {
	timeout, params, err := validateBase(toolName, cfg)
	if err != nil {
		return nil, err
	}
	out := &PreparedConfig{OperationConfig: cfg, TimeoutDuration: timeout, Parameters: params}
	switch cfg.Kind {
	case KindProvision:
		err = prepareProvision(out)
	case KindQuery:
		err = prepareQuery(out)
	case KindWrite:
		err = prepareWrite(out)
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func validateBase(name string, cfg OperationConfig) (time.Duration, *ParameterValidator, error) {
	if strings.TrimSpace(cfg.ConnectionRef) == "" {
		return 0, nil, fmt.Errorf("requires connection_ref")
	}
	if !literalIdentifier.MatchString(cfg.Database) || !literalIdentifier.MatchString(cfg.Operation) {
		return 0, nil, fmt.Errorf("database and operation must be literal identifiers")
	}
	if cfg.Kind != KindProvision && cfg.Kind != KindQuery && cfg.Kind != KindWrite {
		return 0, nil, fmt.Errorf("unknown operation kind %q", cfg.Kind)
	}
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil || timeout <= 0 {
		return 0, nil, fmt.Errorf("timeout must be a positive duration")
	}
	if cfg.MaxRows < 0 || cfg.MaxBytes < 0 {
		return 0, nil, fmt.Errorf("result limits must not be negative")
	}
	params, err := CompileParameterSchema(name, cfg.ParameterSchema)
	return timeout, params, err
}

func prepareProvision(cfg *PreparedConfig) error {
	if strings.TrimSpace(cfg.Statement) != "" || len(cfg.SchemaStatements) == 0 {
		return fmt.Errorf("provision requires schema_statements and no statement")
	}
	if len(cfg.Parameters.declared) != 0 || cfg.MaxRows != 0 || cfg.MaxBytes != 0 {
		return fmt.Errorf("provision cannot declare runtime parameters or result limits")
	}
	for i, raw := range cfg.SchemaStatements {
		statement, err := prepareStatement(raw)
		if err != nil {
			return fmt.Errorf("schema_statements[%d]: %w", i, err)
		}
		if statement.Kind != StatementSchema || len(statement.Names) != 0 {
			return fmt.Errorf("schema_statements[%d]: requires placeholder-free schema SQL", i)
		}
	}
	return setCommitTemplate(cfg)
}

func prepareQuery(cfg *PreparedConfig) error {
	if len(cfg.SchemaStatements) != 0 || cfg.CommitMessage != "" || cfg.CommitOnNoChange {
		return fmt.Errorf("query cannot configure schema or commit behavior")
	}
	if cfg.MaxRows <= 0 || cfg.MaxBytes <= 0 {
		return fmt.Errorf("query requires positive max_rows and max_bytes")
	}
	return setStatement(cfg, StatementRead)
}

func prepareWrite(cfg *PreparedConfig) error {
	if len(cfg.SchemaStatements) != 0 || cfg.MaxRows != 0 || cfg.MaxBytes != 0 {
		return fmt.Errorf("write cannot configure schema or result limits")
	}
	if err := setStatement(cfg, StatementWrite); err != nil {
		return err
	}
	return setCommitTemplate(cfg)
}

func setStatement(cfg *PreparedConfig, want StatementKind) error {
	statement, err := prepareStatement(cfg.Statement)
	if err != nil {
		return err
	}
	if statement.Kind != want {
		return fmt.Errorf("operation-kind mismatch: %s requires %s SQL, got %s", cfg.Kind, want, statement.Kind)
	}
	if err := cfg.Parameters.ValidatePlaceholders(statement.Names); err != nil {
		return err
	}
	cfg.SQL = statement
	return nil
}

func setCommitTemplate(cfg *PreparedConfig) error {
	template, err := CompileCommitTemplate(cfg.CommitMessage, cfg.Parameters.Declared())
	if err != nil {
		return fmt.Errorf("commit_message: %w", err)
	}
	if template == nil {
		return fmt.Errorf("%s requires commit_message", cfg.Kind)
	}
	cfg.CommitTemplate = template
	return nil
}

func prepareStatement(statement string) (PreparedStatement, error) { return scanSQL(statement) }

func classify(tokens []string) (StatementKind, error) {
	if len(tokens) == 0 {
		return "", fmt.Errorf("statement must not be empty")
	}
	for _, token := range tokens {
		if strings.HasPrefix(token, "DOLT_") {
			return "", fmt.Errorf("dolt control function %q is not allowed", token)
		}
	}
	first := tokens[0]
	if first == "INSERT" || first == "UPDATE" || first == "DELETE" || first == "REPLACE" {
		return StatementWrite, nil
	}
	if first == "CREATE" && len(tokens) > 1 && slices.Contains([]string{"TABLE", "INDEX", "VIEW"}, tokens[1]) ||
		first == "ALTER" && len(tokens) > 1 && tokens[1] == "TABLE" {
		return StatementSchema, nil
	}
	return classifyRead(first, tokens)
}

func classifyRead(first string, tokens []string) (StatementKind, error) {
	read := slices.Contains([]string{"SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN", "WITH"}, first)
	mutation := hasAny(tokens, "INSERT", "UPDATE", "DELETE", "REPLACE")
	if first == "WITH" && mutation {
		return "", fmt.Errorf("mutating common-table expressions are not supported")
	}
	if !read || first == "EXPLAIN" && mutation {
		return "", fmt.Errorf("unsupported SQL statement kind %q", first)
	}
	if first == "WITH" && !hasAny(tokens, "SELECT") {
		return "", fmt.Errorf("common-table expression must end in a read")
	}
	if hasAny(tokens, "INTO", "OUTFILE", "DUMPFILE", "FOR", "LOCK") {
		return "", fmt.Errorf("unsupported potentially mutating read statement")
	}
	return StatementRead, nil
}

func hasAny(values []string, choices ...string) bool {
	for _, value := range values {
		if slices.Contains(choices, value) {
			return true
		}
	}
	return false
}

type sqlLexer struct {
	raw                 string
	out, plain          strings.Builder
	names               []string
	pos                 int
	quote, comment      byte
	statementTerminated bool
}

func scanSQL(raw string) (PreparedStatement, error) {
	lexer := &sqlLexer{raw: raw}
	for lexer.pos < len(raw) {
		var err error
		switch {
		case lexer.quote != 0:
			lexer.scanQuote()
		case lexer.comment != 0:
			lexer.scanComment()
		default:
			err = lexer.scanCode()
		}
		if err != nil {
			return PreparedStatement{}, err
		}
	}
	return lexer.finish()
}

func (l *sqlLexer) scanCode() error {
	ch := l.raw[l.pos]
	if started, err := l.startComment(ch); started || err != nil {
		return err
	}
	if l.statementTerminated && !unicode.IsSpace(rune(ch)) {
		return fmt.Errorf("multiple SQL statements are not allowed")
	}
	if ch == '\'' || ch == '"' || ch == '`' {
		l.quote = ch
		l.out.WriteByte(ch)
		l.plain.WriteByte(' ')
		l.pos++
		return nil
	}
	if ch == ';' {
		l.statementTerminated = true
		l.out.WriteByte(ch)
		l.plain.WriteByte(' ')
		l.pos++
		return nil
	}
	if ch == '?' {
		return fmt.Errorf("unnamed SQL placeholders are not allowed")
	}
	if ch == ':' && isIdentStart(l.peek(1)) {
		l.scanPlaceholder()
		return nil
	}
	l.out.WriteByte(ch)
	l.plain.WriteByte(ch)
	l.pos++
	return nil
}

func (l *sqlLexer) startComment(ch byte) (bool, error) {
	size, kind := 0, byte(0)
	switch {
	case ch == '#':
		size, kind = 1, '#'
	case ch == '-' && l.peek(1) == '-' && unicode.IsSpace(rune(l.peek(2))):
		size, kind = 2, '-'
	case ch == '/' && l.peek(1) == '*':
		if l.peek(2) == '!' {
			return true, fmt.Errorf("executable SQL comments are not allowed")
		}
		size, kind = 2, '/'
	default:
		return false, nil
	}
	l.comment = kind
	l.out.WriteString(l.raw[l.pos : l.pos+size])
	l.plain.WriteByte(' ')
	l.pos += size
	return true, nil
}

func (l *sqlLexer) scanQuote() {
	ch := l.raw[l.pos]
	l.out.WriteByte(ch)
	l.pos++
	if l.quote == '\'' && ch == '\\' && l.pos < len(l.raw) {
		l.out.WriteByte(l.raw[l.pos])
		l.pos++
		return
	}
	if ch != l.quote {
		return
	}
	if l.pos < len(l.raw) && l.raw[l.pos] == ch {
		l.out.WriteByte(ch)
		l.pos++
		return
	}
	l.quote = 0
}

func (l *sqlLexer) scanComment() {
	ch := l.raw[l.pos]
	l.out.WriteByte(ch)
	l.pos++
	if (l.comment == '#' || l.comment == '-') && ch == '\n' {
		l.comment = 0
	}
	if l.comment == '/' && ch == '*' && l.pos < len(l.raw) && l.raw[l.pos] == '/' {
		l.out.WriteByte('/')
		l.pos++
		l.comment = 0
	}
}

func (l *sqlLexer) scanPlaceholder() {
	end := l.pos + 2
	for end < len(l.raw) && isIdentPart(l.raw[end]) {
		end++
	}
	l.names = append(l.names, l.raw[l.pos+1:end])
	l.out.WriteByte('?')
	l.plain.WriteByte(' ')
	l.pos = end
}

func (l *sqlLexer) finish() (PreparedStatement, error) {
	if l.quote != 0 || l.comment == '/' {
		return PreparedStatement{}, fmt.Errorf("unterminated SQL literal or comment")
	}
	tokens := strings.FieldsFunc(strings.ToUpper(l.plain.String()), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
	if len(tokens) == 0 {
		return PreparedStatement{}, fmt.Errorf("statement must not be empty")
	}
	kind, err := classify(tokens)
	return PreparedStatement{Query: l.out.String(), Names: l.names, Kind: kind}, err
}

func (l *sqlLexer) peek(offset int) byte {
	if l.pos+offset >= len(l.raw) {
		return 0
	}
	return l.raw[l.pos+offset]
}

func isIdentStart(ch byte) bool { return ch == '_' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' }
func isIdentPart(ch byte) bool  { return isIdentStart(ch) || ch >= '0' && ch <= '9' }
