package lsp

import (
	"bytes"
	"fmt"
	"math"
	"strings"
)

// KernelState holds the executable state for a Lua file, including:
// - Parse tree
// - Analysis results
// - Runtime state (symbol table with values)
// - Execution history for code tracking
type KernelState struct {
	URI      string
	Text     string
	Version  int
	Block    *Block
	Analysis *Analysis

	// Runtime state for execution-based LSP
	Env     *Env
	history ExecutionHistory
}

// Env holds runtime variable values
type Env struct {
	Parent  *Env
	Symbols map[string]*EnvValue
}

// EnvValue represents a runtime value in the kernel state
type EnvValue struct {
	Kind  ValueKind
	Value interface{}
	Def   Span   // definition span
	Refs  []Span // all reference spans
}

// ValueKind classifies Lua value types
type ValueKind int

const (
	ValueUnknown ValueKind = iota
	ValueNil
	ValueBoolean
	ValueNumber
	ValueString
	ValueTable
	ValueFunction
)

// ExecutionHistory tracks executed code for introspection
type ExecutionHistory struct {
	StatementsExecuted int
	Assignments        []AssignmentRecord
	FunctionCalls      []CallRecord
	GlobalsModified    map[string][]Span
}

// AssignmentRecord tracks a variable assignment
type AssignmentRecord struct {
	Span    Span
	Name    string
	Kind    ValueKind
	Value   interface{}
	EnvPath []string // scope chain
}

// CallRecord tracks a function call
type CallRecord struct {
	Span     Span
	FuncName string
	Args     []interface{}
	Results  []interface{}
}

// NewKernelState creates a new kernel state for a document
func NewKernelState(uri string, text string, version int) *KernelState {
	return &KernelState{
		URI:     uri,
		Text:    text,
		Version: version,
		Env:     NewEnv(nil),
		history: ExecutionHistory{GlobalsModified: make(map[string][]Span)},
	}
}

// NewEnv creates a new environment with optional parent
func NewEnv(parent *Env) *Env {
	return &Env{
		Parent:  parent,
		Symbols: make(map[string]*EnvValue),
	}
}

// Lookup searches for a value in the environment chain
func (e *Env) Lookup(name string) (*EnvValue, bool) {
	if val, ok := e.Symbols[name]; ok {
		return val, true
	}
	if e.Parent != nil {
		return e.Parent.Lookup(name)
	}
	return nil, false
}

// Set assigns a value in the current environment
func (e *Env) Set(name string, val *EnvValue) {
	e.Symbols[name] = val
}

// DefSet assigns a value at definition time (always current env)
func (e *Env) DefSet(name string, val *EnvValue) {
	e.Symbols[name] = val
}

// Clone creates a shallow copy of the environment
func (e *Env) Clone() *Env {
	newEnv := &Env{
		Parent:  e.Parent,
		Symbols: make(map[string]*EnvValue),
	}
	for k, v := range e.Symbols {
		newEnv.Symbols[k] = v
	}
	return newEnv
}

// Reset clears runtime state while preserving structure
func (k *KernelState) Reset() {
	k.Env = NewEnv(nil)
	k.history = ExecutionHistory{
		GlobalsModified: make(map[string][]Span),
	}
}

// ExecuteBlock evaluates a block with runtime state tracking
func (k *KernelState) ExecuteBlock(block *Block) error {
	return k.executeBlock(block, k.Env, nil)
}

func (k *KernelState) executeBlock(block *Block, env *Env, returnLabels []string) error {
	for _, stmt := range block.Stmts {
		if err := k.executeStmt(stmt, env, returnLabels); err != nil {
			return err
		}
	}
	if block.Ret != nil {
		return k.executeStmt(block.Ret, env, returnLabels)
	}
	return nil
}

func (k *KernelState) executeStmt(stmt Stmt, env *Env, returnLabels []string) error {
	switch s := stmt.(type) {
	case *AssignStmt:
		return k.executeAssign(s, env)
	case *LocalStmt:
		return k.executeLocal(s, env)
	case *DoStmt:
		return k.executeBlock(s.Body, env, returnLabels)
	case *WhileStmt:
		return k.executeWhile(s, env, returnLabels)
	case *RepeatStmt:
		return k.executeRepeat(s, env, returnLabels)
	case *IfStmt:
		return k.executeIf(s, env, returnLabels)
	case *ForNumStmt:
		return k.executeForNum(s, env, returnLabels)
	case *ForInStmt:
		return k.executeForIn(s, env, returnLabels)
	case *FuncStmt:
		return k.executeFuncStmt(s, env)
	case *ReturnStmt:
		return k.executeReturn(s, env, returnLabels)
	case *BreakStmt:
		return &BreakError{}
	case *GotoStmt:
		return k.executeGoto(s, env, returnLabels)
	case *LabelStmt:
		// Labels just mark positions, no execution needed
		return nil
	case *ExprStmt:
		return k.executeExpr(s.X, env)
	}
	return nil
}

