package engine

import (
	"fmt"
	"sort"
	"sync"

	"github.com/jayant/nebuladb/internal/index"
	"github.com/jayant/nebuladb/internal/row"
	"github.com/jayant/nebuladb/internal/sql/ast"
	"github.com/jayant/nebuladb/internal/sql/parser"
	"github.com/jayant/nebuladb/internal/storage"
)

// Result is a statement outcome.
type Result struct {
	Columns  []string
	Rows     [][]string
	Affected int
	Message  string
}

// Engine executes SQL against an LSM store.
type Engine struct {
	mu  sync.Mutex
	kv  *storage.Engine
	cat *Catalog
	txn *storage.Txn
}

// New wraps an opened KV engine.
func New(kv *storage.Engine) (*Engine, error) {
	cat, err := loadCatalog(kv)
	if err != nil {
		return nil, err
	}
	return &Engine{kv: kv, cat: cat}, nil
}

// Exec parses and runs one statement.
func (e *Engine) Exec(src string) (*Result, error) {
	stmt, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	switch s := stmt.(type) {
	case *ast.CreateTable:
		return e.createTable(s)
	case *ast.CreateIndex:
		return e.createIndex(s)
	case *ast.Insert:
		return e.insert(s)
	case *ast.Select:
		return e.selectStmt(s)
	case *ast.Update:
		return e.update(s)
	case *ast.Delete:
		return e.delete(s)
	case *ast.Begin:
		return e.begin(s)
	case *ast.Commit:
		return e.commit()
	case *ast.Rollback:
		return e.rollback()
	default:
		return nil, fmt.Errorf("sql: unsupported statement")
	}
}

func (e *Engine) dataKV() storage.KV {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.txn != nil {
		return e.txn
	}
	return e.kv
}

func (e *Engine) inTxn() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.txn != nil
}

func (e *Engine) begin(s *ast.Begin) (*Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.txn != nil {
		return nil, fmt.Errorf("sql: transaction already open")
	}
	iso := storage.ReadCommitted
	if s.Iso == ast.IsoRepeatableRead {
		iso = storage.RepeatableRead
	}
	e.txn = e.kv.Begin(iso)
	return &Result{Message: "begin"}, nil
}

func (e *Engine) commit() (*Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.txn == nil {
		return nil, fmt.Errorf("sql: no transaction")
	}
	err := e.txn.Commit()
	e.txn = nil
	if err != nil {
		return nil, err
	}
	return &Result{Message: "commit"}, nil
}

func (e *Engine) rollback() (*Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.txn == nil {
		return nil, fmt.Errorf("sql: no transaction")
	}
	err := e.txn.Rollback()
	e.txn = nil
	if err != nil {
		return nil, err
	}
	return &Result{Message: "rollback"}, nil
}

func (e *Engine) store(table string) (*index.Store, error) {
	if _, err := e.cat.get(table); err != nil {
		return nil, err
	}
	return index.WrapTable(e.dataKV(), table)
}

func (e *Engine) createTable(s *ast.CreateTable) (*Result, error) {
	if e.inTxn() {
		return nil, fmt.Errorf("sql: DDL is not allowed inside a transaction")
	}
	cols := make([]Column, 0, len(s.Cols))
	seen := map[string]bool{}
	for _, c := range s.Cols {
		if seen[c.Name] {
			return nil, fmt.Errorf("sql: duplicate column %q", c.Name)
		}
		seen[c.Name] = true
		cols = append(cols, Column{Name: c.Name, Type: c.Type})
	}
	if err := e.cat.create(&Table{Name: s.Name, Cols: cols}); err != nil {
		return nil, err
	}
	return &Result{Message: "table created"}, nil
}

func (e *Engine) createIndex(s *ast.CreateIndex) (*Result, error) {
	if e.inTxn() {
		return nil, fmt.Errorf("sql: DDL is not allowed inside a transaction")
	}
	i, _, err := e.cat.col(s.Table, s.Column)
	if err != nil {
		return nil, err
	}
	st, err := e.store(s.Table)
	if err != nil {
		return nil, err
	}
	if err := st.CreateIndex(s.Name, i); err != nil {
		return nil, err
	}
	return &Result{Message: "index created"}, nil
}

