from typing import Optional, List, Dict, Any
from clang.cindex import Index, TranslationUnit, CursorKind, TypeKind

class MacroProcessor:
    def __init__(self, structs: Dict[str, Any], unions: Dict[str, Any]):
        self.structs = structs
        self.unions = unions

    def _map_c_type_to_nature(self, c_type) -> str:
        """Lightweight C type to Nature type mapping for macros."""
        if c_type is None:
            return "any"

        kind = c_type.kind
        spelling = c_type.spelling.replace("const ", "").strip()

        # Pointers
        if kind == TypeKind.POINTER:
            pointee = c_type.get_pointee()
            if pointee.kind == TypeKind.CHAR_S:
                return "anyptr"
            return "anyptr"

        # Arrays
        if kind == TypeKind.CONSTANTARRAY:
            elem = c_type.get_array_element_type()
            size = c_type.get_array_size()
            elem_nt = self._map_c_type_to_nature(elem)
            return f"[{elem_nt};{size}]"

        # Records
        if kind == TypeKind.RECORD:
            decl = c_type.get_declaration()
            name = decl.spelling or spelling
            name = name.replace("struct ", "").replace("union ", "").strip()
            if name in self.structs:
                return name
            if name in self.unions:
                return self.unions[name].name
            return name or spelling or "any"

        # Typedef and canonical fallbacks
        canon = c_type.get_canonical()
        canon_sp = canon.spelling.replace("const ", "").strip()

        basic_map = {
            TypeKind.VOID: "void",
            TypeKind.BOOL: "bool",
            TypeKind.CHAR_S: "i8",
            TypeKind.SCHAR: "i8",
            TypeKind.UCHAR: "u8",
            TypeKind.SHORT: "i16",
            TypeKind.USHORT: "u16",
            TypeKind.INT: "i32",
            TypeKind.UINT: "u32",
            TypeKind.LONG: "i64",
            TypeKind.ULONG: "u64",
            TypeKind.LONGLONG: "i64",
            TypeKind.ULONGLONG: "u64",
            TypeKind.FLOAT: "f32",
            TypeKind.DOUBLE: "f64",
            TypeKind.LONGDOUBLE: "f64",
        }
        if canon.kind in basic_map:
            return basic_map[canon.kind]

        # Fallbacks
        if spelling:
            return spelling
        return "any"

    def _expr_to_str(self, expr) -> str:
        """Convert a clang expression to a string value."""
        if expr.kind == CursorKind.INTEGER_LITERAL:
            # Get the tokens to get the exact literal text
            tokens = list(expr.get_tokens())
            if tokens:
                return tokens[0].spelling
            return "0"  # Fallback
        
        elif expr.kind == CursorKind.FLOATING_LITERAL:
            tokens = list(expr.get_tokens())
            if tokens:
                return tokens[0].spelling
            return "0.0"  # Fallback
        
        elif expr.kind == CursorKind.STRING_LITERAL:
            # Handle string literals with proper escaping
            return f'"{expr.spelling}".ref()'
        
        elif expr.kind in (CursorKind.UNEXPOSED_EXPR, CursorKind.COMPOUND_LITERAL_EXPR) and any(c.kind == CursorKind.INIT_LIST_EXPR for c in expr.get_children()):
            # Handle compound literals (structs/unions)
            type_decl = expr.type.get_declaration() if expr.type else None
            if type_decl and type_decl.kind == CursorKind.STRUCT_DECL:
                struct_name = type_decl.spelling
                if struct_name in self.structs:
                    struct = self.structs[struct_name]
                    init_children = list(expr.get_children())[0].get_children() if expr.kind == CursorKind.UNEXPOSED_EXPR else list(expr.get_children())
                    fields = []
                    for i, field in enumerate(struct.fields):
                        try:
                            init = list(init_children)[i]
                            fields.append(f"{field.name}={self._expr_to_str(init)}")
                        except Exception:
                            break
                    return f"{struct_name}{{{', '.join(fields)}}}"
            elif type_decl and type_decl.kind == CursorKind.UNION_DECL:
                union_name = type_decl.spelling
                if union_name in self.unions:
                    union = self.unions[union_name]
                    # For unions, we only use the first field's initialization
                    init_children = list(expr.get_children())[0].get_children() if expr.kind == CursorKind.UNEXPOSED_EXPR else list(expr.get_children())
                    if init_children:
                        field = union.fields[0]
                        return f"{union.name}{{{field.name}={self._expr_to_str(init_children[0])}}}"
        
        # Fallback: get the raw text
        tokens = list(expr.get_tokens())
        if tokens:
            return " ".join(t.spelling for t in tokens)
        return "<unknown>"

    def process_macro(self, header_path: str, define_name: str, clang_args: Optional[List[str]] = None) -> Optional[str]:
        """Process a macro definition using Python-side type information."""
        # Skip system headers
        if header_path.startswith('<') and header_path.endswith('>'):
            print(f"DEBUG: Skipping system header: {header_path}")
            return None

        # Generate code to evaluate the macro
        code = (
            f'#include "{header_path}"\n'
            f'static const __typeof__({define_name}) __dummy_var = {define_name};\n'
        )
        print(f"DEBUG: Generated code:\n{code}")

        # Parse the code
        index = Index.create()
        args = ['-x', 'c', '-std=c11'] + (clang_args or [])
        
        # Add include path for the header's directory
        if '/' in header_path:
            header_dir = header_path.rsplit('/', 1)[0]
            args.append(f'-I{header_dir}')

        print(f"DEBUG: MacroProcessor.parse args: {args}")
        tu = index.parse('tmp.c', args=args, unsaved_files=[('tmp.c', code)])
        if not tu:
            print("DEBUG: Failed to parse translation unit")
            return None

        # Find our dummy variable
        for cursor in tu.cursor.get_children():
            if (cursor.kind == CursorKind.VAR_DECL and 
                cursor.spelling == '__dummy_var' and 
                cursor.location.file and 
                str(cursor.location.file) == 'tmp.c'):
                print(f"DEBUG: Found __dummy_var: type='{cursor.type.spelling if cursor.type else None}'")
                
                if not cursor.type:
                    print("DEBUG: No type information for macro value")
                    return None

                type_name = self._map_c_type_to_nature(cursor.type)

                if not type_name:
                    print(f"DEBUG: Could not determine type for {define_name}")
                    return None

                # Reconstruct RHS text from tokens for robust parsing
                tok_list = list(cursor.get_tokens())
                print(f"DEBUG: VAR_DECL tokens: {[t.spelling for t in tok_list]}")
                eq_index = -1
                for i, t in enumerate(tok_list):
                    if t.spelling == '=':
                        eq_index = i
                        break
                rhs_text = ''
                if eq_index != -1:
                    expr_tokens = tok_list[eq_index + 1:]
                    if expr_tokens and expr_tokens[-1].spelling == ';':
                        expr_tokens = expr_tokens[:-1]
                    rhs_text = ''.join(t.spelling for t in expr_tokens).strip()
                # If there's no RHS at all (header guards etc.), skip
                if not rhs_text:
                    return None

                # Prefer AST child initializer if present
                children = list(cursor.get_children())
                if children:
                    value = self._expr_to_str(children[0])
                    print(f"DEBUG: Initializer child expr -> '{value}' from kind={children[0].kind}")
                else:
                    value = rhs_text
                    print(f"DEBUG: Initializer from RHS text -> '{value}'")

                # If value still unknown, try to parse C compound literal
                if value == '<unknown>' and rhs_text:
                    import re as _re
                    # (Type){...}
                    m = _re.match(r"^\(\s*(?:struct\s+)?(\w+)\s*\)\s*\{([\s\S]*)\}$", rhs_text)
                    if m:
                        struct_name = m.group(1)
                        if struct_name in self.structs:
                            raw_vals = m.group(2)
                            parts = [p.strip() for p in raw_vals.split(',') if p.strip()]
                            field_names = [f.name for f in self.structs[struct_name].fields]
                            pairs = []
                            for i, fname in enumerate(field_names):
                                if i < len(parts):
                                    pairs.append(f"{fname}={parts[i]}")
                            value = f"{struct_name}{{{','.join(pairs)}}}"
                            type_name = struct_name
                            print(f"DEBUG: Parsed compound literal (Type){{...}} -> '{value}'")
                    else:
                        # Type{...}
                        m2 = _re.match(r"^(?:struct\s+)?(\w+)\s*\{([\s\S]*)\}$", rhs_text)
                        if m2:
                            struct_name = m2.group(1)
                            if struct_name in self.structs:
                                raw_vals = m2.group(2)
                                parts = [p.strip() for p in raw_vals.split(',') if p.strip()]
                                field_names = [f.name for f in self.structs[struct_name].fields]
                                pairs = []
                                for i, fname in enumerate(field_names):
                                    if i < len(parts):
                                        pairs.append(f"{fname}={parts[i]}")
                                value = f"{struct_name}{{{','.join(pairs)}}}"
                                type_name = struct_name
                                print(f"DEBUG: Parsed compound literal Type{{...}} -> '{value}'")

                # Normalize string literal to Nature pointer form
                if isinstance(value, str) and value.startswith('"') and value.endswith('"'):
                    value = f'{value}.ref()'
                    type_name = 'anyptr'

                # If the value looks like Struct{...}, ensure type_name matches
                if isinstance(value, str):
                    import re as _re
                    m3 = _re.match(r"^(\w+)\s*\{", value)
                    if m3 and m3.group(1) in self.structs:
                        type_name = m3.group(1)

                return f"{type_name} {define_name} = {value};"

        print("DEBUG: Could not find macro value")
        return None