type BreakError struct{}

func (b BreakError) Error() string { return "break" }

func (k *KernelState) executeAssign(stmt *AssignStmt, env *Env) error {
	// Evaluate all values first
	values := make([]interface{}, len(stmt.Values))
	for i, v := range stmt.Values {
		val, err := k.evalExpr(v, env)
		if err != nil {
			return err
		}
		values[i] = val
	}

	// Handle multiple assignments with possible nils
	valueIdx := 0
	for _, target := range stmt.Targets {
		if valueIdx >= len(values) {
			values = append(values, nil)
		}

		switch t := target.(type) {
		case *NameExpr:
			val := values[valueIdx]
			env.Set(t.Name, &EnvValue{
				Kind:  inferKind(val),
				Value: val,
			})
			k.history.Assignments = append(k.history.Assignments, AssignmentRecord{
				Span:    t.Sp,
				Name:    t.Name,
				Kind:    inferKind(val),
				Value:   val,
				EnvPath: getEnvPath(env),
			})
			valueIdx++
		case *IndexExpr:
			tableVal, err := k.evalExpr(t.Table, env)
			if err != nil {
				return err
			}
			if table, ok := tableVal.(*EnvTable); ok {
				key, err := k.evalExpr(t.Key, env)
				if err != nil {
					return err
				}
				table.Set(key, values[valueIdx])
				valueIdx++
			}
		}
	}

	return nil
}

func (k *KernelState) executeLocal(stmt *LocalStmt, env *Env) error {
	for i, name := range stmt.Names {
		var val interface{}
		if i < len(stmt.Values) {
			v, err := k.evalExpr(stmt.Values[i], env)
			if err != nil {
				return err
			}
			val = v
		}

		env.DefSet(name.Name, &EnvValue{
			Kind:  inferKind(val),
			Value: val,
			Def:   name.Sp,
			Refs:  []Span{name.Sp},
		})

		k.history.Assignments = append(k.history.Assignments, AssignmentRecord{
			Span:    name.Sp,
			Name:    name.Name,
			Kind:    inferKind(val),
			Value:   val,
			EnvPath: getEnvPath(env),
		})
	}

	return nil
}

func (k *KernelState) executeWhile(stmt *WhileStmt, env *Env, returnLabels []string) error {
	for {
		cond, err := k.evalExpr(stmt.Cond, env)
		if err != nil {
			if _, ok := err.(*BreakError); ok {
				return nil
			}
			return err
		}

		if !isTruthy(cond) {
			break
		}

		err = k.executeBlock(stmt.Body, env, returnLabels)
		if err != nil {
			if _, ok := err.(*BreakError); ok {
				return nil
			}
			return err
		}
	}
	return nil
}

func (k *KernelState) executeRepeat(stmt *RepeatStmt, env *Env, returnLabels []string) error {
	for {
		err := k.executeBlock(stmt.Body, env, returnLabels)
		if err != nil {
			if _, ok := err.(*BreakError); ok {
				return nil
			}
			return err
		}

		cond, err := k.evalExpr(stmt.Cond, env)
		if err != nil {
			return err
		}

		if isTruthy(cond) {
			break
		}
	}
	return nil
}

func (k *KernelState) executeIf(stmt *IfStmt, env *Env, returnLabels []string) error {
	for _, clause := range stmt.Clauses {
		cond, err := k.evalExpr(clause.Cond, env)
		if err != nil {
			return err
		}

		if isTruthy(cond) {
			return k.executeBlock(clause.Body, env, returnLabels)
		}
	}

	if stmt.ElseBody != nil {
		return k.executeBlock(stmt.ElseBody, env, returnLabels)
	}

	return nil
}

