#!/usr/bin/env python3
import argparse
import os
import re
import sys
from num2words import num2words
from textwrap import dedent
from typing import Dict, List, Optional, Set

# Ensure libclang and num2words are installed:
# pip install libclang num2words
from clang.cindex import (Config, Cursor, CursorKind, Index, TranslationUnit,
                          Type, TypeKind)

if os.name == "posix":
    Config.set_library_file("/usr/lib/libclang.so")
else:
    Config.set_library_file(os.getenv("LIBCLANG_PATH") or "")

from out_types import (
    Constant, Parameter, Function, StructField, Struct,
    Union, EnumMember, Enum, UnnamedObject
)

# --- Core Binding Generator ---

class BindingGenerator:
    """
    Generates Nature language bindings from C header files by parsing the AST
    using libclang.
    """

    def __init__(self):
        self.structs: Dict[str, Struct] = {}
        self.unions: Dict[str, Union] = {}
        self.enums: Dict[str, Enum] = {}
        self.functions: Dict[str, Function] = {}
        self.constants: Dict[str, Constant] = {}
        self.typedefs: Dict[str, str] = {}
        self.union_sizes: Dict[str, int] = {}  # Maps union names to their sizes
        self.clang_to_contextual: Dict[UnnamedObject, str] = {}  # Maps raw clang spelling to contextual names

        self._processed_cursors: Set[Cursor] = set()
        self._anon_type_map: Dict[str, str] = {} # Maps Clang anonymous names to our generated ones
        self._queued_macros: List[tuple[Cursor, str, List[str], bool]] = []

        self.reserved_keywords = {"type"}

        self._initialize_type_mappings()

    def _parse_unnamed_object(self, spelling: str) -> Optional[UnnamedObject]:
        """Parse clang spelling to extract unnamed object information."""
        if not spelling or ("unnamed" not in spelling and "anonymous" not in spelling):
            print(f"DEBUG: Failed to parse unnamed object: {spelling}")
            return None

        # Handle patterns like:
        # "struct (unnamed at test_edge_cases.h:31:5)"
        # "union (unnamed at test_edge_cases.h:57:5)"
        # "(unnamed struct at test_edge_cases.h:31:5)"
        # "(unnamed union at test_edge_cases.h:57:5)"

        is_union = "union" in spelling
        is_struct = "struct" in spelling

        # Extract file and location
        match = re.search(r'at ([^:]+):(\d+):(\d+)', spelling)
        if match:
            file = match.group(1)
            location = f"{match.group(2)}:{match.group(3)}"
            return UnnamedObject(is_union=is_union, file=file, location=location)

        print(f"DEBUG: Failed to parse unnamed object: {spelling}")
        return None

    def _get_unnamed_object_mapping(self, type_spelling: str) -> Optional[str]:
        """
        Gets the contextual name for an unnamed/anonymous object from our mapping.
        
        Args:
            type_spelling: The raw type spelling from clang that might contain unnamed/anonymous references
            
        Returns:
            The mapped contextual name if found, otherwise None
        """
        if not type_spelling or ("unnamed" not in type_spelling and "anonymous" not in type_spelling):
            return None

        # Try direct mapping first
        if type_spelling in self.clang_to_contextual:
            unnamed_obj = self._parse_unnamed_object(type_spelling)
            if unnamed_obj:
                return self.clang_to_contextual[unnamed_obj]

        # Try parsing as UnnamedObject and find matching mapping
        unnamed_obj = self._parse_unnamed_object(type_spelling)
        if unnamed_obj:
            # Look for a mapping that matches this unnamed object
            for clang_spelling, contextual_name in self.clang_to_contextual.items():
                if isinstance(clang_spelling, UnnamedObject):  # Ensure we're comparing UnnamedObjects
                    if (clang_spelling.is_union == unnamed_obj.is_union and
                        clang_spelling.file == unnamed_obj.file and
                        clang_spelling.location == unnamed_obj.location):
                        return contextual_name

        # Check for struct/union prefix
        if type_spelling.startswith("struct "):
            result = self._get_unnamed_object_mapping(type_spelling[7:])
            if result:
                return result
        elif type_spelling.startswith("union "):
            result = self._get_unnamed_object_mapping(type_spelling[6:])
            if result:
                return result

        return None

    def _initialize_type_mappings(self):
        """Sets up the default C to Nature type mappings."""
        mappings = [
            ("void", "void"), ("char", "i8"), ("signed char", "i8"),
            ("unsigned char", "u8"), ("short", "i16"), ("unsigned short", "u16"),
            ("int", "i32"), ("signed int", "i32"), ("unsigned int", "u32"),
            ("unsigned", "u32"), ("long", "i64"), ("long int", "i64"),
            ("unsigned long", "u64"), ("unsigned long int", "u64"),
            ("long long", "i64"), ("unsigned long long", "u64"),
            ("float", "f32"), ("double", "f64"), ("long double", "f64"),
            ("size_t", "uint"), ("ssize_t", "int"), ("ptrdiff_t", "int"),
            ("uintptr_t", "anyptr"), ("intptr_t", "anyptr"),
            ("int8_t", "i8"), ("uint8_t", "u8"), ("int16_t", "i16"),
            ("uint16_t", "u16"), ("int32_t", "i32"), ("uint32_t", "u32"),
            ("int64_t", "i64"), ("uint64_t", "u64"), ("bool", "bool"),
            ("_Bool", "bool"),
            # Common typedefs
            ("Uint8", "u8"), ("Uint16", "u16"), ("Uint32", "u32"), ("Uint64", "u64"),
            ("Sint8", "i8"), ("Sint16", "i16"), ("Sint32", "i32"), ("Sint64", "i64"),
            # Pointer base types
            ("void*", "anyptr"), ("char*", "anyptr"), ("const char*", "anyptr"),
        ]
        self.type_mappings = {c_type: nature_type for c_type, nature_type in mappings}

    def _sanitize_name(self, name: str) -> str:
        """Appends an underscore to a name if it's a reserved keyword."""
        return f"{name}_" if name in self.reserved_keywords else name

    def _map_c_type_to_nature(self, c_type: Type) -> str:
        """Converts a clang Type object to a Nature language type string."""
        type_spelling = c_type.spelling.replace("const ", "").strip()

        # 1. Handle Pointers
        if c_type.kind == TypeKind.POINTER:
            pointee = c_type.get_pointee()

            # Function Pointers are just generic pointers in Nature
            if pointee.kind == TypeKind.FUNCTIONPROTO:
                return "anyptr"

            if pointee.kind in (TypeKind.VOID, TypeKind.CHAR_S):
                return "anyptr"

            # For known record types, return a typed raw pointer
            pointee_decl = pointee.get_declaration()
            if pointee_decl.kind in (CursorKind.STRUCT_DECL, CursorKind.UNION_DECL):
                # Use the canonical name if it's an anonymous type we've mapped
                record_name = self._anon_type_map.get(pointee_decl.spelling, pointee_decl.spelling)
                return f"rawptr<{record_name}>"

            return "anyptr" # Default for other pointers

        # 2. Handle Arrays
        if c_type.kind == TypeKind.CONSTANTARRAY:
            element_type = self._map_c_type_to_nature(c_type.get_array_element_type())
            size = c_type.get_array_size()
            return f"[{element_type};{size}]"

        # 3. Handle Typedefs
        if c_type.kind == TypeKind.TYPEDEF:
            return self.typedefs.get(type_spelling, type_spelling)

        # 4. Handle Structs and Unions
        if c_type.kind == TypeKind.RECORD:
            decl = c_type.get_declaration()
            record_name = decl.spelling
            print(f"DEBUG: Record type found: '{record_name}' (kind: {c_type.kind})")

            # Check if this is an anonymous type we've processed
            if "anonymous" in record_name or "unnamed" in record_name or not record_name:
                print(f"DEBUG: Anonymous type found: '{record_name}'")
                # Try to get the contextual name from our mapping
                contextual_name = self._get_unnamed_object_mapping(record_name)
                if contextual_name:
                    print(f"DEBUG: Mapped '{record_name}' to '{contextual_name}'")
                    return contextual_name
                # Fallback to anon type map
                return self._anon_type_map.get(record_name, record_name)

            # Remove "struct" prefix if present
            if record_name.startswith("struct "):
                record_name = record_name[7:]  # Remove "struct " prefix
            return record_name

        # 5. Basic Types from our map
        # Use canonical type for robustness (e.g., `long int` -> `long`)
        canonical_spelling = c_type.get_canonical().spelling.replace("const ", "").strip()
        if canonical_spelling in self.type_mappings:
            return self.type_mappings[canonical_spelling]
        if type_spelling in self.type_mappings:
            return self.type_mappings[type_spelling]

        # 6. Check typedefs for mapped types
        if type_spelling in self.typedefs:
            mapped_type = self.typedefs[type_spelling]
            return mapped_type

        # 7. Remove "struct" and "union" prefixes for lookup
        normalized_type = type_spelling.replace("struct ", "").replace("union ", "")

        # 8. Check if this is a union name we've seen
        if normalized_type in self.union_sizes:
            size = self.union_sizes[normalized_type]
            return f"Union_{num2words(size)}_bytes"

        # 9. Check if this is a known struct
        if normalized_type in self.structs:
            return normalized_type

        # 10. Fallback for unknown but declared types (structs, etc.)
        if type_spelling:
            # Remove "struct" prefix if present
            if type_spelling.startswith("struct "):
                return type_spelling[7:]
            return type_spelling

        return "any" # Last resort

    def parse_header(self, header_path: str, c_args: List[str] | None = None):
        """Parses the given C header file and populates the binding model."""
        if not os.path.exists(header_path):
            raise FileNotFoundError(f"Header file not found: {header_path}")

        print(f"Parsing header: {header_path}")
        index = Index.create()
        # Use '-x', 'c-header' to force parsing as C
        # Add -fparse-all-comments to ensure we get macro definitions
        args = ['-x', 'c-header', '-fparse-all-comments', '-dD']  # -dD preserves macro definitions
        if c_args:
            args.extend(c_args)
        print(f"DEBUG: Parsing with args: {args}")
        tu = index.parse(header_path, args=args, 
                        options=TranslationUnit.PARSE_DETAILED_PROCESSING_RECORD | 
                                TranslationUnit.PARSE_INCLUDE_BRIEF_COMMENTS_IN_CODE_COMPLETION)

        if not tu:
            raise RuntimeError("Failed to parse the translation unit.")

        has_errors = any(diag.severity >= diag.Error for diag in tu.diagnostics)
        if has_errors:
            print("Clang errors encountered during parsing. Bindings may be incomplete.", file=sys.stderr)

        self._visit_cursor(tu.cursor, header_path, c_args or [], is_root=True)

        # Post-process: fix field types using our contextual mappings
        self._fix_field_types()

        # Fix field types by mapping anonymous/unnamed types
        self._map_anonymous_field_types()

        # Final adjustments to constants (e.g., struct compound literals)
        self._postprocess_constants()

    def _visit_cursor(self, cursor: Cursor, header_path: str = "", clang_args: Optional[List[str]] = None, is_root: bool = False):
        """Recursively traverses the AST and dispatches to handlers."""
        if not cursor or (cursor.location.file and "usr/include" in str(cursor.location.file)):
            return
        if cursor in self._processed_cursors:
            return
        self._processed_cursors.add(cursor)

        kind = cursor.kind
        print(f"DEBUG: Visiting cursor: {cursor.spelling} (kind: {kind})")

        if kind == CursorKind.STRUCT_DECL:
            self._handle_struct_or_union(cursor, is_union=False)
        elif kind == CursorKind.UNION_DECL:
            self._handle_struct_or_union(cursor, is_union=True)
        elif kind == CursorKind.ENUM_DECL:
            self._handle_enum(cursor)
        elif kind == CursorKind.FUNCTION_DECL:
            self._handle_function(cursor)
        elif kind == CursorKind.TYPEDEF_DECL:
            self._handle_typedef(cursor)
        elif kind == CursorKind.MACRO_DEFINITION:
            print(f"DEBUG: Found macro definition: {cursor.spelling}")
            # Queue macros to process after types so struct info is available
            is_first = True
            for prev_cursor in cursor.translation_unit.cursor.get_children():
                if prev_cursor.kind == CursorKind.MACRO_DEFINITION and prev_cursor != cursor:
                    is_first = False
                    break
            self._queued_macros.append((cursor, str(cursor.location.file), clang_args or [], is_first))

        for child in cursor.get_children():
            self._visit_cursor(child, header_path, clang_args, is_root=False)

        # Only flush once, after walking the TU root
        if is_root and self._queued_macros:
            print(f"DEBUG: Flushing {len(self._queued_macros)} queued macros after type collection")
            for mc, hp, ca, is_first in self._queued_macros:
                self._handle_macro(mc, hp, ca, is_first_macro=is_first)
            self._queued_macros.clear()

    def _fix_field_types(self):
        """Post-process field types to use contextual names instead of raw clang spelling."""
        print("DEBUG: Post-processing field types...")
        print(f"DEBUG: Available clang_to_contextual mappings: {list(self.clang_to_contextual.keys())}")

        # Fix struct field types
        for struct_name, struct in self.structs.items():
            for field in struct.fields:
                original_type = field.ntype
                # Try to get mapped type
                mapped_type = self._get_unnamed_object_mapping(original_type)
                if mapped_type:
                    field.ntype = mapped_type
                    print(f"DEBUG: Fixed struct field '{field.name}' in '{struct_name}': '{original_type}' -> '{field.ntype}'")

        # Fix union field types
        for union_name, union in self.unions.items():
            for field in union.fields:
                original_type = field.ntype
                # Try to get mapped type
                mapped_type = self._get_unnamed_object_mapping(original_type)
                if mapped_type:
                    field.ntype = mapped_type
                    print(f"DEBUG: Fixed union field '{field.name}' in '{union_name}': '{original_type}' -> '{field.ntype}'")

    def _map_anonymous_field_types(self):
        """
        Post-processes field types to map anonymous and unnamed types
        to their contextual names if they were previously mapped.
        """
        print("DEBUG: Post-processing anonymous field types...")
        print(f"DEBUG: Available clang_to_contextual mappings: {list(self.clang_to_contextual.keys()) if self.clang_to_contextual else 'none'}")

        # Iterate through all structs and unions to find anonymous/unnamed types
        for struct_name, struct in self.structs.items():
            for field in struct.fields:
                if "anonymous" in field.ntype or "unnamed" in field.ntype:
                    print(f"DEBUG: Found anonymous/unnamed struct field '{field.name}' in '{struct_name}': '{field.ntype}'")
                    mapped_type = self._get_unnamed_object_mapping(field.ntype)
                    if mapped_type:
                        field.ntype = mapped_type
                        print(f"DEBUG: Mapped struct field '{field.name}' from '{field.ntype}' to '{mapped_type}'")

        for union_name, union in self.unions.items():
            for field in union.fields:
                if "anonymous" in field.ntype or "unnamed" in field.ntype:
                    print(f"DEBUG: Found anonymous/unnamed union field '{field.name}' in '{union_name}': '{field.ntype}'")
                    mapped_type = self._get_unnamed_object_mapping(field.ntype)
                    if mapped_type:
                        field.ntype = mapped_type
                        print(f"DEBUG: Mapped union field '{field.name}' from '{field.ntype}' to '{mapped_type}'")

    def _postprocess_constants(self):
        """Fix up constant values and types after full AST traversal."""
        import re as _re
        to_update: Dict[str, Constant] = {}
        for name, const in list(self.constants.items()):
            value = const.value or ""
            # Drop empty values (e.g., header guards)
            if value == "":
                print(f"DEBUG: Dropping empty constant '{name}'")
                self.constants.pop(name, None)
                continue
            # Normalize struct initializers to named-field form using recursive formatter
            m_named = _re.match(r"^(?:struct\s+)?(\w+)\s*\{([\s\S]*)\}$", value)
            if m_named:
                struct_name = m_named.group(1)
                init_body = m_named.group(2)
                if struct_name in self.structs:
                    new_value = self._format_struct_initializer(struct_name, init_body) or f"{struct_name}{{{init_body}}}"
                    new_ctype = struct_name
                    if const.ctype != new_ctype or value != new_value:
                        print(f"DEBUG: Post-processed constant '{name}': '{const.ctype} {value}' -> '{new_ctype} {new_value}'")
                    to_update[name] = Constant(name=name, value=new_value, ctype=new_ctype)
                    continue
            # Convert unknown typeof(T) to T when possible
            m_typeof = _re.match(r"^typeof\((\w+)\)$", const.ctype or "")
            if m_typeof and m_typeof.group(1) in self.structs:
                real = m_typeof.group(1)
                if value == "<unknown>":
                    # Try to rebuild from a simple literal macro pattern existing in typedef order
                    # If we cannot, leave as-is
                    pass
                else:
                    to_update[name] = Constant(name=name, value=value, ctype=real)
        # Apply updates
        for n, c in to_update.items():
            self.constants[n] = c

    def _get_contextual_name(self, cursor: Cursor, prefix: str) -> str:
        """Creates a stable name for anonymous records based on context."""
        if cursor.spelling and not "anonymous" in cursor.spelling and not "unnamed" in cursor.spelling:
            return cursor.spelling

        # Create a name based on the parent's name and the field name
        parent = cursor.semantic_parent
        if parent:
            # Find the field that this anonymous record is the type of
            for child in parent.get_children():
                if child.kind == CursorKind.FIELD_DECL and child.type.get_declaration() == cursor:
                    field_name = child.spelling
                    parent_name = parent.spelling or "Anonymous"
                    return f"{parent_name}_{field_name}_{prefix}"

            # If this is a nested anonymous struct/union
            if parent.kind == CursorKind.STRUCT_DECL:
                parent_name = parent.spelling or "Anonymous"
                return f"{parent_name}_nested_{prefix}"

        return f"Anonymous_{prefix}_{cursor.hash}" # Fallback


    def _split_top_level(self, text: str) -> List[str]:
        """Split a comma-separated initializer at top level, ignoring nested braces/parens."""
        parts: List[str] = []
        buf: List[str] = []
        depth_brace = 0
        depth_paren = 0
        i = 0
        while i < len(text):
            ch = text[i]
            if ch == '{':
                depth_brace += 1
                buf.append(ch)
            elif ch == '}':
                depth_brace -= 1
                buf.append(ch)
            elif ch == '(':
                depth_paren += 1
                buf.append(ch)
            elif ch == ')':
                depth_paren -= 1
                buf.append(ch)
            elif ch == ',' and depth_brace == 0 and depth_paren == 0:
                part = ''.join(buf).strip()
                if part:
                    parts.append(part)
                buf = []
            else:
                buf.append(ch)
            i += 1
        tail = ''.join(buf).strip()
        if tail:
            parts.append(tail)
        return parts

    def _format_struct_initializer(self, struct_name: str, inner_text: str) -> Optional[str]:
        """Return named-field initializer for struct_name using inner_text, recursively handling nested structs/unions."""
        if struct_name not in self.structs:
            return None
        field_values = self._split_top_level(inner_text)
        print(f"DEBUG: _format_struct_initializer struct={struct_name} inner='{inner_text}' -> parts={field_values}")
        field_names = [f.name for f in self.structs[struct_name].fields]
        pairs: List[str] = []
        for i, fname in enumerate(field_names):
            if i >= len(field_values):
                break
            raw_val = field_values[i]
            print(f"DEBUG:   field {fname} type={self.structs[struct_name].fields[i].ntype} raw='{raw_val}'")
            # Handle designated initializers: field=value
            if raw_val.strip().startswith(f"{fname}="):
                rhs = raw_val.split('=', 1)[1].strip()
                ftype = self.structs[struct_name].fields[i].ntype
                nested_struct = ftype if ftype in self.structs else None
                union_def = None
                for u in self.unions.values():
                    if getattr(u, 'name', None) == ftype:
                        union_def = u
                        break
                if nested_struct:
                    nested_inner = None
                    if rhs.startswith('{') and rhs.endswith('}'):
                        nested_inner = rhs[1:-1].strip()
                    elif rhs.startswith('(') and rhs.endswith('}'):
                        import re as _re
                        m = _re.match(r"^\(\s*(?:struct\s+)?(\w+)\s*\)\s*\{([\s\S]*)\}$", rhs)
                        if m and m.group(1) == nested_struct:
                            nested_inner = m.group(2)
                    nested_fmt = self._format_struct_initializer(nested_struct, nested_inner) if nested_inner is not None else None
                    pairs.append(f"{fname}={nested_fmt if nested_fmt else rhs}")
                    continue
                if union_def:
                    uval = rhs
                    if rhs.startswith('{') and rhs.endswith('}'):
                        inner_parts = self._split_top_level(rhs[1:-1].strip())
                        uval = inner_parts[0] if inner_parts else ''
                    elif rhs.startswith('(') and '){' in rhs and rhs.endswith('}'):
                        inner = rhs.split('){', 1)[1]
                        inner = inner[:-1]
                        inner_parts = self._split_top_level(inner.strip())
                        uval = inner_parts[0] if inner_parts else ''
                    ctor_name = f"new{union_def.name}"
                    pairs.append(f"{fname}={ctor_name}({uval})")
                    continue
                if ftype == 'anyptr' and rhs.startswith('"') and rhs.endswith('"'):
                    pairs.append(f"{fname}={rhs}.ref()")
                else:
                    pairs.append(raw_val)
                continue
            # Nested struct
            ftype = self.structs[struct_name].fields[i].ntype
            nested_struct = ftype if ftype in self.structs else None
            # unions are stored by contextual key; match by union value name
            union_def = None
            for u in self.unions.values():
                if getattr(u, 'name', None) == ftype:
                    union_def = u
                    break
            if nested_struct:
                # Accept '{...}' or '(Type){...}' forms
                nested_inner = None
                if raw_val.startswith('{') and raw_val.endswith('}'):
                    nested_inner = raw_val[1:-1].strip()
                elif raw_val.startswith('(') and raw_val.endswith('}'):
                    import re as _re
                    m = _re.match(r"^\(\s*(?:struct\s+)?(\w+)\s*\)\s*\{([\s\S]*)\}$", raw_val)
                    if m:
                        cand_type, inner = m.group(1), m.group(2)
                        if cand_type == nested_struct:
                            nested_inner = inner
                nested_fmt = self._format_struct_initializer(nested_struct, nested_inner) if nested_inner is not None else None
                if nested_fmt:
                    pairs.append(f"{fname}={nested_fmt}")
                else:
                    pairs.append(f"{fname}={raw_val}")
            elif union_def:
                # If we have a literal for the union field value, wrap it with helper ctor
                # Accept '{...}', '(Type){...}', or a bare literal
                uval = raw_val
                if raw_val.startswith('{') and raw_val.endswith('}'):
                    inner_parts = self._split_top_level(raw_val[1:-1].strip())
                    uval = inner_parts[0] if inner_parts else ''
                elif raw_val.startswith('(') and '){' in raw_val and raw_val.endswith('}'):
                    # Strip (Type){...}
                    inner = raw_val.split('){', 1)[1]
                    inner = inner[:-1]
                    inner_parts = self._split_top_level(inner.strip())
                    uval = inner_parts[0] if inner_parts else ''
                ctor_name = f"new{union_def.name}"
                pairs.append(f"{fname}={ctor_name}({uval})")
            else:
                # Normalize pointer-like string fields
                if ftype == 'anyptr' and raw_val.startswith('"') and raw_val.endswith('"'):
                    pairs.append(f"{fname}={raw_val}.ref()")
                else:
                    pairs.append(f"{fname}={raw_val}")
        formatted = f"{struct_name}{{{','.join(pairs)}}}"
        print(f"DEBUG: _format_struct_initializer result: {formatted}")
        return formatted


    def _handle_struct_or_union(self, cursor: Cursor, is_union: bool):
        if not cursor.is_definition(): return

        prefix = "Union" if is_union else "Struct"
        decl_name = self._get_contextual_name(cursor, prefix)

        # For anonymous structs/unions, create a better contextual name
        if not cursor.spelling or "anonymous" in cursor.spelling or "unnamed" in cursor.spelling:
            print(f"DEBUG: Processing anonymous type with spelling: '{cursor.spelling}'")
            parent = cursor.semantic_parent
            if parent and parent.kind == CursorKind.FIELD_DECL:
                # This is a field of a struct/union, use parent context
                parent_struct = parent.semantic_parent
                if parent_struct:
                    field_name = parent.spelling
                    struct_name = parent_struct.spelling or "Anonymous"
                    decl_name = f"{struct_name}_{field_name}_{prefix}"
                    print(f"DEBUG: Created contextual name: '{decl_name}' for field '{field_name}' in '{struct_name}'")
                                    # Store mapping immediately
                if cursor.spelling:
                    unnamed_obj = self._parse_unnamed_object(cursor.spelling)
                    if unnamed_obj:
                        self.clang_to_contextual[unnamed_obj] = decl_name
                        print(f"DEBUG: Stored mapping '{cursor.spelling}' -> '{decl_name}'")
            elif parent and parent.kind == CursorKind.STRUCT_DECL:
                # Nested anonymous struct
                parent_name = parent.spelling or "Anonymous"
                decl_name = f"{parent_name}_nested_{prefix}"
                print(f"DEBUG: Created contextual name: '{decl_name}' for nested struct in '{parent_name}'")
                # Store mapping immediately
                if cursor.spelling:
                    unnamed_obj = self._parse_unnamed_object(cursor.spelling)
                    if unnamed_obj:
                        self.clang_to_contextual[unnamed_obj] = decl_name
                        print(f"DEBUG: Stored mapping '{cursor.spelling}' -> '{decl_name}'")
            else:
                # Fallback for truly anonymous types
                decl_name = f"Anonymous_{prefix}_{cursor.hash}"
                print(f"DEBUG: Created fallback name: '{decl_name}'")
                # Store mapping immediately
                if cursor.spelling:
                    unnamed_obj = self._parse_unnamed_object(cursor.spelling)
                    if unnamed_obj:
                        self.clang_to_contextual[unnamed_obj] = decl_name
                        print(f"DEBUG: Stored mapping '{unnamed_obj}' -> '{decl_name}'")

        target_dict = self.unions if is_union else self.structs
        if decl_name in target_dict: return

        print(f"Found {prefix}: {decl_name}")

        fields = []
        for field_cursor in cursor.get_children():
            if field_cursor.kind == CursorKind.FIELD_DECL:
                field_name = self._sanitize_name(field_cursor.spelling)
                nature_type = self._map_c_type_to_nature(field_cursor.type)

                # Check if this type contains anonymous or unnamed and try to map it
                if "anonymous" in nature_type or "unnamed" in nature_type:
                    if nature_type in self.clang_to_contextual:
                        nature_type = self.clang_to_contextual[self._parse_unnamed_object(nature_type) or UnnamedObject(is_union=False, file="", location="")]
                        print(f"DEBUG: Mapped field type '{field_name}' from '{nature_type}' to '{self.clang_to_contextual[self._parse_unnamed_object(nature_type) or UnnamedObject(is_union=False, file="", location="")]}'")

                fields.append(StructField(name=field_name, ntype=nature_type))

        if is_union:
            size = cursor.type.get_size()
            if size <= 0: return # Don't process incomplete unions
            union_name_by_size = f"Union_{num2words(size)}_bytes"
            self.unions[decl_name] = Union(name=union_name_by_size, size=size, fields=fields, cursor=cursor)
            # Store the original name and size mapping
            self.union_sizes[decl_name] = size
            # Map the original name to the sized name for type mapping
            self.typedefs[decl_name] = union_name_by_size
            # For unions, update the mapping to use the size-based name
            if cursor.spelling and ("anonymous" in cursor.spelling or "unnamed" in cursor.spelling):
                unnamed_obj = self._parse_unnamed_object(cursor.spelling)
                if unnamed_obj:
                    self.clang_to_contextual[unnamed_obj] = union_name_by_size
                    print(f"DEBUG: Updated union mapping '{cursor.spelling}' -> '{union_name_by_size}'")
        else:
            self.structs[decl_name] = Struct(name=decl_name, fields=fields, cursor=cursor)
            # Store mapping from raw clang spelling to contextual name
            if cursor.spelling and ("anonymous" in cursor.spelling or "unnamed" in cursor.spelling):
                unnamed_obj = self._parse_unnamed_object(cursor.spelling)
                if unnamed_obj:
                    self.clang_to_contextual[unnamed_obj] = decl_name
                    print(f"DEBUG: Stored struct mapping '{cursor.spelling}' -> '{decl_name}'")


    def _handle_enum(self, cursor: Cursor):
        enum_name = cursor.spelling
        if not enum_name or enum_name in self.enums: return

        print(f"Found Enum: {enum_name}")
        members = [
            EnumMember(name=c.spelling, value=c.enum_value)
            for c in cursor.get_children() if c.kind == CursorKind.ENUM_CONSTANT_DECL
        ]
        self.enums[enum_name] = Enum(name=enum_name, members=members)

    def _handle_function(self, cursor: Cursor):
        func_name = cursor.spelling
        if not func_name or func_name in self.functions: return

        print(f"Found Function: {func_name}")
        return_type = self._map_c_type_to_nature(cursor.result_type)
        params = [
            Parameter(
                name=self._sanitize_name(p.spelling or f"arg{i}"),
                ntype=self._map_c_type_to_nature(p.type)
            )
            for i, p in enumerate(cursor.get_arguments())
        ]

        self.functions[func_name] = Function(
            name=func_name, c_name=cursor.mangled_name,
            return_type=return_type, parameters=params,
            is_variadic=cursor.type.is_function_variadic()
        )

    def _handle_typedef(self, cursor: Cursor):
        name = cursor.spelling
        underlying_type = cursor.underlying_typedef_type

        # Handle typedefs for anonymous records, e.g., typedef struct { ... } MyStruct;
        # In this case, we name the record `MyStruct`.
        underlying_decl = underlying_type.get_declaration()
        if underlying_decl.kind in (CursorKind.STRUCT_DECL, CursorKind.UNION_DECL) and not underlying_decl.spelling:
            is_union = underlying_decl.kind == CursorKind.UNION_DECL
            self._handle_struct_or_union(underlying_decl, is_union)
            # Find the anonymous record we just created and rename it
            anon_name = self._get_contextual_name(underlying_decl, "Union" if is_union else "Struct")
            target_dict = self.unions if is_union else self.structs
            if anon_name in target_dict:
                record = target_dict.pop(anon_name)
                record.name = name
                # Type assertion to handle the union type
                if is_union:
                    self.unions[name] = record  # type: ignore
                else:
                    self.structs[name] = record  # type: ignore
            self.typedefs[name] = name # Map the typedef name to the new record name
            return

        mapped_type = self._map_c_type_to_nature(underlying_type)
        self.typedefs[name] = mapped_type
        print(f"Found Typedef: {name} -> {mapped_type}")

    def _handle_macro(self, cursor: Cursor, header_path: str, clang_args: List[str], is_first_macro: bool = False):
        macro_name = cursor.spelling
        print(f"DEBUG: Handling macro: {macro_name}")
        if macro_name in self.constants:
            print(f"DEBUG: Macro {macro_name} already processed, skipping")
            return

        # Skip internal/compiler macros
        if macro_name.startswith('__'):
            print(f"DEBUG: Skipping internal macro: {macro_name}")
            return

        # Skip internal macros
        if not cursor.location.file:
            print(f"DEBUG: Skipping macro with no location: {macro_name}")
            return

        # Skip macros from system headers (included with <>)
        file_path = str(cursor.location.file)
        
        # Get tokens safely
        tokens = []
        if cursor.extent and cursor.translation_unit:
            try:
                tokens = list(cursor.translation_unit.get_tokens(extent=cursor.extent))
            except Exception as e:
                print(f"DEBUG: Error getting tokens: {e}")
                tokens = []

        # Check for system header includes
        for i, token in enumerate(tokens[:-2]):  # Need at least 3 tokens for include
            if token.spelling == '#' and tokens[i + 1].spelling == 'include':
                include_path = tokens[i + 2].spelling
                if include_path.startswith('<') and include_path.endswith('>'):
                    if include_path[1:-1] in file_path:
                        print(f"DEBUG: Skipping macro from system header: {macro_name}")
                        return
                else:
                    print(f"Header Path: {include_path}")

        # Skip first macro if it's likely a header guard
        if is_first_macro:
            # Check if this file uses pragma once instead of header guards
            has_pragma_once = False
            if cursor.extent and cursor.translation_unit:
                try:
                    tokens = list(cursor.translation_unit.get_tokens(extent=cursor.extent))
                    for i, token in enumerate(tokens[:-1]):  # Skip last token to avoid index error
                        if token.spelling == 'pragma' and tokens[i + 1].spelling == 'once':
                            has_pragma_once = True
                            break
                except Exception as e:
                    print(f"DEBUG: Error getting tokens: {e}")
                    has_pragma_once = False
            
            if not has_pragma_once:
                print(f"DEBUG: Skipping first macro (likely header guard): {macro_name}")
                return

        # Try a fast path: parse the macro replacement directly from the macro definition tokens
        fast_tokens = []
        if cursor.extent:
            try:
                fast_tokens = list(cursor.get_tokens())
            except Exception as e:
                print(f"DEBUG: Could not read macro tokens for fast path: {e}")
                fast_tokens = []

        if fast_tokens:
            # Expect a pattern like: #, define, NAME, <replacement...>
            try:
                name_index = next(i for i, t in enumerate(fast_tokens) if t.spelling == macro_name)
            except StopIteration:
                name_index = -1
            if name_index != -1 and name_index + 1 < len(fast_tokens):
                replacement_tokens = fast_tokens[name_index + 1:]
                replacement = ''.join(t.spelling for t in replacement_tokens).strip()
                print(f"DEBUG: Macro replacement for {macro_name}: '{replacement}'")
                if replacement:
                    # Detect struct compound literals: (Type){...} or Type{...}
                    import re as _re
                    m = _re.match(r"^\(\s*(?:struct\s+)?(\w+)\s*\)\s*\{([\s\S]*)\}$", replacement)
                    if not m:
                        m = _re.match(r"^(?:struct\s+)?(\w+)\s*\{([\s\S]*)\}$", replacement)
                    if m:
                        struct_name = m.group(1)
                        if struct_name in self.structs:
                            raw_vals = m.group(2)
                            value = self._format_struct_initializer(struct_name, raw_vals) or f"{struct_name}{{{raw_vals}}}"
                            ctype = struct_name
                            self.constants[macro_name] = Constant(name=macro_name, value=value, ctype=ctype)
                            print(f"DEBUG: [fast-struct] Added constant: {macro_name} = {value} ({ctype})")
                            return
                        else:
                            # Unknown struct; fall back to processor
                            from macro_processor import MacroProcessor
                            processor = MacroProcessor(self.structs, self.unions)
                            result = processor.process_macro(header_path=str(cursor.location.file), define_name=macro_name, clang_args=clang_args)
                            if result:
                                try:
                                    first_space = result.find(' ')
                                    eq_pos = result.find('=')
                                    semi_pos = result.rfind(';')
                                    ctype = result[:first_space].strip()
                                    name = result[first_space:eq_pos].strip()
                                    value = result[eq_pos+1: semi_pos if semi_pos != -1 else None].strip()
                                    self.constants[name] = Constant(name=name, value=value, ctype=ctype)
                                    print(f"DEBUG: [fast->proc] Added constant: {name} = {value} ({ctype})")
                                    return
                                except Exception:
                                    pass
                    # Determine a simple type
                    if replacement.startswith('"') and replacement.endswith('"'):
                        ctype = 'anyptr'
                        value = f"{replacement}.ref()"
                    else:
                        # Try to detect integer literal (default to i32)
                        ctype = 'i32'
                        value = replacement

                    self.constants[macro_name] = Constant(name=macro_name, value=value, ctype=ctype)
                    print(f"DEBUG: [fast] Added constant: {macro_name} = {value} ({ctype})")
                    return

        # Get the actual header file path
        actual_header = str(cursor.location.file)
        print(f"DEBUG: Using header path: {actual_header}")

        from macro_processor import MacroProcessor
        processor = MacroProcessor(self.structs, self.unions)
        result = processor.process_macro(
            header_path=actual_header,
            define_name=macro_name,
            clang_args=clang_args
        )
        print(f"DEBUG: Macro result: {result}")
        
        if result:
            # Parse the result which should be in format "<type> <name> = <value>;"
            try:
                # Find first space (end of type) and ' = ' separator
                first_space = result.find(' ')
                eq_pos = result.find('=')
                semi_pos = result.rfind(';')
                if first_space == -1 or eq_pos == -1:
                    raise ValueError("missing separators")
                ctype = result[:first_space].strip()
                name = result[first_space:eq_pos].strip()
                # Remove any '=' from value start and trailing ';'
                value = result[eq_pos+1: semi_pos if semi_pos != -1 else None].strip()
                self.constants[name] = Constant(name=name, value=value, ctype=ctype)
                print(f"DEBUG: Added constant: {name} = {value} ({ctype})")
            except Exception as e:
                print(f"DEBUG: Unexpected macro result format: {result} ({e})")


    def generate_bindings(self) -> str:
        """Generates the full Nature language binding code as a string."""

        def generate_constants_and_enums():
            lines = []
            if self.constants:
                lines.append("// Constants from Macros")
                # Simple alphabetical sort is sufficient for most cases
                for const in sorted(self.constants.values(), key=lambda c: c.name):
                    lines.append(f"{const.ctype} {const.name} = {const.value}")

            if self.enums:
                lines.append("\n// Enum Constants")
                for enum in self.enums.values():
                    for member in enum.members:
                        lines.append(f"int {enum.name}_{member.name} = {member.value}")
            return "\n".join(lines)

        def generate_records():
            lines = []
            # Generate unions first, as they are simple type aliases
            if self.unions:
                lines.append("\n// Union Definitions (as byte arrays)\n")
                # Use a set to only define each size-based union type once
                defined_unions = set()
                for union in self.unions.values():
                    if union.name not in defined_unions:
                        lines.append(union.to_nature())
                        # Add helper constructor for writing typed value into byte array
                        if union.size in (4, 8):
                            elem_count = union.size
                            zeros = ",".join(["zero"] * elem_count)
                            lines.append("")
                            lines.append(f"fn new{union.name}<T>(T value):{union.name} {{")
                            lines.append("    u8 zero = 0 as u8")
                            lines.append(f"    {union.name} result = [{zeros}]")
                            lines.append(f"    result as anyptr as rawptr<T> as T = value")
                            lines.append("    return result")
                            lines.append("}")
                        defined_unions.add(union.name)

            if self.structs:
                lines.append("\n// Struct Definitions")
                for struct in self.structs.values():
                    lines.append(f"type {struct.name} = struct {{")
                    for f in struct.fields:
                        lines.append(f"    {f.ntype} {f.name}")
                    lines.append("}")
                    lines.append("")
            return "\n".join(lines)

        def generate_functions():
            lines = ["\n// Function Bindings"]
            for func in self.functions.values():
                # Add linkid tag for C interop
                lines.append(f'#linkid {func.c_name}')

                param_list = ", ".join(f"{p.ntype} {p.name}" for p in func.parameters)

                # Handle variadic functions according to Nature syntax
                if func.is_variadic:
                    if param_list: param_list += ", "
                    # Assuming variadic params are ints; this could be made configurable
                    param_list += "...[any] args"

                return_type = f":{func.return_type}" if func.return_type != "void" else ""
                lines.append(f"fn {func.name}({param_list}){return_type}\n")
            return "\n".join(lines)

        # Assemble the final code
        header = "// Generated Nature bindings\n// This file was automatically generated naturebindgen.\n"
        code_parts = [
            header,
            generate_constants_and_enums(),
            generate_records(),
            generate_functions(),
        ]
        return "\n".join(filter(None, code_parts))


