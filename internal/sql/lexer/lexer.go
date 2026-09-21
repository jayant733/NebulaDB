// Package lexer tokenizes the NebulaDB SQL subset.
package lexer

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind is a token type.
type Kind int

const (
	EOF Kind = iota
	Ident
	Number
	String
	Star
	Comma
	LParen
	RParen
	Semicolon
	Eq
	Ne
	Lt
	Gt
	Le
	Ge

	KwCreate
	KwTable
	KwInsert
	KwInto
	KwValues
	KwSelect
	KwFrom
	KwWhere
	KwUpdate
	KwSet
	KwDelete
	KwOrder
	KwBy
	KwLimit
	KwIndex
	KwOn
	KwAsc
	KwDesc
	KwAnd
	KwInt
	KwText
	KwBegin
	KwCommit
	KwRollback
	KwTransaction
	KwRead
	KwCommitted
	KwRepeatable
	KwJoin
	KwInner
	KwGroup
	KwCount
	KwSum
	KwAvg
	Dot
)

var keywords = map[string]Kind{
	"CREATE":      KwCreate,
	"TABLE":       KwTable,
	"INSERT":      KwInsert,
	"INTO":        KwInto,
	"VALUES":      KwValues,
	"SELECT":      KwSelect,
	"FROM":        KwFrom,
	"WHERE":       KwWhere,
	"UPDATE":      KwUpdate,
	"SET":         KwSet,
	"DELETE":      KwDelete,
	"ORDER":       KwOrder,
	"BY":          KwBy,
	"LIMIT":       KwLimit,
	"INDEX":       KwIndex,
	"ON":          KwOn,
	"ASC":         KwAsc,
	"DESC":        KwDesc,
	"AND":         KwAnd,
	"INT":         KwInt,
	"TEXT":        KwText,
	"BEGIN":       KwBegin,
	"COMMIT":      KwCommit,
	"ROLLBACK":    KwRollback,
	"TRANSACTION": KwTransaction,
	"READ":        KwRead,
	"COMMITTED":   KwCommitted,
	"REPEATABLE":  KwRepeatable,
	"JOIN":        KwJoin,
	"INNER":       KwInner,
	"GROUP":       KwGroup,
	"COUNT":       KwCount,
	"SUM":         KwSum,
	"AVG":         KwAvg,
}

// Token is one lexeme.
type Token struct {
	Kind Kind
	Val  string
	Pos  int
}

// Lexer walks src.
type Lexer struct {
	src string
	i   int
}

// New creates a lexer.
func New(src string) *Lexer {
	return &Lexer{src: src}
}

// Next returns the next token.
func (l *Lexer) Next() (Token, error) {
	l.skipWS()
	if l.i >= len(l.src) {
		return Token{Kind: EOF, Pos: l.i}, nil
	}
	pos := l.i
	r, w := utf8.DecodeRuneInString(l.src[l.i:])
	switch r {
	case '.':
		l.i += w
		return Token{Kind: Dot, Val: ".", Pos: pos}, nil
	case '*':
		l.i += w
		return Token{Kind: Star, Val: "*", Pos: pos}, nil
	case ',':
		l.i += w
		return Token{Kind: Comma, Val: ",", Pos: pos}, nil
	case '(':
		l.i += w
		return Token{Kind: LParen, Val: "(", Pos: pos}, nil
	case ')':
		l.i += w
		return Token{Kind: RParen, Val: ")", Pos: pos}, nil
	case ';':
		l.i += w
		return Token{Kind: Semicolon, Val: ";", Pos: pos}, nil
	case '=':
		l.i += w
		return Token{Kind: Eq, Val: "=", Pos: pos}, nil
	case '<':
		if l.peek('=') {
			l.i += 2
			return Token{Kind: Le, Val: "<=", Pos: pos}, nil
		}
		l.i += w
		return Token{Kind: Lt, Val: "<", Pos: pos}, nil
	case '>':
		if l.peek('=') {
			l.i += 2
			return Token{Kind: Ge, Val: ">=", Pos: pos}, nil
		}
		l.i += w
		return Token{Kind: Gt, Val: ">", Pos: pos}, nil
	case '!':
		if l.peek('=') {
			l.i += 2
			return Token{Kind: Ne, Val: "!=", Pos: pos}, nil
		}
		return Token{}, fmt.Errorf("lexer: unexpected '!' at %d", pos)
	case '\'':
		return l.string(pos)
	}
	if r == '-' && l.i+1 < len(l.src) && l.src[l.i+1] == '-' {
		l.skipLine()
		return l.Next()
	}
	if r == '-' && l.i+1 < len(l.src) && isDigit(rune(l.src[l.i+1])) {
		return l.number(pos)
	}
	if isDigit(r) {
		return l.number(pos)
	}
	if isIdentStart(r) {
		return l.ident(pos)
	}
	return Token{}, fmt.Errorf("lexer: unexpected %q at %d", r, pos)
}

func (l *Lexer) skipWS() {
	for l.i < len(l.src) {
		r, w := utf8.DecodeRuneInString(l.src[l.i:])
		if !unicode.IsSpace(r) {
			return
		}
		l.i += w
	}
}

func (l *Lexer) skipLine() {
	for l.i < len(l.src) && l.src[l.i] != '\n' {
		l.i++
	}
}

func (l *Lexer) peek(c byte) bool {
	return l.i+1 < len(l.src) && l.src[l.i+1] == c
}

func (l *Lexer) string(pos int) (Token, error) {
	l.i++ // opening quote
	var b strings.Builder
	for l.i < len(l.src) {
		if l.src[l.i] == '\'' {
			if l.i+1 < len(l.src) && l.src[l.i+1] == '\'' {
				b.WriteByte('\'')
				l.i += 2
				continue
			}
			l.i++
			return Token{Kind: String, Val: b.String(), Pos: pos}, nil
		}
		b.WriteByte(l.src[l.i])
		l.i++
	}
	return Token{}, fmt.Errorf("lexer: unterminated string at %d", pos)
}

func (l *Lexer) number(pos int) (Token, error) {
	start := l.i
	if l.src[l.i] == '-' {
		l.i++
	}
	for l.i < len(l.src) && isDigit(rune(l.src[l.i])) {
		l.i++
	}
	return Token{Kind: Number, Val: l.src[start:l.i], Pos: pos}, nil
}

func (l *Lexer) ident(pos int) (Token, error) {
	start := l.i
	for l.i < len(l.src) {
		r, w := utf8.DecodeRuneInString(l.src[l.i:])
		if !isIdentPart(r) {
			break
		}
		l.i += w
	}
	raw := l.src[start:l.i]
	up := strings.ToUpper(raw)
	if k, ok := keywords[up]; ok {
		return Token{Kind: k, Val: up, Pos: pos}, nil
	}
	return Token{Kind: Ident, Val: raw, Pos: pos}, nil
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentPart(r rune) bool {
	return isIdentStart(r) || isDigit(r)
}

// Collect lexes the entire input (for tests).
func Collect(src string) ([]Token, error) {
	l := New(src)
	var out []Token
	for {
		t, err := l.Next()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		if t.Kind == EOF {
			return out, nil
		}
	}
}