func (k *KernelState) executeForNum(stmt *ForNumStmt, env *Env, returnLabels []string) error {
	start, err := k.evalExpr(stmt.Start, env)
	if err != nil {
		return err
	}
	limit, err := k.evalExpr(stmt.Limit, env)
	if err != nil {
		return err
	}

	var step float64 = 1
	if stmt.Step != nil {
		s, err := k.evalExpr(stmt.Step, env)
		if err != nil {
			return err
		}
		step = toNumber(s)
	}

	name := stmt.Name.Name
	env.DefSet(name, &EnvValue{Kind: ValueNumber, Value: start, Def: stmt.Name.Sp})

	limitFloat := toNumber(limit)
	for i := toNumber(start); compareFloat(i, limitFloat, step > 0); i += step {
		env.Symbols[name].Value = i
		env.Symbols[name].Value = i

		err = k.executeBlock(stmt.Body, env, returnLabels)
		if err != nil {
			if _, ok := err.(*BreakError); ok {
				return nil
			}
			return err
		}
	}

	return nil
}

func (k *KernelState) executeForIn(stmt *ForInStmt, env *Env, returnLabels []string) error {
	values := make([]interface{}, len(stmt.Values))
	for i, v := range stmt.Values {
		val, err := k.evalExpr(v, env)
		if err != nil {
			return err
		}
		values[i] = val
	}

	iter := createIterator(values)

	for {
		nextVals := iter()
		if len(nextVals) == 0 || nextVals[0] == nil {
			break
		}

		for i, name := range stmt.Names {
			if i < len(nextVals) {
				env.DefSet(name.Name, &EnvValue{
					Kind:  inferKind(nextVals[i]),
					Value: nextVals[i],
					Def:   name.Sp,
				})
			}
		}

		err := k.executeBlock(stmt.Body, env, returnLabels)
		if err != nil {
			if _, ok := err.(*BreakError); ok {
				return nil
			}
			return err
		}
	}

	return nil
}

func (k *KernelState) executeFuncStmt(stmt *FuncStmt, env *Env) error {
	var name string
	var defSpan Span

	if stmt.IsLocal {
		name = stmt.LocalName.Name
		defSpan = stmt.LocalName.Sp
	} else {
		name = stmt.Name.Parts[0].Name
		defSpan = stmt.Name.Parts[0].Sp
	}

	funcVal := &EnvFunction{
		Params:    stmt.Func.Params,
		Body:      stmt.Func.Body,
		HasVarArg: stmt.Func.HasVarArg,
		Env:       env,
	}

	env.DefSet(name, &EnvValue{
		Kind:  ValueFunction,
		Value: funcVal,
		Def:   defSpan,
		Refs:  []Span{defSpan},
	})

	k.history.Assignments = append(k.history.Assignments, AssignmentRecord{
		Span:    defSpan,
		Name:    name,
		Kind:    ValueFunction,
		Value:   funcVal,
		EnvPath: getEnvPath(env),
	})

	return nil
}

func (k *KernelState) executeReturn(stmt *ReturnStmt, env *Env, returnLabels []string) error {
	values := make([]interface{}, len(stmt.Values))
	for i, v := range stmt.Values {
		val, err := k.evalExpr(v, env)
		if err != nil {
			return err
		}
		values[i] = val
	}
	return &ReturnError{Values: values}
}

type ReturnError struct {
	Values []interface{}
}

func (r ReturnError) Error() string { return "return" }

func (k *KernelState) executeGoto(stmt *GotoStmt, env *Env, returnLabels []string) error {
	// Gotos are statically resolved during analysis
	return nil
}

func (k *KernelState) executeExpr(expr Expr, env *Env) error {
	_, err := k.evalExpr(expr, env)
	return err
}

func (k *KernelState) evalExpr(expr Expr, env *Env) (interface{}, error) {
	if expr == nil {
		return nil, nil
	}

	switch e := expr.(type) {
	case *NilExpr:
		return nil, nil
	case *TrueExpr:
		return true, nil
	case *FalseExpr:
		return false, nil
	case *NumberExpr:
		return e.Value, nil
	case *StringExpr:
		return e.Value, nil
	case *VarArgExpr:
		return nil, nil // vararg handled specially
	case *NameExpr:
		if val, ok := env.Lookup(e.Name); ok {
			return val.Value, nil
		}
		return nil, nil
	case *IndexExpr:
		tableVal, err := k.evalExpr(e.Table, env)
		if err != nil {
			return nil, err
		}
		key, err := k.evalExpr(e.Key, env)
		if err != nil {
			return nil, err
		}
		if table, ok := tableVal.(*EnvTable); ok {
			return table.Get(key), nil
		}
		return nil, nil
	case *FieldExpr:
		tableVal, err := k.evalExpr(e.Table, env)
		if err != nil {
			return nil, err
		}
		if table, ok := tableVal.(*EnvTable); ok {
			return table.Get(e.Field.Name), nil
		}
		return nil, nil
	case *UnaryExpr:
		operand, err := k.evalExpr(e.Operand, env)
		if err != nil {
			return nil, err
		}
		return k.evalUnary(e.Op, operand)
	case *BinaryExpr:
		left, err := k.evalExpr(e.Left, env)
		if err != nil {
			return nil, err
		}
		right, err := k.evalExpr(e.Right, env)
		if err != nil {
			return nil, err
		}
		return k.evalBinary(e.Op, left, right)
	case *CallExpr:
		return k.evalCall(e, env)
	case *FuncExpr:
		return &EnvFunction{
			Params:    e.Params,
			Body:      e.Body,
			HasVarArg: e.HasVarArg,
			Env:       env,
		}, nil
	case *TableExpr:
		return k.evalTable(e, env)
	case *ParenExpr:
		return k.evalExpr(e.Inner, env)
	}

	return nil, nil
}

