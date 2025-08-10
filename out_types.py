from dataclasses import dataclass, field
from typing import List, Optional

@dataclass
class Constant:
    name: str
    ctype: str
    value: str

    def __hash__(self):
        return hash(self.name)

@dataclass
class Parameter:
    name: str
    ntype: str  # Nature type

@dataclass
class Function:
    name: str
    c_name: str
    return_type: str
    parameters: List[Parameter]
    is_variadic: bool = False

@dataclass
class StructField:
    name: str
    ntype: str

@dataclass
class Struct:
    name: str
    fields: List[StructField] = field(default_factory=list)
    cursor: Optional['Cursor'] = None

@dataclass
class Union:
    name: str
    size: int
    fields: List[StructField] = field(default_factory=list)
    cursor: Optional['Cursor'] = None

    def to_nature(self) -> str:
        """Generates Nature code for the union as a type alias to [u8;N]."""
        from num2words import num2words
        size_in_words = num2words(self.size)
        union_type_name = f"Union_{size_in_words}_bytes"
        return f"type {self.name} = [u8;{self.size}]\n"

@dataclass
class EnumMember:
    name: str
    value: int

@dataclass
class Enum:
    name: str
    members: List[EnumMember] = field(default_factory=list)

@dataclass
class UnnamedObject:
    is_union: bool
    file: str
    location: str

    def __hash__(self):
        """Make UnnamedObject hashable for use as dictionary key."""
        return hash((self.is_union, self.file, self.location))

    def __eq__(self, other):
        """Required for hash-based containers like dict and set."""
        if not isinstance(other, UnnamedObject):
            return False
        return (self.is_union == other.is_union and
                self.file == other.file and
                self.location == other.location)

    def to_str(self, put_at_start: bool) -> str:
        struct_or_union: str

        if self.is_union:
            struct_or_union = "union"
        else:
            struct_or_union = "struct"

        if put_at_start:
            return f"{struct_or_union} (unnamed at {self.file}:{self.location})"
        else:
            return f"(unnamed {struct_or_union} at {self.file}:{self.location})"

# Forward reference for type hints
from clang.cindex import Cursor
