package gash

import (
	"fmt"
	"strings"
	"unicode"

	"goblin/pkg/contract"
)

// tokenizeExpr separa una condición en tokens, respetando comillas y llaves.
func tokenizeExpr(s string) []string {
	var tokens []string
	var current strings.Builder
	var quote rune

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for _, r := range s {
		if quote != 0 {
			current.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			current.WriteRune(r)
			quote = r
		case '{', '}':
			flush()
			tokens = append(tokens, string(r))
		default:
			if unicode.IsSpace(r) {
				flush()
			} else {
				current.WriteRune(r)
			}
		}
	}
	flush()
	return tokens
}

// exprParser es un parser recursivo descendente. No hay precedencia
// implícita: las llaves son la única agrupación, tal como fija el diseño.
type exprParser struct {
	tokens []string
	pos    int
}

func (p *exprParser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return ""
}

func (p *exprParser) next() string {
	t := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return t
}

func (p *exprParser) parse() (contract.Expr, error) {
	e, err := p.parseOr()
	if err != nil {
		return contract.Expr{}, err
	}
	if p.pos != len(p.tokens) {
		return contract.Expr{}, fmt.Errorf("tokens sobrantes: %v", p.tokens[p.pos:])
	}
	return e, nil
}

func (p *exprParser) parseOr() (contract.Expr, error) {
	left, err := p.parseXor()
	if err != nil {
		return contract.Expr{}, err
	}
	for p.peek() == "or" {
		p.next()
		right, err := p.parseXor()
		if err != nil {
			return contract.Expr{}, err
		}
		left = contract.Expr{Op: "or", Args: []contract.Expr{left, right}}
	}
	return left, nil
}

func (p *exprParser) parseXor() (contract.Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return contract.Expr{}, err
	}
	for p.peek() == "xor" {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return contract.Expr{}, err
		}
		left = contract.Expr{Op: "xor", Args: []contract.Expr{left, right}}
	}
	return left, nil
}

func (p *exprParser) parseAnd() (contract.Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return contract.Expr{}, err
	}
	for p.peek() == "and" {
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return contract.Expr{}, err
		}
		left = contract.Expr{Op: "and", Args: []contract.Expr{left, right}}
	}
	return left, nil
}

func (p *exprParser) parseUnary() (contract.Expr, error) {
	switch p.peek() {
	case "not":
		p.next()
		inner, err := p.parseUnary()
		if err != nil {
			return contract.Expr{}, err
		}
		return contract.Expr{Op: "not", Args: []contract.Expr{inner}}, nil
	case "{":
		p.next()
		inner, err := p.parseOr()
		if err != nil {
			return contract.Expr{}, err
		}
		if p.peek() != "}" {
			return contract.Expr{}, fmt.Errorf("falta } en la expresión")
		}
		p.next()
		return inner, nil
	default:
		return p.parsePredicate()
	}
}

func (p *exprParser) parsePredicate() (contract.Expr, error) {
	tok := p.next()
	switch tok {
	case "all", "extreme", "oom":
		return contract.Expr{Op: tok}, nil
	case "and", "or", "xor":
		// Forma prefija dentro de llaves: {and A B}.
		var args []contract.Expr
		for p.pos < len(p.tokens) && p.peek() != "}" {
			arg, err := p.parseUnary()
			if err != nil {
				return contract.Expr{}, err
			}
			args = append(args, arg)
		}
		return contract.Expr{Op: tok, Args: args}, nil
	case "not":
		inner, err := p.parseUnary()
		if err != nil {
			return contract.Expr{}, err
		}
		return contract.Expr{Op: "not", Args: []contract.Expr{inner}}, nil
	case "match":
		pattern := strings.Trim(p.next(), "'\"")
		stream := ""
		if p.peek() == "in" {
			p.next()
			stream = p.next()
		}
		return contract.Expr{Op: "match", Value: pattern, Stream: stream}, nil
	case "gte", "gt", "lte", "lt", "eq", "neq":
		fact, level, err := p.parseFact()
		if err != nil {
			return contract.Expr{}, err
		}
		value := p.next()
		if value == "" {
			return contract.Expr{}, fmt.Errorf("comparación sin valor")
		}
		if strings.HasPrefix(fact, "filesize:") {
			// La ruta va en Level; Value queda para el comparador.
			level = strings.TrimPrefix(fact, "filesize:")
			fact = "filesize"
		}
		return contract.Expr{Op: tok, Fact: fact, Level: level, Value: value}, nil
	default:
		return contract.Expr{}, fmt.Errorf("predicado desconocido: %s", tok)
	}
}

// parseFact lee un hecho, con soporte para mempeak(worker) y filesize(path).
func (p *exprParser) parseFact() (fact, level string, err error) {
	tok := p.next()
	if tok == "" {
		return "", "", fmt.Errorf("comparación sin hecho")
	}
	if open := strings.IndexByte(tok, '('); open >= 0 {
		base := tok[:open]
		arg := strings.TrimSuffix(tok[open+1:], ")")
		if base == "filesize" {
			return "filesize:" + arg, "", nil
		}
		return base, arg, nil
	}
	return tok, "", nil
}

// ParseExpr publica el parser para usar desde tests.
func ParseExpr(s string) (contract.Expr, error) {
	p := &exprParser{tokens: tokenizeExpr(s)}
	return p.parse()
}