func (k *KernelState) evalUnary(op string, operand interface{}) (interface{}, error) {
	switch op {
	case "not":
		return !isTruthy(operand), nil
	case "-":
		if n, ok := operand.(float64); ok {
			return -n, nil
		}
		return nil, fmt.Errorf("cannot negate non-numeric value")
	case "#":
		switch v := operand.(type) {
		case *EnvTable:
			return float64(v.Len()), nil
		case string:
			return float64(len(v)), nil
		default:
			return nil, fmt.Errorf("cannot get length of %T", operand)
		}
	}
	return nil, fmt.Errorf("unknown unary operator: %s", op)
}

func (k *KernelState) evalBinary(op string, left, right interface{}) (interface{}, error) {
	switch op {
	case "+":
		return arith(left, right, func(a, b float64) float64 { return a + b }), nil
	case "-":
		return arith(left, right, func(a, b float64) float64 { return a - b }), nil
	case "*":
		return arith(left, right, func(a, b float64) float64 { return a * b }), nil
	case "/":
		return arith(left, right, func(a, b float64) float64 { return a / b }), nil
	case "%":
		return arith(left, right, func(a, b float64) float64 { return float64(int(a) % int(b)) }), nil
	case "^":
		return arith(left, right, math.Pow), nil
	case "..":
		lstr := toString(left)
		rstr := toString(right)
		return lstr + rstr, nil
	case "==":
		return equal(left, right), nil
	case "~=":
		return !equal(left, right), nil
	case "<":
		return compare(left, right, func(a, b float64) bool { return a < b }), nil
	case "<=":
		return compare(left, right, func(a, b float64) bool { return a <= b }), nil
	case ">":
		return compare(left, right, func(a, b float64) bool { return a > b }), nil
	case ">=":
		return compare(left, right, func(a, b float64) bool { return a >= b }), nil
	case "and":
		if !isTruthy(left) {
			return left, nil
		}
		return right, nil
	case "or":
		if isTruthy(left) {
			return left, nil
		}
		return right, nil
	}
	return nil, fmt.Errorf("unknown binary operator: %s", op)
}

func (k *KernelState) evalCall(call *CallExpr, env *Env) (interface{}, error) {
	var funcVal interface{}
	var funcName string

	switch f := call.Func.(type) {
	case *NameExpr:
		if val, ok := env.Lookup(f.Name); ok {
			funcVal = val.Value
			funcName = f.Name
		}
	case *FieldExpr:
		tableVal, err := k.evalExpr(f.Table, env)
		if err != nil {
			return nil, err
		}
		if table, ok := tableVal.(*EnvTable); ok {
			funcVal = table.Get(f.Field.Name)
			funcName = f.Field.Name
		}
	}

	if funcVal == nil {
		return nil, fmt.Errorf("undefined function: %s", funcName)
	}

	// Evaluate arguments
	args := make([]interface{}, len(call.Args))
	for i, arg := range call.Args {
		val, err := k.evalExpr(arg, env)
		if err != nil {
			return nil, err
		}
		args[i] = val
	}

	// Call the function
	switch v := funcVal.(type) {
	case *EnvFunction:
		return k.callFunction(v, args, env)
	case *EnvBuiltin:
		return v.Func(args)
	default:
		return nil, fmt.Errorf("cannot call %T", funcVal)
	}
}

