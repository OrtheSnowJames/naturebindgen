from _typeshed import Incomplete
from out_types import Constant, Enum, Function, Struct, Union, UnnamedObject
from textwrap import dedent as dedent

class BindingGenerator:
    structs: dict[str, Struct]
    unions: dict[str, Union]
    enums: dict[str, Enum]
    functions: dict[str, Function]
    constants: dict[str, Constant]
    typedefs: dict[str, str]
    union_sizes: dict[str, int]
    clang_to_contextual: dict[UnnamedObject, str]
    reserved_keywords: Incomplete
    def __init__(self) -> None: ...
    def parse_header(self, header_path: str, c_args: list[str] | None = None): ...
    def generate_bindings(self) -> str: ...

def main() -> None: ...
