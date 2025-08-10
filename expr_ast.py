from __future__ import annotations
from dataclasses import dataclass
from typing import List, Optional, Tuple, Dict, Any


# --- Tokens ---

@dataclass
class Token:
    kind: str
    text: str


def tokenize(src: str) -> List[Token]:
    tokens: List[Token] = []
    i = 0
    n = len(src)
    while i < n:
        ch = src[i]
        # Skip whitespace
        if ch.isspace():
            i += 1
            continue
        # Identifiers
        if ch.isalpha() or ch == '_':
            start = i
            i += 1
            while i < n and (src[i].isalnum() or src[i] == '_'):
                i += 1
            tokens.append(Token('ident', src[start:i]))
            continue
        # Numbers (very simple, keep spelling)
        if ch.isdigit():
            start = i
            i += 1
            while i < n and (src[i].isalnum() or src[i] in '.xX'):
                i += 1
            tokens.append(Token('number', src[start:i]))
            continue
        # Strings
        if ch == '"':
            start = i
            i += 1
            while i < n:
                if src[i] == '\\':
                    i += 2
                    continue
                if src[i] == '"':
                    i += 1
                    break
                i += 1
            tokens.append(Token('string', src[start:i]))
            continue
        # Two-char operators
        if i + 1 < n and src[i:i+2] in ("<<", ">>", "<=", ">=", "==", "!=", "&&", "||"):
            tokens.append(Token('op', src[i:i+2]))
            i += 2
            continue
        # Single characters
        if ch in '{}()[],=+-*/%&|^~!<>?:.':
            kind = 'brace' if ch in '{}' else ('paren' if ch in '()' else ('bracket' if ch in '[]' else ('comma' if ch == ',' else ('assign' if ch == '=' else 'op'))))
            tokens.append(Token(kind, ch))
            i += 1
            continue
        # Fallback: treat as op
        tokens.append(Token('op', ch))
        i += 1
    return tokens


# --- AST Nodes ---

class Expr: ...

@dataclass
class Number(Expr):
    text: str

@dataclass
class String(Expr):
    text: str

@dataclass
class Identifier(Expr):
    name: str

@dataclass
class Unary(Expr):
    op: str
    expr: Expr

@dataclass
class Binary(Expr):
    left: Expr
    op: str
    right: Expr

@dataclass
class Call(Expr):
    func: Identifier
    args: List[Expr]

@dataclass
class InitItem:
    field: Optional[str]  # designated field or None
    value: Expr

@dataclass
class CompoundLiteral(Expr):
    type_name: str
    items: List[InitItem]


# --- Parser ---

class Parser:
    def __init__(self, tokens: List[Token]):
        self.toks = tokens
        self.i = 0

    def _peek(self, k: int = 0) -> Optional[Token]:
        j = self.i + k
        return self.toks[j] if 0 <= j < len(self.toks) else None

    def _eat(self, kind: Optional[str] = None, text: Optional[str] = None) -> Optional[Token]:
        t = self._peek()
        if not t:
            return None
        if kind is not None and t.kind != kind:
            return None
        if text is not None and t.text != text:
            return None
        self.i += 1
        return t

    def parse(self) -> Optional[Expr]:
        # Special-case CLITERAL(Type){...}
        t0 = self._peek()
        if t0 is not None and t0.kind == 'ident' and t0.text == 'CLITERAL':
            return self._parse_cliteral()
        # Compound literal forms
        t0 = self._peek()
        if t0 is not None and t0.kind == 'paren' and t0.text == '(':
            # (Type){...}
            save = self.i
            self._eat('paren', '(')
            type_id = self._eat('ident')
            if type_id is not None and self._eat('paren', ')') and self._eat('brace', '{'):
                items = self._parse_init_list()
                if self._eat('brace', '}'):
                    return CompoundLiteral(type_id.text, items)
            self.i = save
        # Type{...}
        t0 = self._peek()
        if t0 is not None and t0.kind == 'ident':
            save = self.i
            type_id = self._eat('ident')
            if type_id is not None and self._eat('brace', '{'):
                items = self._parse_init_list()
                if self._eat('brace', '}'):
                    return CompoundLiteral(type_id.text, items)
            self.i = save
        # Generic expression
        return self._parse_expr()

    def _parse_cliteral(self) -> Optional[Expr]:
        if not (self._eat('ident', 'CLITERAL') and self._eat('paren', '(')):
            return None
        type_id = self._eat('ident')
        if not type_id:
            return None
        if not self._eat('paren', ')'):
            return None
        if not self._eat('brace', '{'):
            return None
        items = self._parse_init_list()
        if not self._eat('brace', '}'):
            return None
        return CompoundLiteral(type_id.text, items)

    def _parse_init_list(self) -> List[InitItem]:
        items: List[InitItem] = []
        while True:
            # designated form: field = expr
            save = self.i
            fld = None
            t0 = self._peek()
            t1 = self._peek(1)
            if t0 is not None and t0.kind == 'ident' and t1 is not None and t1.kind == 'assign':
                ident_tok = self._eat('ident')
                fld = ident_tok.text if ident_tok is not None else None
                self._eat('assign', '=')
            expr = self._parse_expr()
            if expr is None:
                self.i = save
                break
            items.append(InitItem(field=fld, value=expr))
            if not self._eat('comma', ','):
                break
        return items

    # Very small Pratt parser for +,-,*,/ with parens and calls, identifiers, numbers, strings
    def _parse_expr(self, min_prec: int = 0) -> Optional[Expr]:
        tok = self._peek()
        if tok is None:
            return None
        # Primary
        if tok.kind == 'number':
            self._eat()
            left: Expr = Number(tok.text)
        elif tok.kind == 'string':
            self._eat()
            left = String(tok.text)
        elif tok.kind == 'ident':
            ident = self._eat('ident')
            # call?
            if self._eat('paren', '('):
                args: List[Expr] = []
                if not self._eat('paren', ')'):
                    while True:
                        arg = self._parse_expr()
                        if arg is None:
                            break
                        args.append(arg)
                        if self._eat('paren', ')'):
                            break
                        if not self._eat('comma', ','):
                            break
                if ident is None:
                    return None
                left = Call(Identifier(ident.text), args)
            else:
                if ident is None:
                    return None
                left = Identifier(ident.text)
        elif tok.kind == 'paren' and tok.text == '(':
            self._eat('paren', '(')
            inner = self._parse_expr()
            self._eat('paren', ')')
            left = inner if inner else Identifier('')
        else:
            return None

        # Binary ops
        prec_map = {
            '||': 1,
            '&&': 2,
            '==': 3, '!=': 3,
            '<': 4, '>': 4, '<=': 4, '>=': 4,
            '+': 5, '-': 5,
            '*': 6, '/': 6, '%': 6,
        }

        def get_prec(tok: Optional[Token]) -> int:
            if tok and tok.kind in ('op',) and tok.text in prec_map:
                return prec_map[tok.text]
            return -1

        while True:
            op_tok = self._peek()
            prec = get_prec(op_tok)
            if prec < min_prec:
                break
            if op_tok is None:
                break
            op = op_tok.text
            self._eat()
            right = self._parse_expr(prec + 1)
            if right is None:
                break
            left = Binary(left, op, right)
        return left


