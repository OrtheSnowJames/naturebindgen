from clang.cindex import Cursor
from dataclasses import dataclass, field

@dataclass
class Constant:
    name: str
    ctype: str
    value: str
    def __hash__(self): ...

@dataclass
class Parameter:
    name: str
    ntype: str

@dataclass
class Function:
    name: str
    c_name: str
    return_type: str
    parameters: list[Parameter]
    is_variadic: bool = ...

@dataclass
class StructField:
    name: str
    ntype: str

@dataclass
class Struct:
    name: str
    fields: list[StructField] = field(default_factory=list)
    cursor: Cursor | None = ...

@dataclass
class Union:
    name: str
    size: int
    fields: list[StructField] = field(default_factory=list)
    cursor: Cursor | None = ...
    def to_nature(self) -> str: ...

@dataclass
class EnumMember:
    name: str
    value: int

@dataclass
class Enum:
    name: str
    members: list[EnumMember] = field(default_factory=list)

@dataclass
class UnnamedObject:
    is_union: bool
    file: str
    location: str
    def __hash__(self): ...
    def __eq__(self, other): ...
    def to_str(self, put_at_start: bool) -> str: ...