# --- Main Execution ---
def main():
    """Command-line interface for the binding generator."""
    parser = argparse.ArgumentParser(
        description="Generate Nature language bindings from a C header file."
    )
    parser.add_argument("header", help="Path to the C header file to parse.")
    parser.add_argument(
        "-o", "--output", default="bindings.n",
        help="Path to the output Nature file (default: bindings.n)."
    )
    parser.add_argument(
        "-I", dest="include_dirs", action="append", default=[],
        help="Add a directory to the Clang include path (e.g., -I/usr/include)."
    )

    args = parser.parse_args()


    clang_args = [f"-I{d}" for d in args.include_dirs]

    generator = BindingGenerator()
    try:
        generator.parse_header(args.header, clang_args)

        print("\n--- Parsing Summary ---")
        print(f"Structs: {len(generator.structs)}, Unions: {len(generator.unions)}, Enums: {len(generator.enums)}")
        print(f"Functions: {len(generator.functions)}, Constants: {len(generator.constants)}, Typedefs: {len(generator.typedefs)}")
        print("-----------------------")

        output_code = generator.generate_bindings()

        with open(args.output, "w") as f:
            f.write(output_code)

        print(f"\nSuccessfully generated Nature bindings at: {args.output}")

    except (FileNotFoundError, RuntimeError) as e:
        print(f"An error occurred: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