func (e *Engine) insert(s *ast.Insert) (*Result, error) {
	tab, err := e.cat.get(s.Table)
	if err != nil {
		return nil, err
	}
	st, err := e.store(s.Table)
	if err != nil {
		return nil, err
	}
	n := 0
	for _, tup := range s.Values {
		if len(tup) != len(tab.Cols) {
			return nil, fmt.Errorf("sql: INSERT expected %d values, got %d", len(tab.Cols), len(tup))
		}
		fields := make([][]byte, len(tup))
		for i, lit := range tup {
			b, err := encodeLiteral(lit, tab.Cols[i].Type)
			if err != nil {
				return nil, err
			}
			fields[i] = b
		}
		pk := fields[0]
		if _, ok, err := st.GetRow(pk); err != nil {
			return nil, err
		} else if ok {
			return nil, fmt.Errorf("sql: duplicate primary key")
		}
		if err := st.PutRow(pk, fields); err != nil {
			return nil, err
		}
		n++
	}
	return &Result{Affected: n, Message: fmt.Sprintf("%d row(s) inserted", n)}, nil
}

type memRow struct {
	pk     []byte
	fields [][]byte
}

func (e *Engine) selectStmt(s *ast.Select) (*Result, error) {
	tab, err := e.cat.get(s.Table)
	if err != nil {
		return nil, err
	}
	st, err := e.store(s.Table)
	if err != nil {
		return nil, err
	}
	rows, err := e.gather(st, tab, s.Where)
	if err != nil {
		return nil, err
	}
	if s.OrderCol != "" {
		oi, ot, err := e.cat.col(s.Table, s.OrderCol)
		if err != nil {
			return nil, err
		}
		desc := s.OrderDesc
		sort.SliceStable(rows, func(i, j int) bool {
			a, _ := row.Field(rows[i].fields, oi)
			b, _ := row.Field(rows[j].fields, oi)
			cmp := compareStored(a, b, ot)
			if desc {
				return cmp > 0
			}
			return cmp < 0
		})
	}
	if s.Limit >= 0 && s.Limit < len(rows) {
		rows = rows[:s.Limit]
	}
	var cols []string
	var idxs []int
	if s.Star || len(s.Cols) == 0 {
		for i, c := range tab.Cols {
			cols = append(cols, c.Name)
			idxs = append(idxs, i)
		}
	} else {
		for _, name := range s.Cols {
			i, _, err := e.cat.col(s.Table, name)
			if err != nil {
				return nil, err
			}
			cols = append(cols, name)
			idxs = append(idxs, i)
		}
	}
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		line := make([]string, len(idxs))
		for j, fi := range idxs {
			f, _ := row.Field(r.fields, fi)
			line[j] = display(f, tab.Cols[fi].Type)
		}
		out = append(out, line)
	}
	return &Result{Columns: cols, Rows: out}, nil
}

func (e *Engine) update(s *ast.Update) (*Result, error) {
	tab, err := e.cat.get(s.Table)
	if err != nil {
		return nil, err
	}
	st, err := e.store(s.Table)
	if err != nil {
		return nil, err
	}
	for _, as := range s.Sets {
		if as.Col == tab.Cols[0].Name {
			return nil, fmt.Errorf("sql: cannot UPDATE primary key")
		}
	}
	rows, err := e.gather(st, tab, s.Where)
	if err != nil {
		return nil, err
	}
	n := 0
	for _, r := range rows {
		fields := cloneFields(r.fields)
		for _, as := range s.Sets {
			i, typ, err := e.cat.col(s.Table, as.Col)
			if err != nil {
				return nil, err
			}
			b, err := encodeLiteral(as.Lit, typ)
			if err != nil {
				return nil, err
			}
			fields[i] = b
		}
		if err := st.PutRow(r.pk, fields); err != nil {
			return nil, err
		}
		n++
	}
	return &Result{Affected: n, Message: fmt.Sprintf("%d row(s) updated", n)}, nil
}

