package utils

import (
  "log"
  "regexp"
  "strconv"

  "gopkg.in/olivere/elastic.v5"
)

var (
  // TODO: real date parsing in real fmt!!!
  SimpleDate = regexp.MustCompile(`\d{4}[/.-]\d{2}[/.-]\d{2}`)
)

const (
  NoField = "__ERR_NO_FIELD_SET__"
  GroupInitField = "__GROUP_INIT__"
)

type RangeOp uint8
const (
  NoOp          RangeOp = iota
  LessThan
  LessThanEqual
  GreaterThan
  GreaterThanEqual
)

type Value struct {
  Q             elastic.Query
  Field         string
  RangeOp       RangeOp
  Negate        bool
}

var (
  // sentinel value marking the start of the "current" nested AND/OR clause, for stacking
  GroupInit = &Value{nil, GroupInitField, NoOp, false}
  NoQuery elastic.Query = nil
)

func NewValue(negate bool) *Value {
  return &Value{NoQuery, NoField, NoOp, negate}
}

type ValueStack struct {
  stack []*Value
}

func (vs *ValueStack) Init() {
  vs.stack = []*Value{}
}

func (vs *ValueStack) Push(v *Value) {
  vs.stack = append(vs.stack, v)
}

func (vs *ValueStack) Pop() *Value {
  if vs.Empty() {
    return nil
  }

  last := len(vs.stack) - 1
  out := vs.stack[last]
  vs.stack = vs.stack[:last]

  return out
}

// internal method used to retrieve current tmp (stub value) on top of stack
// that is being populated by successive parse steps, or a fresh value if
// no such value exists. Wraps Pop() for general use in ES query setter methods.
// each caller is expected to re-Push the value back to the stack after use/modification/
func (vs *ValueStack) current() *Value {
  peek := len(vs.stack) - 1
  if vs.Empty() || vs.stack[peek] == GroupInit || vs.stack[peek].Q != NoQuery {
    return NewValue(false)
  }

  return vs.Pop()
}

func (vs *ValueStack) Empty() bool {
  return len(vs.stack) == 0
}

// start sentinel for parens-nested groupings of AND/OR separated query elements
func (vs *ValueStack) StartGroup() {
  vs.Push(GroupInit)
}

// returns the group of values for this nested AND/OR block, and whether it was prefixed by NOT
func (vs *ValueStack) PopGroup() []*Value {
  out := []*Value{}
  next := vs.Pop()
  for next != nil && next.Field != GroupInitField {
    out = append(out, next)
    if vs.Empty() {
      break
    }
    next = vs.Pop()
  }

  return out
}

// first thing that happens in Term parsing (if present), so append a dummy vaue for filling in as we parse
func (vs *ValueStack) SetNegation() {
  vs.Push(NewValue(true))
}

// pop the tmp value stacked by SetNegation earlier, or produce
// new one if not - then fill in Field, replace on stack
func (vs *ValueStack) SetField(field string) {
  v := vs.current()
  v.Field = field
  vs.Push(v)
}

// pop the tmp value stacked by SetNegation and SetField, fill in range op, replace on stack
func (vs *ValueStack) SetRangeOp(rop string) {
  tmp := vs.current()

  switch rop {
  case ">=":
    tmp.RangeOp = GreaterThanEqual
  case "<=":
    tmp.RangeOp = LessThanEqual
  case ">":
    tmp.RangeOp = GreaterThan
  case "<":
    tmp.RangeOp = LessThan
  default:
    log.Fatalf("[ERROR] invalid range operator %q found, aborting", rop)
  }

  vs.Push(tmp)
}

func (vs *ValueStack) Boolean(value string) {
  tmp := vs.current()

  b, err := strconv.ParseBool(value)
  if err != nil {
    log.Fatalf("[ERROR] failed to parse boolean from term %q for field %q, err=%s", value, tmp.Field, err)
  }

  tmp.Q = elastic.NewTermQuery(tmp.Field, b)
  vs.Push(tmp)
}

func (vs *ValueStack) Exists() {
  tmp := vs.current()
  tmp.Q = elastic.NewExistsQuery(tmp.Field)
  vs.Push(tmp)
}


func (vs *ValueStack) Date(value string) {
  if !SimpleDate.MatchString(value) {
    log.Fatalf("[ERROR] failed to parse date string term value: %q", value)
  }
  vs.Term(value)
}

func (vs *ValueStack) Number(value string) {
  tmp := vs.current()

  i, err := strconv.Atoi(value)
  if err != nil {
    log.Fatalf("[ERROR] failed to parse invalid integer from term %q for field %q, err=%s", value, tmp.Field, err)
  }

  tmp.Q = elastic.NewTermQuery(tmp.Field, i)
  vs.Push(tmp)
}

func (vs *ValueStack) Term(term string) {
  tmp := vs.current()
  if tmp.Field == NoField {
    tmp.Field = "_all"
  }

  tmp.Q = elastic.NewTermQuery(tmp.Field, term)
  vs.Push(tmp)
}

// only used in single-value context (i.e. a not a KV)
func (vs *ValueStack) Match(text string) {
  tmp := vs.current()
  if tmp.Field == NoField {
    tmp.Field = "_all"
  }

  tmp.Q = elastic.NewMatchQuery(tmp.Field, text)
  vs.Push(tmp)
}

// only used in single-value (quoted phrase) context (i.e. not a KV)
func (vs *ValueStack) Phrase(phrase string) {
  tmp := vs.current()
  if tmp.Field == NoField {
    tmp.Field = "_all"
  }

  tmp.Q = elastic.NewMatchPhraseQuery(tmp.Field, phrase)
  vs.Push(tmp)
}

func (vs *ValueStack) RangeOrNumber(value string) {
  // if this isn't an in-progress KV parse of a range, its a number, just pass the value along
  switch vs.Empty() || vs.stack[len(vs.stack) - 1].RangeOp == NoOp {
  case true:
    vs.Number(value)
  case false:
    vs.Range(value)
  }
}

func (vs *ValueStack) RangeOrDate(value string) {
  // if this isn't an in-progress KV parse of a range, its a number, just pass the value along
  switch vs.Empty() || vs.stack[len(vs.stack) - 1].RangeOp == NoOp {
  case true:
    vs.Date(value)
  case false:
    vs.Range(value)
  }
}

// value accepts dates "YYYY/MM/DD" or integers "-93"
func (vs *ValueStack) Range(value string) {
  tmp := vs.current()
  rq := elastic.NewRangeQuery(tmp.Field)

  // if this is an in-progress range parse, the value could be a date or a number - check both
  var v interface{}
  var err error

  if SimpleDate.MatchString(value) {
    v = value
  } else if v, err = strconv.Atoi(value); err != nil {
    log.Fatalf("[ERROR] couldn't parse valid integer for range value %q for field %q, err=%s", value, tmp.Field, err)
  }

  switch tmp.RangeOp {
  case LessThan:
    rq.Lt(v)
  case LessThanEqual:
    rq.Lte(v)
  case GreaterThan:
    rq.Gt(v)
  case GreaterThanEqual:
    rq.Gte(v)
  default:
    log.Fatalf("[ERROR] invalid range operation (code %d) parsing range value %q for field %q", tmp.RangeOp, value, tmp.Field)
  }
  tmp.Q = rq

  vs.Push(tmp)
}

