from _typeshed import Incomplete
from dataclasses import dataclass
from typing import Any

@dataclass
class Token:
    kind: str
    text: str

def tokenize(src: str) -> list[Token]: ...

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
    args: list[Expr]

@dataclass
class InitItem:
    field: str | None
    value: Expr

@dataclass
class CompoundLiteral(Expr):
    type_name: str
    items: list[InitItem]

class Parser:
    toks: Incomplete
    i: int
    def __init__(self, tokens: list[Token]) -> None: ...
    def parse(self) -> Expr | None: ...

def render_expr(e: Expr) -> str: ...
def render_constant(e: Expr, structs: dict[str, Any], unions: dict[str, Any]) -> tuple[str, str] | None: ...
def parse_macro_replacement(text: str) -> Expr | None: ...