func (e *Engine) delete(s *ast.Delete) (*Result, error) {
	tab, err := e.cat.get(s.Table)
	if err != nil {
		return nil, err
	}
	st, err := e.store(s.Table)
	if err != nil {
		return nil, err
	}
	rows, err := e.gather(st, tab, s.Where)
	if err != nil {
		return nil, err
	}
	n := 0
	for _, r := range rows {
		if err := st.DeleteRow(r.pk); err != nil {
			return nil, err
		}
		n++
	}
	return &Result{Affected: n, Message: fmt.Sprintf("%d row(s) deleted", n)}, nil
}

func (e *Engine) gather(st *index.Store, tab *Table, where *ast.Predicate) ([]memRow, error) {
	var rows []memRow
	if where != nil {
		ci, typ, err := e.cat.col(tab.Name, where.Col)
		if err != nil {
			return nil, err
		}
		want, err := encodeLiteral(where.Lit, typ)
		if err != nil {
			return nil, err
		}
		if ci == 0 && where.Op == ast.OpEq {
			fields, ok, err := st.GetRow(want)
			if err != nil {
				return nil, err
			}
			if ok {
				rows = append(rows, memRow{pk: want, fields: fields})
			}
			return e.filter(tab, rows, where)
		}
		if sp, ok := st.IndexOnField(ci); ok && where.Op == ast.OpEq {
			pks, err := st.Find(sp.Name, want)
			if err != nil {
				return nil, err
			}
			for _, pk := range pks {
				fields, ok, err := st.GetRow(pk)
				if err != nil {
					return nil, err
				}
				if ok {
					rows = append(rows, memRow{pk: pk, fields: fields})
				}
			}
			return e.filter(tab, rows, where)
		}
		if sp, ok := st.IndexOnField(ci); ok && (where.Op == ast.OpLt || where.Op == ast.OpLe || where.Op == ast.OpGt || where.Op == ast.OpGe) {
			var lo, hi []byte
			switch where.Op {
			case ast.OpGe, ast.OpGt:
				lo = want
			case ast.OpLe, ast.OpLt:
				hi = want
			}
			pks, err := st.RangeFind(sp.Name, lo, hi)
			if err != nil {
				return nil, err
			}
			for _, pk := range pks {
				fields, ok, err := st.GetRow(pk)
				if err != nil {
					return nil, err
				}
				if ok {
					rows = append(rows, memRow{pk: pk, fields: fields})
				}
			}
			return e.filter(tab, rows, where)
		}
	}
	err := st.ScanRows(func(pk []byte, fields [][]byte) bool {
		rows = append(rows, memRow{pk: append([]byte(nil), pk...), fields: cloneFields(fields)})
		return true
	})
	if err != nil {
		return nil, err
	}
	return e.filter(tab, rows, where)
}

func (e *Engine) filter(tab *Table, rows []memRow, where *ast.Predicate) ([]memRow, error) {
	if where == nil {
		return rows, nil
	}
	var out []memRow
	for _, r := range rows {
		ok, err := matchPred(tab, r.fields, where)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func matchPred(tab *Table, fields [][]byte, p *ast.Predicate) (bool, error) {
	for p != nil {
		i := -1
		var typ ast.Type
		for j, c := range tab.Cols {
			if c.Name == p.Col {
				i, typ = j, c.Type
				break
			}
		}
		if i < 0 {
			return false, fmt.Errorf("sql: column %q not found", p.Col)
		}
		got, ok := row.Field(fields, i)
		if !ok {
			return false, nil
		}
		want, err := encodeLiteral(p.Lit, typ)
		if err != nil {
			return false, err
		}
		if !matchOp(compareStored(got, want, typ), p.Op) {
			return false, nil
		}
		p = p.Next
	}
	return true, nil
}

func cloneFields(in [][]byte) [][]byte {
	out := make([][]byte, len(in))
	for i, f := range in {
		out[i] = append([]byte(nil), f...)
	}
	return out
}
