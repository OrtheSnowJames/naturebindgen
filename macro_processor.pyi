from _typeshed import Incomplete
from clang.cindex import TranslationUnit as TranslationUnit
from typing import Any

class MacroProcessor:
    structs: Incomplete
    unions: Incomplete
    def __init__(self, structs: dict[str, Any], unions: dict[str, Any]) -> None: ...
    def process_macro(self, header_path: str, define_name: str, clang_args: list[str] | None = None) -> str | None: ...
