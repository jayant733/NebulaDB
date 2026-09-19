// Package parser turns tokens into an AST.
package parser

import (
	"fmt"
	"strconv"

	"github.com/jayant/nebuladb/internal/sql/ast"
	"github.com/jayant/nebuladb/internal/sql/lexer"
)

// Parser is a recursive-descent parser.
type Parser struct {
	lx   *lexer.Lexer
	cur  lexer.Token
	peek lexer.Token
}

// Parse lexes and parses one statement (trailing semicolon optional).
func Parse(src string) (ast.Stmt, error) {
	p := &Parser{lx: lexer.New(src)}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	stmt, err := p.statement()
	if err != nil {
		return nil, err
	}
	if p.cur.Kind == lexer.Semicolon {
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
	if p.cur.Kind != lexer.EOF {
		return nil, fmt.Errorf("parser: unexpected token %q", p.cur.Val)
	}
	return stmt, nil
}

func (p *Parser) advance() error {
	p.cur = p.peek
	t, err := p.lx.Next()
	if err != nil {
		return err
	}
	p.peek = t
	return nil
}

func (p *Parser) expect(k lexer.Kind, what string) error {
	if p.cur.Kind != k {
		return fmt.Errorf("parser: expected %s, got %q", what, p.cur.Val)
	}
	return p.advance()
}

func (p *Parser) statement() (ast.Stmt, error) {
	switch p.cur.Kind {
	case lexer.KwCreate:
		return p.create()
	case lexer.KwInsert:
		return p.insert()
	case lexer.KwSelect:
		return p.selectStmt()
	case lexer.KwUpdate:
		return p.update()
	case lexer.KwDelete:
		return p.delete()
	case lexer.KwBegin:
		return p.begin()
	case lexer.KwCommit:
		return p.commit()
	case lexer.KwRollback:
		return p.rollback()
	default:
		return nil, fmt.Errorf("parser: expected statement, got %q", p.cur.Val)
	}
}

func (p *Parser) create() (ast.Stmt, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	switch p.cur.Kind {
	case lexer.KwTable:
		return p.createTable()
	case lexer.KwIndex:
		return p.createIndex()
	default:
		return nil, fmt.Errorf("parser: expected TABLE or INDEX")
	}
}

func (p *Parser) createTable() (ast.Stmt, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	name, err := p.ident()
	if err != nil {
		return nil, err
	}
	if err := p.expect(lexer.LParen, "("); err != nil {
		return nil, err
	}
	var cols []ast.ColumnDef
	for {
		cname, err := p.ident()
		if err != nil {
			return nil, err
		}
		var typ ast.Type
		switch p.cur.Kind {
		case lexer.KwInt:
			typ = ast.TypeInt
		case lexer.KwText:
			typ = ast.TypeText
		default:
			return nil, fmt.Errorf("parser: expected INT or TEXT")
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		cols = append(cols, ast.ColumnDef{Name: cname, Type: typ})
		if p.cur.Kind == lexer.Comma {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		break
	}
	if err := p.expect(lexer.RParen, ")"); err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("parser: table needs columns")
	}
	return &ast.CreateTable{Name: name, Cols: cols}, nil
}

func (p *Parser) createIndex() (ast.Stmt, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	iname, err := p.ident()
	if err != nil {
		return nil, err
	}
	if err := p.expect(lexer.KwOn, "ON"); err != nil {
		return nil, err
	}
	tname, err := p.ident()
	if err != nil {
		return nil, err
	}
	if err := p.expect(lexer.LParen, "("); err != nil {
		return nil, err
	}
	col, err := p.ident()
	if err != nil {
		return nil, err
	}
	if err := p.expect(lexer.RParen, ")"); err != nil {
		return nil, err
	}
	return &ast.CreateIndex{Name: iname, Table: tname, Column: col}, nil
}

func (p *Parser) insert() (ast.Stmt, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	if err := p.expect(lexer.KwInto, "INTO"); err != nil {
		return nil, err
	}
	table, err := p.ident()
	if err != nil {
		return nil, err
	}
	if err := p.expect(lexer.KwValues, "VALUES"); err != nil {
		return nil, err
	}
	var tuples [][]ast.Literal
	for {
		tup, err := p.tuple()
		if err != nil {
			return nil, err
		}
		tuples = append(tuples, tup)
		if p.cur.Kind == lexer.Comma {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		break
	}
	return &ast.Insert{Table: table, Values: tuples}, nil
}

func (p *Parser) tuple() ([]ast.Literal, error) {
	if err := p.expect(lexer.LParen, "("); err != nil {
		return nil, err
	}
	var vals []ast.Literal
	for {
		lit, err := p.literal()
		if err != nil {
			return nil, err
		}
		vals = append(vals, lit)
		if p.cur.Kind == lexer.Comma {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		break
	}
	if err := p.expect(lexer.RParen, ")"); err != nil {
		return nil, err
	}
	return vals, nil
}

func (p *Parser) selectStmt() (ast.Stmt, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	s := &ast.Select{Limit: -1}
	if p.cur.Kind == lexer.Star {
		s.Star = true
		if err := p.advance(); err != nil {
			return nil, err
		}
	} else {
		for {
			col, err := p.ident()
			if err != nil {
				return nil, err
			}
			s.Cols = append(s.Cols, col)
			if p.cur.Kind == lexer.Comma {
				if err := p.advance(); err != nil {
					return nil, err
				}
				continue
			}
			break
		}
	}
	if err := p.expect(lexer.KwFrom, "FROM"); err != nil {
		return nil, err
	}
	table, err := p.ident()
	if err != nil {
		return nil, err
	}
	s.Table = table
	if p.cur.Kind == lexer.KwWhere {
		if err := p.advance(); err != nil {
			return nil, err
		}
		pred, err := p.predicate()
		if err != nil {
			return nil, err
		}
		s.Where = pred
	}
	if p.cur.Kind == lexer.KwOrder {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if err := p.expect(lexer.KwBy, "BY"); err != nil {
			return nil, err
		}
		col, err := p.ident()
		if err != nil {
			return nil, err
		}
		s.OrderCol = col
		if p.cur.Kind == lexer.KwDesc {
			s.OrderDesc = true
			if err := p.advance(); err != nil {
				return nil, err
			}
		} else if p.cur.Kind == lexer.KwAsc {
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
	}
	if p.cur.Kind == lexer.KwLimit {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.cur.Kind != lexer.Number {
			return nil, fmt.Errorf("parser: LIMIT needs a number")
		}
		n, err := strconv.Atoi(p.cur.Val)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("parser: bad LIMIT")
		}
		s.Limit = n
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (p *Parser) update() (ast.Stmt, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	table, err := p.ident()
	if err != nil {
		return nil, err
	}
	if err := p.expect(lexer.KwSet, "SET"); err != nil {
		return nil, err
	}
	u := &ast.Update{Table: table}
	for {
		col, err := p.ident()
		if err != nil {
			return nil, err
		}
		if err := p.expect(lexer.Eq, "="); err != nil {
			return nil, err
		}
		lit, err := p.literal()
		if err != nil {
			return nil, err
		}
		u.Sets = append(u.Sets, ast.Assignment{Col: col, Lit: lit})
		if p.cur.Kind == lexer.Comma {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		break
	}
	if p.cur.Kind == lexer.KwWhere {
		if err := p.advance(); err != nil {
			return nil, err
		}
		pred, err := p.predicate()
		if err != nil {
			return nil, err
		}
		u.Where = pred
	}
	return u, nil
}

func (p *Parser) delete() (ast.Stmt, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	if err := p.expect(lexer.KwFrom, "FROM"); err != nil {
		return nil, err
	}
	table, err := p.ident()
	if err != nil {
		return nil, err
	}
	d := &ast.Delete{Table: table}
	if p.cur.Kind == lexer.KwWhere {
		if err := p.advance(); err != nil {
			return nil, err
		}
		pred, err := p.predicate()
		if err != nil {
			return nil, err
		}
		d.Where = pred
	}
	return d, nil
}

func (p *Parser) begin() (ast.Stmt, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.cur.Kind == lexer.KwTransaction {
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
	iso := ast.IsoReadCommitted
	if p.cur.Kind == lexer.KwRead {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if err := p.expect(lexer.KwCommitted, "COMMITTED"); err != nil {
			return nil, err
		}
		iso = ast.IsoReadCommitted
	} else if p.cur.Kind == lexer.KwRepeatable {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if err := p.expect(lexer.KwRead, "READ"); err != nil {
			return nil, err
		}
		iso = ast.IsoRepeatableRead
	}
	return &ast.Begin{Iso: iso}, nil
}

func (p *Parser) commit() (ast.Stmt, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.cur.Kind == lexer.KwTransaction {
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
	return &ast.Commit{}, nil
}

func (p *Parser) rollback() (ast.Stmt, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.cur.Kind == lexer.KwTransaction {
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
	return &ast.Rollback{}, nil
}

func (p *Parser) predicate() (*ast.Predicate, error) {
	col, err := p.ident()
	if err != nil {
		return nil, err
	}
	op, err := p.cmpOp()
	if err != nil {
		return nil, err
	}
	lit, err := p.literal()
	if err != nil {
		return nil, err
	}
	pred := &ast.Predicate{Col: col, Op: op, Lit: lit}
	if p.cur.Kind == lexer.KwAnd {
		if err := p.advance(); err != nil {
			return nil, err
		}
		next, err := p.predicate()
		if err != nil {
			return nil, err
		}
		pred.Next = next
	}
	return pred, nil
}

func (p *Parser) cmpOp() (ast.CmpOp, error) {
	var op ast.CmpOp
	switch p.cur.Kind {
	case lexer.Eq:
		op = ast.OpEq
	case lexer.Ne:
		op = ast.OpNe
	case lexer.Lt:
		op = ast.OpLt
	case lexer.Gt:
		op = ast.OpGt
	case lexer.Le:
		op = ast.OpLe
	case lexer.Ge:
		op = ast.OpGe
	default:
		return 0, fmt.Errorf("parser: expected comparison operator")
	}
	return op, p.advance()
}

func (p *Parser) ident() (string, error) {
	if p.cur.Kind != lexer.Ident {
		return "", fmt.Errorf("parser: expected identifier, got %q", p.cur.Val)
	}
	s := p.cur.Val
	return s, p.advance()
}

func (p *Parser) literal() (ast.Literal, error) {
	switch p.cur.Kind {
	case lexer.Number:
		n, err := strconv.ParseInt(p.cur.Val, 10, 64)
		if err != nil {
			return ast.Literal{}, fmt.Errorf("parser: bad integer")
		}
		if err := p.advance(); err != nil {
			return ast.Literal{}, err
		}
		return ast.Literal{Num: true, Int: n, Text: strconv.FormatInt(n, 10)}, nil
	case lexer.String:
		s := p.cur.Val
		if err := p.advance(); err != nil {
			return ast.Literal{}, err
		}
		return ast.Literal{Text: s}, nil
	default:
		return ast.Literal{}, fmt.Errorf("parser: expected literal, got %q", p.cur.Val)
	}
}
