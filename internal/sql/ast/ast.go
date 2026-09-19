// Package ast is the SQL abstract syntax tree.
package ast

// Type is a column type.
type Type int

const (
	TypeInt Type = iota
	TypeText
)

// CmpOp is a comparison operator.
type CmpOp int

const (
	OpEq CmpOp = iota
	OpNe
	OpLt
	OpGt
	OpLe
	OpGe
)

// Stmt is a statement.
type Stmt interface{ sqlStmt() }

// ColumnDef is a CREATE TABLE column.
type ColumnDef struct {
	Name string
	Type Type
}

// CreateTable creates a relation. The first column is the primary key.
type CreateTable struct {
	Name string
	Cols []ColumnDef
}

func (*CreateTable) sqlStmt() {}

// CreateIndex creates a secondary index.
type CreateIndex struct {
	Name   string
	Table  string
	Column string
}

func (*CreateIndex) sqlStmt() {}

// Literal is a constant.
type Literal struct {
	Num    bool
	Text   string
	Int    int64
	IsNull bool
}

// Insert inserts one or more tuples.
type Insert struct {
	Table  string
	Values [][]Literal
}

func (*Insert) sqlStmt() {}

// Predicate is col op literal [AND ...].
type Predicate struct {
	Col  string
	Op   CmpOp
	Lit  Literal
	Next *Predicate
}

// Select reads rows.
type Select struct {
	Table     string
	Cols      []string // nil or empty means *
	Star      bool
	Where     *Predicate
	OrderCol  string
	OrderDesc bool
	Limit     int // -1 = none
}

func (*Select) sqlStmt() {}

// Assignment is SET col = literal.
type Assignment struct {
	Col string
	Lit Literal
}

// Update updates matching rows.
type Update struct {
	Table string
	Sets  []Assignment
	Where *Predicate
}

func (*Update) sqlStmt() {}

// Delete removes matching rows.
type Delete struct {
	Table string
	Where *Predicate
}

func (*Delete) sqlStmt() {}

// Isolation is a SQL transaction isolation level.
type Isolation int

const (
	IsoReadCommitted Isolation = iota
	IsoRepeatableRead
)

// Begin starts a transaction.
type Begin struct {
	Iso Isolation
}

func (*Begin) sqlStmt() {}

// Commit ends a transaction and makes writes visible.
type Commit struct{}

func (*Commit) sqlStmt() {}

// Rollback discards a transaction.
type Rollback struct{}

func (*Rollback) sqlStmt() {}
