package agentgw

import (
	"context"
	"fmt"
)

type Risk int

const (
	RiskRead  Risk = iota // no state change
	RiskWrite             // changes state; needs Idempotency-Key
)

// Args is any operation's argument struct. Validate runs after strict JSON
// decoding (unknown fields already rejected) and must check ranges, lengths
// and formats. Return Invalid(...) errors so the model gets a useful message.
type Args interface{ Validate() error }

// Op is one allowlisted thing an agent may do. There is no "run SQL" op and
// there never should be: each op maps to a fixed, parameterized query.
type Op struct {
	Name    string
	Scope   string
	Risk    Risk
	newArgs func() Args
	handle  func(ctx context.Context, c *Claims, a Args) (any, error)
}

// Register adds a typed operation. The handler receives the verified claims,
// so user identity ALWAYS comes from c.Sub, never from model-supplied args.
func Register[A any, P interface {
	*A
	Args
}](g *Gateway, name, scope string, risk Risk, h func(ctx context.Context, c *Claims, a *A) (any, error)) {
	if _, dup := g.ops[name]; dup {
		panic("agentgw: duplicate op " + name)
	}
	g.ops[name] = Op{
		Name: name, Scope: scope, Risk: risk,
		newArgs: func() Args { return P(new(A)) },
		handle: func(ctx context.Context, c *Claims, a Args) (any, error) {
			return h(ctx, c, (*A)(a.(P)))
		},
	}
}

// UserError is safe to show to the model. Anything else becomes a generic
// "internal error" so SQL errors, table names and stack traces never leak.
type UserError struct {
	Status int
	Code   string
	Msg    string
}

func (e *UserError) Error() string { return e.Code + ": " + e.Msg }

func Invalid(format string, a ...any) error {
	return &UserError{400, "invalid_args", fmt.Sprintf(format, a...)}
}

func NotFound(format string, a ...any) error {
	return &UserError{404, "not_found", fmt.Sprintf(format, a...)}
}

func Conflict(format string, a ...any) error {
	return &UserError{409, "conflict", fmt.Sprintf(format, a...)}
}