func (k *KernelState) callFunction(fn *EnvFunction, args []interface{}, callerEnv *Env) (interface{}, error) {
	// Create new environment for function
	newEnv := NewEnv(fn.Env)

	// Bind parameters
	for i, param := range fn.Params {
		if i < len(args) {
			newEnv.DefSet(param.Name, &EnvValue{
				Kind:  inferKind(args[i]),
				Value: args[i],
				Def:   param.Sp,
			})
		} else {
			newEnv.DefSet(param.Name, &EnvValue{Kind: ValueNil, Value: nil, Def: param.Sp})
		}
	}

	// Execute function body
	returnErr := k.executeBlock(fn.Body, newEnv, nil)

	if retErr, ok := returnErr.(*ReturnError); ok {
		if len(retErr.Values) == 0 {
			return nil, nil
		}
		if len(retErr.Values) == 1 {
			return retErr.Values[0], nil
		}
		return retErr.Values, nil
	}

	if returnErr != nil {
		return nil, returnErr
	}

	return nil, nil
}

func (k *KernelState) evalTable(expr *TableExpr, env *Env) (*EnvTable, error) {
	table := NewEnvTable()

	for _, field := range expr.Fields {
		var key, value interface{}

		if field.Key != nil {
			k, err := k.evalExpr(field.Key, env)
			if err != nil {
				return nil, err
			}
			key = k
		} else if field.Name != nil {
			key = field.Name.Name
		}

		v, err := k.evalExpr(field.Value, env)
		if err != nil {
			return nil, err
		}
		value = v

		if key != nil {
			table.Set(key, value)
		} else {
			table.Append(value)
		}
	}

	return table, nil
}

// EnvTable is a runtime table representation
type EnvTable struct {
	elements map[interface{}]interface{}
	keys     []interface{}
}

func NewEnvTable() *EnvTable {
	return &EnvTable{
		elements: make(map[interface{}]interface{}),
		keys:     make([]interface{}, 0),
	}
}

func (t *EnvTable) Get(key interface{}) interface{} {
	return t.elements[key]
}

func (t *EnvTable) Set(key, value interface{}) {
	if _, exists := t.elements[key]; !exists {
		t.keys = append(t.keys, key)
	}
	t.elements[key] = value
}

func (t *EnvTable) Append(value interface{}) {
	t.keys = append(t.keys, len(t.keys))
	t.elements[len(t.keys)-1] = value
}

func (t *EnvTable) Len() int {
	return len(t.keys)
}

// EnvFunction represents a Lua function in the kernel
type EnvFunction struct {
	Params    []*Ident
	Body      *Block
	HasVarArg bool
	Env       *Env
}

// EnvBuiltin represents a built-in Go function
type EnvBuiltin struct {
	Func func([]interface{}) (interface{}, error)
}

// EnvTableBuilder is for creating test tables
type EnvTableBuilder struct {
	*EnvTable
}

func (b *EnvTableBuilder) Add(key, value interface{}) *EnvTableBuilder {
	b.Set(key, value)
	return b
}

func (b *EnvTableBuilder) AddNum(key string, value float64) *EnvTableBuilder {
	b.Set(key, value)
	return b
}

func (b *EnvTableBuilder) AddStr(key string, value string) *EnvTableBuilder {
	b.Set(key, value)
	return b
}

func (b *EnvTableBuilder) AddTable(key string, child *EnvTable) *EnvTableBuilder {
	b.Set(key, child)
	return b
}

// Conveniences for value handling
func inferKind(val interface{}) ValueKind {
	if val == nil {
		return ValueNil
	}
	switch val.(type) {
	case bool:
		return ValueBoolean
	case float64:
		return ValueNumber
	case string:
		return ValueString
	case *EnvTable:
		return ValueTable
	case *EnvFunction:
		return ValueFunction
	case *EnvBuiltin:
		return ValueFunction
	}
	return ValueUnknown
}

func isTruthy(val interface{}) bool {
	if val == nil {
		return false
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return true
}

func toNumber(val interface{}) float64 {
	if n, ok := val.(float64); ok {
		return n
	}
	return 0
}

func equal(left, right interface{}) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return left == right
}

func compare(left, right interface{}, cmp func(float64, float64) bool) bool {
	l, lok := left.(float64)
	r, rok := right.(float64)
	if lok && rok {
		return cmp(l, r)
	}
	return false
}

func arith(left, right interface{}, op func(float64, float64) float64) float64 {
	l := toNumber(left)
	r := toNumber(right)
	return op(l, r)
}

func toString(val interface{}) string {
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", val)
}