# --- Rendering to Nature ---

def _strip_float_suffix(num: str) -> str:
    # remove trailing f/F if present
    if num.lower().endswith('f') and (len(num) == 1 or num[-2].isdigit() or num[-2] == '.'): 
        return num[:-1]
    return num

def _expr_contains_float(e: Expr) -> bool:
    if isinstance(e, Number):
        t = e.text
        return ('f' in t.lower()) or ('.' in t) or ('e' in t.lower())
    if isinstance(e, String):
        return False
    if isinstance(e, Identifier):
        return False
    if isinstance(e, Unary):
        return _expr_contains_float(e.expr)
    if isinstance(e, Binary):
        return _expr_contains_float(e.left) or _expr_contains_float(e.right)
    if isinstance(e, Call):
        return False
    if isinstance(e, CompoundLiteral):
        return False
    return False

def render_expr(e: Expr) -> str:
    if isinstance(e, Number):
        return _strip_float_suffix(e.text)
    if isinstance(e, String):
        return f"{e.text}.ref()"
    if isinstance(e, Identifier):
        return e.name
    if isinstance(e, Unary):
        return f"{e.op}{render_expr(e.expr)}"
    if isinstance(e, Binary):
        left = render_expr(e.left)
        right = render_expr(e.right)
        return f"({left}{e.op}{right})"
    if isinstance(e, Call):
        args = ','.join(render_expr(a) for a in e.args)
        return f"{e.func.name}({args})"
    if isinstance(e, CompoundLiteral):
        # Fallback rendering without struct context
        inner = ','.join((f"{it.field}=" if it.field else "") + render_expr(it.value) for it in e.items)
        return f"{e.type_name}{{{inner}}}"
    return "<unknown>"


def render_constant(e: Expr, structs: Dict[str, Any], unions: Dict[str, Any]) -> Optional[Tuple[str, str]]:
    # Skip identifiers and calls (aliases or function-like)
    if isinstance(e, Identifier):
        return None
    if isinstance(e, Call):
        return None

    # String literal
    if isinstance(e, String):
        return ("anyptr", render_expr(e))

    # Numbers or arithmetic
    if isinstance(e, (Number, Unary, Binary)):
        ctype = 'f32' if _expr_contains_float(e) else 'i32'
        return (ctype, render_expr(e))

    # Compound literal mapped through struct info
    if isinstance(e, CompoundLiteral):
        tname = e.type_name
        if tname in structs:
            fields = structs[tname].fields
            pairs: List[str] = []
            # If designated, use as-is. Otherwise, map by order.
            designated = any(it.field for it in e.items)
            if designated:
                for it in e.items:
                    if not it.field:
                        continue
                    pairs.append(f"{it.field}={render_expr(it.value)}")
            else:
                for i, it in enumerate(e.items):
                    if i >= len(fields):
                        break
                    pairs.append(f"{fields[i].name}={render_expr(it.value)}")
            return (tname, f"{tname}{{{','.join(pairs)}}}")
        # Unknown type: render generic
        return (tname, render_expr(e))

    # Fallback
    return None


def parse_macro_replacement(text: str) -> Optional[Expr]:
    toks = tokenize(text)
    p = Parser(toks)
    return p.parse()