func getEnvPath(env *Env) []string {
	path := []string{}
	current := env
	for current != nil {
		path = append([]string{"<root>"}, path...)
		current = current.Parent
	}
	return path
}

// compareFloat compares with consideration for step direction
func compareFloat(val, limit float64, positiveStep bool) bool {
	if positiveStep {
		return val <= limit
	}
	return val >= limit
}

// createIterator creates an iterator function from values
func createIterator(values []interface{}) func() []interface{} {
	idx := 0
	return func() []interface{} {
		if idx >= len(values) {
			return nil
		}
		val := values[idx]
		idx++
		return []interface{}{idx - 1, val}
	}
}

// ExecuteWithTrace executes a statement and records the trace
func (k *KernelState) ExecuteWithTrace(stmt Stmt) error {
	oldHistory := k.history
	defer func() {
		k.history = oldHistory
	}()

	k.history = ExecutionHistory{
		GlobalsModified: make(map[string][]Span),
	}

	return k.executeStmt(stmt, k.Env, nil)
}

// GetVariableValue returns the runtime value of a variable
func (k *KernelState) GetVariableValue(name string) (*EnvValue, bool) {
	return k.Env.Lookup(name)
}

// GetVariableValueAt returns the value at a specific offset
func (k *KernelState) GetVariableValueAt(offset int) (*EnvValue, bool) {
	for _, assignment := range k.history.Assignments {
		if assignment.Span.Contains(Pos{Offset: offset}) {
			return &EnvValue{
				Kind:  assignment.Kind,
				Value: assignment.Value,
			}, true
		}
	}
	return nil, false
}

// GetFunctionAt returns function info at offset
func (k *KernelState) GetFunctionAt(offset int) (*EnvFunction, bool) {
	for _, assignment := range k.history.Assignments {
		if assignment.Span.Contains(Pos{Offset: offset}) && assignment.Kind == ValueFunction {
			if fn, ok := assignment.Value.(*EnvFunction); ok {
				return fn, true
			}
		}
	}
	return nil, false
}

// GetExecutionHistory returns the recorded execution history
func (k *KernelState) GetExecutionHistory() ExecutionHistory {
	return k.history
}

// Update parses and analyzes new text, preserving as much state as possible
func (k *KernelState) Update(text string, version int) error {
	k.Text = text
	k.Version = version
	k.history = ExecutionHistory{GlobalsModified: make(map[string][]Span)}

	p := NewParser(text)
	block, errs := p.Parse()

	if len(errs) > 0 {
		return fmt.Errorf("parse error")
	}

	k.Block = block
	a := Analyze(block)
	k.Analysis = a

	// Re-initialize environment with global scope
	k.Env = NewEnv(nil)

	// Execute initial block to populate state
	if block != nil {
		k.ExecuteBlock(block)
	}

	return nil
}

// ExecuteString executes a string of Lua code in the kernel state
func (k *KernelState) ExecuteString(code string) ([]interface{}, error) {
	p := NewParser(code)
	block, errs := p.Parse()

	if len(errs) > 0 {
		return nil, fmt.Errorf("parse error")
	}

	originalBlock := k.Block
	defer func() {
		k.Block = originalBlock
	}()

	k.Block = block

	err := k.ExecuteBlock(block)
	if err != nil {
		return nil, err
	}

	return []interface{}{nil}, nil
}

// PrintEnv prints the current environment for debugging
func (k *KernelState) PrintEnv() string {
	var buf bytes.Buffer
	printEnvRecursive(&buf, k.Env, 0)
	return buf.String()
}

func printEnvRecursive(buf *bytes.Buffer, env *Env, indent int) {
	prefix := strings.Repeat("  ", indent)
	buf.WriteString(prefix + "Env:\n")
	for name, val := range env.Symbols {
		buf.WriteString(prefix + "  " + name + " = ")
		printValue(buf, val.Value, indent+1)
		buf.WriteString("\n")
	}
	if env.Parent != nil {
		printEnvRecursive(buf, env.Parent, indent+1)
	}
}

func printValue(buf *bytes.Buffer, val interface{}, indent int) {
	switch v := val.(type) {
	case nil:
		buf.WriteString("nil")
	case string:
		buf.WriteString(fmt.Sprintf("%q", v))
	case bool:
		if v {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case float64:
		buf.WriteString(fmt.Sprintf("%.2f", v))
	case *EnvTable:
		buf.WriteString("table")
	case *EnvFunction:
		buf.WriteString("function")
	default:
		buf.WriteString(fmt.Sprintf("%T", v))
	}
}
