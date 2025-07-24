package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/divan/num2words"
	"github.com/go-clang/clang-v13/clang"
)

// TypeMapping represents the mapping between C types and Nature types
type TypeMapping struct {
	CType      string
	NatureType string
	IsPointer  bool
}

// FunctionBinding represents a C function binding
type FunctionBinding struct {
	Name       string
	CName      string
	Parameters []Parameter
	ReturnType string
}

// Parameter represents a function parameter
type Parameter struct {
	Name string
	Type string
}

// StructBinding represents a C struct binding
type StructBinding struct {
	Name   string
	Fields []StructField
}

// StructField represents a struct field
type StructField struct {
	Name        string
	Type        string
	Nested      *StructBinding // For nested structs/unions
	IsUnion     bool           // True if this field is a union
	UnionFields []StructField  // If this is a union, these are the union fields
}

// EnumBinding represents a C enum binding
type EnumBinding struct {
	Name    string
	Members []EnumMember
}

// EnumMember represents a member of an enum
type EnumMember struct {
	Name    string
	Value   int
	Literal string // the literal value after '=' or empty if not present
}

type ConstantItem struct {
	Name  string
	Type  string
	Value string
}

// UnionBinding represents a C union binding
type UnionBinding struct {
	Name            string
	BiggestTypeSize int64
	Fields          []StructField
}

func NewUnionBinding(name string, biggestTypeSize int64, fields []StructField) *UnionBinding {
	return &UnionBinding{
		Name:            name,
		BiggestTypeSize: biggestTypeSize,
		Fields:          fields,
	}
}

// Generate Nature code for the union as a type alias to [u8;N] and extension functions
func (ub *UnionBinding) ToNature(bg *BindingGenerator) string {
	var sb strings.Builder

	unionSizeBytes := ub.BiggestTypeSize
	unionTypeName := fmt.Sprintf("Union_%s_bytes", num2words.Convert(int(unionSizeBytes)))
	sb.WriteString(fmt.Sprintf("type %s = [u8;%d]\n\n", unionTypeName, unionSizeBytes))

	// Track generated function names to avoid duplicates
	generatedFunctions := make(map[string]bool)

	// Generate helper functions for each field as extension methods
	for _, field := range ub.Fields {
		cleanFieldName := strings.TrimSuffix(field.Name, "_")
		cleanFieldType := field.Type
		if strings.Contains(cleanFieldType, " at ") {
			for structName := range bg.structs {
				if strings.Contains(cleanFieldType, structName) {
					cleanFieldType = structName
					break
				}
			}
			if strings.Contains(cleanFieldType, " at ") {
				for name := range bg.structs {
					if strings.HasPrefix(name, "AnonymousStruct_") {
						cleanFieldType = name
						break
					}
				}
			}
		}

		typeSuffix := cleanFieldType
		if strings.Contains(typeSuffix, "[") {
			parts := strings.Split(typeSuffix, "[")
			baseType := parts[0]
			sizePart := strings.TrimRight(parts[1], "]")
			sizePart = strings.ReplaceAll(sizePart, ";", "_")
			typeSuffix = fmt.Sprintf("%s_%s", baseType, sizePart)
		}
		if strings.Contains(typeSuffix, "ptr") {
			typeSuffix = strings.ReplaceAll(typeSuffix, "rawptr<", "")
			typeSuffix = strings.ReplaceAll(typeSuffix, ">", "")
		}

		getterName := fmt.Sprintf("get_%s_%s", cleanFieldName, typeSuffix)
		setterName := fmt.Sprintf("set_%s_%s", cleanFieldName, typeSuffix)

		if generatedFunctions[getterName] || generatedFunctions[setterName] {
			continue
		}
		generatedFunctions[getterName] = true
		generatedFunctions[setterName] = true

		// Getter extension function (no explicit self argument)
		sb.WriteString(fmt.Sprintf("fn %s.%s():%s {\n", unionTypeName, getterName, cleanFieldType))
		sb.WriteString(fmt.Sprintf("    return self as %s\n", cleanFieldType))
		sb.WriteString("}\n\n")

		// Setter extension function (no explicit self argument, but use 'self' in body)
		sb.WriteString(fmt.Sprintf("fn %s.%s(value %s) {\n", unionTypeName, setterName, cleanFieldType))
		sb.WriteString(fmt.Sprintf("    self = value as [u8;%d]\n", unionSizeBytes))
		sb.WriteString("}\n\n")
	}

	// No .to_c() method needed; type is already C-representable

	return sb.String()
}

// BindingGenerator generates Nature bindings from C headers
type BindingGenerator struct {
	typeMappings         map[string]TypeMapping
	functions            map[string]FunctionBinding
	structs              map[string]StructBinding
	constants            map[string]ConstantItem
	unions               map[int64]*UnionBinding
	unionNames           map[string]int64 // Map union names to their sizes for type mapping
	includes             []string
	enums                map[string]EnumBinding
	constantValues       map[string]int
	includedFiles        map[string]bool
	baseDir              string
	headerLog            strings.Builder
	nestedStructCounters map[string]int
	processedCursors     map[clang.Cursor]bool // Track processed cursors to avoid duplicates
	anonTypeNameMap      map[string]string     // Map clang spelling to context-based name
}

func areStringsEqualAfterDynamicPrefixTrim(s1, s2 string) bool {
	prefixConst := "AnonymousStruct_"
	suffixConst := "_"

	// 1. Check if both strings start with "AnonymousStruct_"
	if !strings.HasPrefix(s1, prefixConst) || !strings.HasPrefix(s2, prefixConst) {
		return false
	}

	// 2. Find the end of the number part for s1
	s1AfterPrefix := s1[len(prefixConst):] // "123_some_unique_id"
	idx1 := strings.Index(s1AfterPrefix, suffixConst)
	if idx1 == -1 { // "_ " not found after number
		return false
	}

	// 3. Extract the number string and the rest for s1
	numStr1 := s1AfterPrefix[:idx1]       // "123"
	s1Remainder := s1AfterPrefix[idx1+1:] // "some_unique_id"

	// 4. Convert number string to integer for validation
	_, err := strconv.Atoi(numStr1)
	if err != nil { // Not a valid number
		return false
	}

	// 5. Repeat for s2
	s2AfterPrefix := s2[len(prefixConst):]
	idx2 := strings.Index(s2AfterPrefix, suffixConst)
	if idx2 == -1 {
		return false
	}

	numStr2 := s2AfterPrefix[:idx2]
	s2Remainder := s2AfterPrefix[idx2+1:]

	_, err = strconv.Atoi(numStr2)
	if err != nil {
		return false
	}

	// 6. Check if the extracted number strings are identical
	if numStr1 != numStr2 {
		return false
	}

	// 7. Finally, compare the remaining parts of the strings
	return s1Remainder == s2Remainder
}

func IsLiteral(kind clang.CursorKind) bool {
	return int(kind) >= int(clang.Cursor_IntegerLiteral) && int(kind) <= int(clang.Cursor_StringLiteral) || kind == clang.Cursor_VarDecl
}

// NewBindingGenerator creates a new binding generator
func NewBindingGenerator() *BindingGenerator {
	bg := &BindingGenerator{
		typeMappings:         make(map[string]TypeMapping),
		functions:            make(map[string]FunctionBinding),
		structs:              make(map[string]StructBinding),
		constants:            make(map[string]ConstantItem),
		unions:               make(map[int64]*UnionBinding),
		unionNames:           make(map[string]int64),
		includes:             make([]string, 0),
		enums:                make(map[string]EnumBinding),
		constantValues:       make(map[string]int),
		includedFiles:        make(map[string]bool),
		nestedStructCounters: make(map[string]int),
		processedCursors:     make(map[clang.Cursor]bool),
		anonTypeNameMap:      make(map[string]string),
	}

	// Initialize default type mappings based on Nature documentation
	bg.initializeTypeMappings()

	return bg
}

// initializeTypeMappings sets up the default C to Nature type mappings
func (bg *BindingGenerator) initializeTypeMappings() {
	mappings := []TypeMapping{
		{"void", "void", false},
		{"char", "i8", false},
		{"signed char", "i8", false},
		{"unsigned char", "u8", false},
		{"short", "i16", false},
		{"short int", "i16", false},
		{"unsigned short", "u16", false},
		{"unsigned short int", "u16", false},
		{"int", "i32", false},
		{"signed int", "i32", false},
		{"unsigned int", "u32", false},
		{"unsigned", "u32", false},
		{"long", "i64", false},
		{"long int", "i64", false},
		{"unsigned long", "u64", false},
		{"unsigned long int", "u64", false},
		{"long long", "i64", false},
		{"long long int", "i64", false},
		{"unsigned long long", "u64", false},
		{"unsigned long long int", "u64", false},
		{"float", "f32", false},
		{"double", "f64", false},
		{"long double", "f64", false},
		{"size_t", "uint", false},
		{"ssize_t", "int", false},
		{"ptrdiff_t", "int", false},
		{"uintptr_t", "anyptr", false},
		{"intptr_t", "anyptr", false},
		{"int8_t", "i8", false},
		{"uint8_t", "u8", false},
		{"int16_t", "i16", false},
		{"uint16_t", "u16", false},
		{"int32_t", "i32", false},
		{"uint32_t", "u32", false},
		{"int64_t", "i64", false},
		{"uint64_t", "u64", false},
		{"bool", "bool", false},
		{"_Bool", "bool", false},
		// SDL-specific types
		{"Uint8", "u8", false},
		{"Uint16", "u16", false},
		{"Uint32", "u32", false},
		{"Uint64", "u64", false},
		{"Sint8", "i8", false},
		{"Sint16", "i16", false},
		{"Sint32", "i32", false},
		{"Sint64", "i64", false},
		// Pointer types
		{"void*", "anyptr", true},
		{"char*", "anyptr", true},
		{"const char*", "anyptr", true},
	}

	for _, mapping := range mappings {
		bg.typeMappings[mapping.CType] = mapping
	}
}

func (bg *BindingGenerator) mapCursorKindToCType(kind clang.CursorKind) string {
	switch kind {
	case clang.Cursor_IntegerLiteral:
		return "int"
	case clang.Cursor_FloatingLiteral:
		return "float"
	case clang.Cursor_StringLiteral:
		return "string"
	case clang.Cursor_CharacterLiteral:
		return "char"
	default:
		return ""
	}
}

// mapCTypeToNature converts a C type to its Nature equivalent
func (bg *BindingGenerator) mapCTypeToNature(cType string) string {
	// Clean up the type string
	cType = strings.TrimSpace(cType)
	cType = regexp.MustCompile(`\s+`).ReplaceAllString(cType, " ")

	// Handle function pointer types
	if strings.Contains(cType, "(*") && strings.Contains(cType, ")(") {
		return "anyptr"
	}

	// Handle pointer types
	if strings.Contains(cType, "*") {
		baseType := strings.TrimSpace(strings.Replace(cType, "*", "", -1))

		// Check if it's a pointer to a known struct
		for _, structDef := range bg.structs {
			if baseType == structDef.Name || baseType == "struct "+structDef.Name {
				return fmt.Sprintf("rawptr<%s>", structDef.Name)
			}
		}

		// Check if it's a pointer to a known enum type
		for _, enumDef := range bg.enums {
			if baseType == enumDef.Name {
				return "rawptr<int>"
			}
		}

		// Default pointer types
		if baseType == "void" || baseType == "char" || baseType == "const char" || baseType == "const void" {
			return "anyptr"
		}
		return "anyptr"
	}

	// Handle array types
	if strings.Contains(cType, "[") {
		parts := strings.Split(cType, "[")
		baseType := strings.TrimSpace(parts[0])
		sizeStr := strings.TrimRight(parts[1], "]")

		// Check if the base type is an anonymous struct that we need to canonicalize
		if mapped, ok := bg.anonTypeNameMap[baseType]; ok {
			bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Canonicalizing array base type '%s' to '%s'\n", baseType, mapped))
			baseType = mapped
		}

		natureBaseType := bg.mapCTypeToNature(baseType)
		if sizeStr == "" {
			return fmt.Sprintf("[%s]", natureBaseType)
		}
		return fmt.Sprintf("[%s;%s]", natureBaseType, sizeStr)
	}

	// Handle "struct" keyword
	if strings.HasPrefix(cType, "struct ") {
		structName := strings.TrimSpace(strings.TrimPrefix(cType, "struct "))
		return structName
	}

	// Handle "union" keyword
	if strings.HasPrefix(cType, "union ") {
		unionName := strings.TrimSpace(strings.TrimPrefix(cType, "union "))
		return unionName
	}

	// Direct mapping
	if mapping, exists := bg.typeMappings[cType]; exists {
		return mapping.NatureType
	}

	// Check if it's a known struct type
	for _, structDef := range bg.structs {
		if cType == structDef.Name {
			return structDef.Name
		}
	}

	// Check if it's a known union type
	if unionSize, exists := bg.unionNames[cType]; exists {
		// Return the union type name based on size
		unionTypeName := fmt.Sprintf("Union_%s_bytes", num2words.Convert(int(unionSize)))
		bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Mapped union type %s to %s\n", cType, unionTypeName))
		return unionTypeName
	}

	// Check if it's a known enum type
	for _, enumDef := range bg.enums {
		if cType == enumDef.Name {
			return "int"
		}
	}

	// Default to any for truly unknown types
	return "any"
}

// parseHeaderFile parses a C header file using go-clang
func (bg *BindingGenerator) parseHeaderFile(filename string) error {
	// Mark this file as included
	bg.includedFiles[filename] = true
	bg.headerLog.WriteString(fmt.Sprintf("Parsing header: %s\n", filename))

	// Determine the base directory for relative includes
	if bg.baseDir == "" {
		bg.baseDir = filepath.Dir(filename)
	}

	// Create a temporary C file to parse the header
	tempFile := filename + "_temp.c"
	tempContent := fmt.Sprintf("#include \"%s\"\n", filepath.Base(filename))

	if err := os.WriteFile(tempFile, []byte(tempContent), 0644); err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile)

	// Parse with clang
	idx := clang.NewIndex(0, 0)
	defer idx.Dispose()

	tu := idx.ParseTranslationUnit(tempFile, nil, nil, 0)
	if tu == (clang.TranslationUnit{}) {
		return fmt.Errorf("failed to parse translation unit")
	}
	defer tu.Dispose()

	// Get the cursor for the translation unit
	cursor := tu.TranslationUnitCursor()

	// Visit all children to find declarations
	bg.visitCursor(cursor, 0)

	return nil
}

// visitCursor recursively visits all cursors in the AST
func (bg *BindingGenerator) visitCursor(cursor clang.Cursor, depth int) {
	// Skip system headers
	if cursor.Location().IsInSystemHeader() {
		return
	}

	// Check if cursor has already been processed
	if _, exists := bg.processedCursors[cursor]; exists {
		return
	}
	bg.processedCursors[cursor] = true

	kind := cursor.Kind()
	spelling := cursor.Spelling()

	bg.headerLog.WriteString(fmt.Sprintf("%s[DEBUG] Visiting cursor: %s (%s) at depth %d\n",
		strings.Repeat("  ", depth), spelling, kind.String(), depth))

	switch kind {
	case clang.Cursor_StructDecl:
		// For anonymous structs, we need to find a proper context
		if spelling == "" || strings.Contains(spelling, "unnamed") || strings.Contains(spelling, " at ") {
			// This is an anonymous struct, we need to find its parent context
			parent := cursor.SemanticParent()
			if parent.Kind() == clang.Cursor_TypedefDecl {
				// This is a typedef struct, use the typedef name as context
				typedefName := parent.Spelling()
				bg.handleCursorStructDecl(cursor, typedefName, depth)
			} else {
				// For truly anonymous structs, use a generic context
				bg.handleCursorStructDecl(cursor, "AnonymousStruct", depth)
			}
		} else {
			bg.handleCursorStructDecl(cursor, spelling, depth)
		}
	case clang.Cursor_FieldDecl:
		bg.handleFieldDecl(cursor, nil, depth)
	case clang.Cursor_TypedefDecl:
		bg.handleTypedefDecl(cursor, depth)
	case clang.Cursor_FunctionDecl:
		bg.handleFunctionDecl(cursor, depth)
	case clang.Cursor_EnumDecl:
		bg.handleEnumDecl(cursor, depth)
	case clang.Cursor_UnionDecl:
		// For anonymous unions, we need to find a proper context
		if spelling == "" || strings.Contains(spelling, "unnamed") || strings.Contains(spelling, " at ") {
			// This is an anonymous union, we need to find its parent context
			parent := cursor.SemanticParent()
			if parent.Kind() == clang.Cursor_TypedefDecl {
				// This is a typedef union, use the typedef name as context
				typedefName := parent.Spelling()
				bg.handleCursorUnionDecl(cursor, typedefName, depth)
			} else {
				// For truly anonymous unions, use a generic context
				bg.handleCursorUnionDecl(cursor, "AnonymousUnion", depth)
			}
		} else {
			bg.handleCursorUnionDecl(cursor, spelling, depth)
		}
	case clang.Cursor_MacroDefinition: // Only call handleMacroDefinition for actual macros
		literalType := cursor.Type() // Although for MacroDefinition, this might not be strictly a "literal type" but the underlying type of the macro's expansion if it's a constant.
		bg.handleMacroDefinition(cursor, depth, literalType)
	case clang.Cursor_InclusionDirective:
		bg.handleIncludeDirective(cursor, depth)
	default:
		bg.headerLog.WriteString(fmt.Sprintf("%s[DEBUG] Unknown cursor kind: %s, %d\n", strings.Repeat("  ", depth), kind.String(), int(kind)))
		initalizerCursor := cursor.VarDeclInitializer()

		if !initalizerCursor.IsNull() {
			kind = initalizerCursor.Kind()
		}
	}

	// Visit children
	cursor.Visit(func(cursor, parent clang.Cursor) clang.ChildVisitResult {
		bg.visitCursor(cursor, depth+1)
		return clang.ChildVisit_Continue
	})
}

// Recursive handler for struct declarations
func (bg *BindingGenerator) handleCursorStructDecl(cursor clang.Cursor, context string, depth int) {
	spelling := cursor.Spelling()
	isAnonymous := spelling == "" || strings.Contains(spelling, "unnamed") || strings.Contains(spelling, " at ")
	var structName string

	if isAnonymous {
		structName = context
		if structName == "" {
			structName = "AnonymousStruct"
		}
		// Always map the Clang spelling to our context-based name
		if spelling != "" {
			bg.anonTypeNameMap[spelling] = structName
			bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Mapping clang anonymous name '%s' to context name '%s'\n", spelling, structName))
		}
	} else {
		// If this spelling is mapped to a context name, use the context name instead
		if mapped, ok := bg.anonTypeNameMap[spelling]; ok {
			structName = mapped
			bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Using mapped context name '%s' for clang spelling '%s'\n", structName, spelling))
		} else {
			structName = spelling
		}
	}

	// Only register if not already registered under the context name
	if _, exists := bg.structs[structName]; exists {
		bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Skipping already registered struct: %s (spelling: '%s', context: '%s')\n", structName, spelling, context))
		return // Already processed
	}

	bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Registering struct: %s (spelling: '%s', context: '%s')\n", structName, spelling, context))

	structBinding := StructBinding{
		Name:   structName,
		Fields: make([]StructField, 0),
	}

	// Track seen fields to prevent duplicates during processing
	seenFields := make(map[string]bool)

	// Process fields
	cursor.Visit(func(child clang.Cursor, parent clang.Cursor) clang.ChildVisitResult {
		if child.Kind() == clang.Cursor_FieldDecl {
			fieldName := child.Spelling()
			fieldType := child.Type()
			typeSpelling := fieldType.Spelling()

			// Canonicalize anonymous type names
			if mapped, ok := bg.anonTypeNameMap[typeSpelling]; ok {
				bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Canonicalizing field type '%s' to '%s' for field '%s' in struct '%s'\n", typeSpelling, mapped, fieldName, structName))
				typeSpelling = mapped
			}

			// Create field key for deduplication
			fieldKey := fmt.Sprintf("%s:%s", fieldName, typeSpelling)
			if seenFields[fieldKey] {
				bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Skipping duplicate field during processing: %s of type %s in struct %s\n", fieldName, typeSpelling, structName))
				return clang.ChildVisit_Continue
			}
			seenFields[fieldKey] = true

			if fieldType.CanonicalType().Kind() == clang.Type_Record {
				declKind := fieldType.Declaration().Kind()

				switch declKind {
				case clang.Cursor_StructDecl:
					// Nested struct - use proper context-based naming
					childContext := structName + "_" + fieldName + "_Struct"
					bg.handleCursorStructDecl(fieldType.Declaration(), childContext, depth+1)
					structBinding.Fields = append(structBinding.Fields, StructField{
						Name: bg.renameReservedKeywords(fieldName),
						Type: childContext,
					})
				case clang.Cursor_UnionDecl:
					// Nested union
					childContext := structName + "_" + fieldName + "_Union"
					bg.handleCursorUnionDecl(fieldType.Declaration(), childContext, depth+1)
					// Use size-based name for union
					unionSize := fieldType.SizeOf()
					unionTypeName := "Union_" + num2words.Convert(int(unionSize)) + "_bytes"
					structBinding.Fields = append(structBinding.Fields, StructField{
						Name: bg.renameReservedKeywords(fieldName),
						Type: unionTypeName,
					})
				default:
					natureType := bg.mapCTypeToNature(typeSpelling)
					structBinding.Fields = append(structBinding.Fields, StructField{
						Name: bg.renameReservedKeywords(fieldName),
						Type: natureType,
					})
				}
			} else {
				// Check if this is an array type that contains an anonymous struct
				if strings.Contains(typeSpelling, "[") && strings.Contains(typeSpelling, "struct (unnamed") {
					// Extract the base type and size
					parts := strings.Split(typeSpelling, "[")
					baseType := strings.TrimSpace(parts[0])
					sizePart := strings.TrimRight(parts[1], "]")

					// Create a context name for the anonymous struct in the array
					arrayStructContext := structName + "_" + fieldName + "_Struct"

					// Map the anonymous struct to our context name
					if _, ok := bg.anonTypeNameMap[baseType]; !ok {
						bg.anonTypeNameMap[baseType] = arrayStructContext
						bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Mapping array anonymous struct '%s' to '%s'\n", baseType, arrayStructContext))
					}

					// Generate the array type with the canonicalized name
					natureType := fmt.Sprintf("[%s;%s]", arrayStructContext, sizePart)
					structBinding.Fields = append(structBinding.Fields, StructField{
						Name: bg.renameReservedKeywords(fieldName),
						Type: natureType,
					})
				} else {
					natureType := bg.mapCTypeToNature(typeSpelling)
					structBinding.Fields = append(structBinding.Fields, StructField{
						Name: bg.renameReservedKeywords(fieldName),
						Type: natureType,
					})
				}
			}
		}
		return clang.ChildVisit_Continue
	})

	bg.structs[structName] = structBinding
}

// Recursive handler for union declarations
func (bg *BindingGenerator) handleCursorUnionDecl(cursor clang.Cursor, context string, depth int) {
	unionSize := cursor.Type().SizeOf()
	unionTypeName := "Union_" + num2words.Convert(int(unionSize)) + "_bytes"
	if _, exists := bg.unions[unionSize]; exists {
		bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Skipping already registered union: %s (context: '%s')\n", unionTypeName, context))
		return // Already processed
	}
	bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Registering union: %s (context: '%s')\n", unionTypeName, context))
	var unionFields []StructField
	cursor.Visit(func(child clang.Cursor, parent clang.Cursor) clang.ChildVisitResult {
		if child.Kind() == clang.Cursor_FieldDecl {
			fieldName := child.Spelling()
			fieldType := child.Type()
			typeSpelling := fieldType.Spelling()
			// Canonicalize anonymous type names
			if mapped, ok := bg.anonTypeNameMap[typeSpelling]; ok {
				bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Canonicalizing union field type '%s' to '%s' for field '%s' in union '%s'\n", typeSpelling, mapped, fieldName, unionTypeName))
				typeSpelling = mapped
			}
			if fieldType.CanonicalType().Kind() == clang.Type_Record {
				declKind := fieldType.Declaration().Kind()
				if declKind == clang.Cursor_StructDecl {
					// Nested struct in union
					childContext := unionTypeName + "_" + fieldName + "_Struct"
					bg.handleCursorStructDecl(fieldType.Declaration(), childContext, depth+1)
					unionFields = append(unionFields, StructField{
						Name: bg.renameReservedKeywords(fieldName),
						Type: childContext,
					})
				} else if declKind == clang.Cursor_UnionDecl {
					// Nested union in union
					childContext := unionTypeName + "_" + fieldName + "_Union"
					bg.handleCursorUnionDecl(fieldType.Declaration(), childContext, depth+1)
					nestedUnionSize := fieldType.SizeOf()
					nestedUnionTypeName := "Union_" + num2words.Convert(int(nestedUnionSize)) + "_bytes"
					unionFields = append(unionFields, StructField{
						Name: bg.renameReservedKeywords(fieldName),
						Type: nestedUnionTypeName,
					})
				}
			} else {
				natureType := bg.mapCTypeToNature(typeSpelling)
				unionFields = append(unionFields, StructField{
					Name: bg.renameReservedKeywords(fieldName),
					Type: natureType,
				})
			}
		}
		return clang.ChildVisit_Continue
	})
	bg.unions[unionSize] = NewUnionBinding(unionTypeName, unionSize, unionFields)
	bg.unionNames[unionTypeName] = unionSize
}

// handleFieldDecl handles struct field declarations
func (bg *BindingGenerator) handleFieldDecl(cursor clang.Cursor, structBinding *StructBinding, depth int) {
	fieldName := cursor.Spelling()
	fieldType := cursor.Type()
	typeSpelling := fieldType.Spelling()

	bg.headerLog.WriteString(fmt.Sprintf("%sFound field: %s of type %s\n",
		strings.Repeat("  ", depth), fieldName, typeSpelling))

	// If structBinding is nil, we need to find the parent struct
	if structBinding == nil {
		parent := cursor.SemanticParent()
		if parent.Kind() == clang.Cursor_StructDecl {
			// Find the struct by name
			structName := parent.Spelling()
			if strings.Contains(structName, " at ") {
				parts := strings.Split(structName, " at ")
				structName = strings.TrimSpace(parts[0])
			}

			// Check if this is a typedef struct
			typedefParent := parent.SemanticParent()
			if typedefParent.Kind() == clang.Cursor_TypedefDecl {
				structName = typedefParent.Spelling()
			}

			if structName == "" || strings.Contains(structName, "unnamed") {
				// This is an anonymous struct, find it by counter
				for name := range bg.structs {
					if strings.HasPrefix(name, "AnonymousStruct_") {
						// Get the struct from the map
						structDef := bg.structs[name]
						structBinding = &structDef
						break
					}
				}
			} else {
				if structDef, exists := bg.structs[structName]; exists {
					// Get the struct from the map
					structBinding = &structDef
				}
			}
		} else if parent.Kind() == clang.Cursor_UnionDecl {
			// Union fields are now handled in handleUnionDecl when the union is declared
			// This field will be processed when the union declaration is visited
			bg.headerLog.WriteString(fmt.Sprintf("%sSkipping union field %s - will be handled in union declaration\n",
				strings.Repeat("  ", depth), fieldName))
			return
		}
	}

	// If we still don't have a struct binding, we can't process this field
	if structBinding == nil {
		bg.headerLog.WriteString(fmt.Sprintf("%sCould not find parent struct for field: %s\n",
			strings.Repeat("  ", depth), fieldName))
		return
	}

	// Handle unions - check if this field type is a union
	if strings.Contains(typeSpelling, "union") {
		bg.headerLog.WriteString(fmt.Sprintf("%sFound union field: %s\n",
			strings.Repeat("  ", depth), fieldName))

		// Get the union declaration
		fieldCursor := cursor.Type().Declaration()
		if fieldCursor.Kind() == clang.Cursor_UnionDecl {
			// Always use the size-based union type name
			unionSize := fieldType.SizeOf()
			unionTypeName := fmt.Sprintf("Union_%s_bytes", num2words.Convert(int(unionSize)))
			// Recursively process the union
			bg.handleCursorUnionDecl(fieldCursor, unionTypeName, depth+1)
			// Add the union field to the struct
			structBinding.Fields = append(structBinding.Fields, StructField{
				Name:    bg.renameReservedKeywords(fieldName),
				Type:    unionTypeName,
				IsUnion: true,
			})
			bg.headerLog.WriteString(fmt.Sprintf("%sAdded union field: %s (%s)\n",
				strings.Repeat("  ", depth), fieldName, unionTypeName))
			// Update the struct in the map
			bg.updateStructInMap(structBinding)
			return
		}
	}
	// Handle nested structs
	if fieldType.CanonicalType().Kind() == clang.Type_Record {
		fieldCursor := cursor.Type().Declaration()
		if fieldCursor.Kind() == clang.Cursor_StructDecl {
			bg.headerLog.WriteString(fmt.Sprintf("%sFound nested struct field: %s\n",
				strings.Repeat("  ", depth), fieldName))
			// Recursively process the struct with proper context-based naming
			nestedStructName := structBinding.Name + "_" + fieldName + "_Struct"
			bg.handleCursorStructDecl(fieldCursor, nestedStructName, depth+1)
			// Add field reference
			structBinding.Fields = append(structBinding.Fields, StructField{
				Name:   bg.renameReservedKeywords(fieldName),
				Type:   nestedStructName,
				Nested: nil,
			})
			bg.headerLog.WriteString(fmt.Sprintf("%sAdded nested struct: %s\n",
				strings.Repeat("  ", depth), nestedStructName))
			// Update the struct in the map
			bg.updateStructInMap(structBinding)
			return
		}
	}

	// Regular field
	natureType := bg.mapCTypeToNature(typeSpelling)
	structBinding.Fields = append(structBinding.Fields, StructField{
		Name: bg.renameReservedKeywords(fieldName),
		Type: natureType,
	})

	// Update the struct in the map
	bg.updateStructInMap(structBinding)
}

// updateStructInMap updates a struct binding in the map
func (bg *BindingGenerator) updateStructInMap(structBinding *StructBinding) {
	// Find and update the struct in the map
	for name, existingStruct := range bg.structs {
		if existingStruct.Name == structBinding.Name {
			// Deduplicate fields before updating
			deduplicatedFields := bg.deduplicateStructFields(structBinding.Fields)
			structBinding.Fields = deduplicatedFields

			bg.structs[name] = *structBinding
			bg.headerLog.WriteString(fmt.Sprintf("Updated struct %s in map with %d fields\n",
				structBinding.Name, len(structBinding.Fields)))
			return
		} else if (strings.HasPrefix(existingStruct.Name, "AnonymousStruct_") && strings.HasPrefix(structBinding.Name, "AnonymousStruct_")) &&
			(strings.TrimPrefix(existingStruct.Name, "AnonymousStruct_") == strings.TrimPrefix(structBinding.Name, "AnonymousStruct_")) {

			// Deduplicate fields before updating
			deduplicatedFields := bg.deduplicateStructFields(structBinding.Fields)
			structBinding.Fields = deduplicatedFields

			bg.structs[name] = *structBinding
			bg.headerLog.WriteString(fmt.Sprintf("Updated struct %s in map with %d fields\n",
				structBinding.Name, len(structBinding.Fields)))
			return
		}
	}
	// If not found, add it (with deduplication)
	deduplicatedFields := bg.deduplicateStructFields(structBinding.Fields)
	structBinding.Fields = deduplicatedFields

	bg.structs[structBinding.Name] = *structBinding
	bg.headerLog.WriteString(fmt.Sprintf("Added struct %s to map with %d fields\n",
		structBinding.Name, len(structBinding.Fields)))
}

// deduplicateStructFields removes duplicate fields from a struct field list
func (bg *BindingGenerator) deduplicateStructFields(fields []StructField) []StructField {
	seen := make(map[string]bool)
	var deduplicated []StructField

	for _, field := range fields {
		// Create a unique key for each field based on name and type
		fieldKey := fmt.Sprintf("%s:%s", field.Name, field.Type)

		if !seen[fieldKey] {
			seen[fieldKey] = true
			deduplicated = append(deduplicated, field)
		} else {
			bg.headerLog.WriteString(fmt.Sprintf("[DEBUG] Skipping duplicate field: %s of type %s\n", field.Name, field.Type))
		}
	}

	return deduplicated
}

// handleTypedefDecl handles typedef declarations
func (bg *BindingGenerator) handleTypedefDecl(cursor clang.Cursor, depth int) {
	typedefName := cursor.Spelling()
	underlyingType := cursor.TypedefDeclUnderlyingType()
	typeSpelling := underlyingType.Spelling()

	bg.headerLog.WriteString(fmt.Sprintf("%sFound typedef: %s -> %s\n",
		strings.Repeat("  ", depth), typedefName, typeSpelling))

	// Handle function pointer typedefs
	if underlyingType.Kind() == clang.Type_Pointer {
		pointeeType := underlyingType.PointeeType()
		if pointeeType.Kind() == clang.Type_FunctionProto {
			bg.headerLog.WriteString(fmt.Sprintf("%sFound function pointer typedef: %s\n",
				strings.Repeat("  ", depth), typedefName))

			// Create function pointer type mapping
			natureType := "fn("
			var paramTypes []string

			numParams := pointeeType.NumArgTypes()
			for i := uint32(0); i < uint32(numParams); i++ {
				paramType := pointeeType.ArgType(i)
				paramTypeSpelling := paramType.Spelling()
				natureParamType := bg.mapCTypeToNature(paramTypeSpelling)
				paramTypes = append(paramTypes, natureParamType)
			}

			natureType += strings.Join(paramTypes, ", ")
			natureType += "):"

			returnType := pointeeType.ResultType()
			returnTypeSpelling := returnType.Spelling()
			natureReturnType := bg.mapCTypeToNature(returnTypeSpelling)
			natureType += natureReturnType

			bg.typeMappings[typedefName] = TypeMapping{
				CType:      typedefName,
				NatureType: natureType,
				IsPointer:  false,
			}
			return
		}
	}

	// Regular typedef
	natureType := bg.mapCTypeToNature(typeSpelling)

	// If this is a typedef for a union, map it directly to the union type name
	if strings.HasPrefix(typeSpelling, "union ") {
		unionName := strings.TrimSpace(strings.TrimPrefix(typeSpelling, "union "))
		if unionSize, exists := bg.unionNames[unionName]; exists {
			natureType = fmt.Sprintf("Union_%s_bytes", num2words.Convert(int(unionSize)))
			bg.headerLog.WriteString(fmt.Sprintf("%sMapped union typedef %s to %s\n",
				strings.Repeat("  ", depth), typedefName, natureType))
		}
	}

	bg.typeMappings[typedefName] = TypeMapping{
		CType:      typedefName,
		NatureType: natureType,
		IsPointer:  false,
	}
}

// handleFunctionDecl handles function declarations
func (bg *BindingGenerator) handleFunctionDecl(cursor clang.Cursor, depth int) {
	funcName := cursor.Spelling()
	if funcName == "" {
		return // Skip unnamed functions
	}

	bg.headerLog.WriteString(fmt.Sprintf("%sFound function: %s\n", strings.Repeat("  ", depth), funcName))

	// Get return type
	returnType := cursor.ResultType()
	returnTypeSpelling := returnType.Spelling()
	natureReturnType := bg.mapCTypeToNature(returnTypeSpelling)

	// Get parameters
	var parameters []Parameter
	numParams := int(cursor.NumArguments())
	for i := 0; i < numParams; i++ {
		param := cursor.Argument(uint32(i))
		paramName := param.Spelling()
		if paramName == "" {
			paramName = fmt.Sprintf("arg%d", i)
		}

		paramType := param.Type()
		paramTypeSpelling := paramType.Spelling()
		natureParamType := bg.mapCTypeToNature(paramTypeSpelling)

		parameters = append(parameters, Parameter{
			Name: bg.renameReservedKeywords(paramName),
			Type: natureParamType,
		})
	}

	// Check if function is variadic by examining the function type
	isVariadic := false
	var variadicType string
	if cursor.IsVariadic() {
		isVariadic = true
		bg.headerLog.WriteString(fmt.Sprintf("%sFound variadic function: %s\n", strings.Repeat("  ", depth), funcName))

		variadicArg := cursor.Argument(uint32(cursor.NumArguments()))

		bg.headerLog.WriteString(fmt.Sprintf("%sVariadic argument: name: %s, type: %s\n", strings.Repeat("  ", depth), variadicArg.Spelling(), variadicArg.Type().Spelling()))

		// just to make sure, print the arg at index 1
		arg1 := cursor.Argument(uint32(1))
		bg.headerLog.WriteString(fmt.Sprintf("%sArg 1: name: %s, type: %s\n", strings.Repeat("  ", depth), arg1.Spelling(), arg1.Type().Spelling()))

		bg.headerLog.WriteString(fmt.Sprintf("%s\n", strconv.Itoa(int(cursor.NumArguments()))))

		variadicType = bg.mapCTypeToNature(variadicArg.Type().Spelling())
	}

	if isVariadic {
		parameters = append(parameters, Parameter{
			Name: "args",
			Type: fmt.Sprintf("...[%s]", variadicType),
		})
	}

	bg.functions[funcName] = FunctionBinding{
		Name:       funcName,
		CName:      funcName,
		Parameters: parameters,
		ReturnType: natureReturnType,
	}

	bg.headerLog.WriteString(fmt.Sprintf("%sAdded function: %s\n", strings.Repeat("  ", depth), funcName))
}

// handleEnumDecl handles enum declarations
func (bg *BindingGenerator) handleEnumDecl(cursor clang.Cursor, depth int) {
	enumName := cursor.Spelling()
	bg.headerLog.WriteString(fmt.Sprintf("%sFound enum: %s\n", strings.Repeat("  ", depth), enumName))

	enumBinding := EnumBinding{
		Name:    enumName,
		Members: make([]EnumMember, 0),
	}

	// Visit enum constants
	cursor.Visit(func(cursor, parent clang.Cursor) clang.ChildVisitResult {
		if cursor.Kind() == clang.Cursor_EnumConstantDecl {
			constantName := cursor.Spelling()
			constantValue := cursor.EnumConstantDeclValue()

			enumBinding.Members = append(enumBinding.Members, EnumMember{
				Name:  constantName,
				Value: int(constantValue),
			})

			bg.headerLog.WriteString(fmt.Sprintf("%sFound enum constant: %s = %d\n",
				strings.Repeat("  ", depth), constantName, constantValue))
		}
		return clang.ChildVisit_Continue
	})

	bg.enums[enumName] = enumBinding
	bg.headerLog.WriteString(fmt.Sprintf("%sAdded enum: %s with %d members\n",
		strings.Repeat("  ", depth), enumName, len(enumBinding.Members)))
}

// handleMacroDefinition handles macro definitions
func (bg *BindingGenerator) handleMacroDefinition(cursor clang.Cursor, depth int, kind clang.Type) {
	bg.headerLog.WriteString(fmt.Sprintf("%sFound macro: %s\n", strings.Repeat("  ", depth), cursor.Spelling()))
	macroName := cursor.Spelling()

	// Get the macro value by reading the source file
	var macroValue string = "0"  // Default value
	var macroType string = "int" // Default type

	location := cursor.Location()
	if !location.IsInSystemHeader() {
		// Try to read the macro definition from the source file
		file, _, _, _ := location.FileLocation()
		if file != (clang.File{}) {
			fileName := file.Name()
			content, err := os.ReadFile(fileName)
			if err == nil {
				lines := strings.Split(string(content), "\n")
				// Look for the macro definition in the file
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "#define "+macroName+" ") {
						bg.headerLog.WriteString(fmt.Sprintf("%sFound macro definition: %s\n", strings.Repeat("  ", depth), line))
						// Extract the value after the macro name
						parts := strings.SplitN(line, " ", 3)
						if len(parts) >= 3 {
							macroValue = strings.TrimSpace(parts[2])
							break
						}
					}
				}
			}
		}
	}

	// Clean up the macro value
	macroValue = strings.TrimSpace(macroValue)
	if macroValue == "" {
		macroValue = "0"
	}

	// --- Struct-valued macro support ---
	// Try to match struct-valued macro, e.g. (Color){255,255,255,255} or Color{255,255,255,255}
	structMacroRe := regexp.MustCompile(`^(?:\((\w+)\)|(\w+))\s*\{([^}]*)\}$`)
	if matches := structMacroRe.FindStringSubmatch(macroValue); matches != nil {
		structType := matches[1]
		if structType == "" {
			structType = matches[2]
		}
		values := matches[3]
		valueList := strings.Split(values, ",")
		// Use the actual struct fields from bg.structs
		if structDef, ok := bg.structs[structType]; ok && len(structDef.Fields) == len(valueList) {
			var parts []string
			for i, v := range valueList {
				parts = append(parts, fmt.Sprintf("%s=%s", structDef.Fields[i].Name, strings.TrimSpace(v)))
			}
			macroValue = fmt.Sprintf("%s{%s}", structType, strings.Join(parts, ","))
		} else {
			// Fallback: just use positional
			macroValue = fmt.Sprintf("%s{%s}", structType, values)
		}
		macroType = structType
		bg.constants[macroName] = ConstantItem{
			Name:  macroName,
			Type:  macroType,
			Value: macroValue,
		}
		return
	}
	// --- End struct-valued macro support ---

	// Determine the type based on the value
	if strings.HasPrefix(macroValue, "\"") && strings.HasSuffix(macroValue, "\"") {
		macroType = "string"
	} else if strings.Contains(macroValue, ".") {
		macroType = "f64"
	} else {
		macroType = "i32"
	}

	bg.constants[macroName] = ConstantItem{
		Name:  macroName,
		Type:  macroType,
		Value: macroValue,
	}

	bg.headerLog.WriteString(fmt.Sprintf("%sFound macro: %s = %s (%s)\n", strings.Repeat("  ", depth), macroName, macroValue, macroType))
}

// handleIncludeDirective handles include directives
func (bg *BindingGenerator) handleIncludeDirective(cursor clang.Cursor, depth int) {
	includeFile := cursor.Spelling()
	bg.headerLog.WriteString(fmt.Sprintf("%sFound include: %s\n", strings.Repeat("  ", depth), includeFile))

	// Add to includes list
	bg.includes = append(bg.includes, includeFile)
}

// renameReservedKeywords renames reserved keywords by adding underscore suffix
func (bg *BindingGenerator) renameReservedKeywords(name string) string {
	reservedKeywords := map[string]bool{
		"type": true,
		"ptr":  true, // ptr is a type, not an argument/field name
	}

	if reservedKeywords[name] {
		return name + "_"
	}
	return name
}

// sortConstantsByDependencies sorts constants so that dependencies come first
func (bg *BindingGenerator) sortConstantsByDependencies() []ConstantItem {
	if len(bg.constants) == 0 {
		return []ConstantItem{}
	}

	// Build dependency graph
	dependencies := make(map[string][]string)
	constantMap := make(map[string]ConstantItem)

	for name, constant := range bg.constants {
		constantMap[name] = constant
		dependencies[name] = bg.extractConstantDependencies(constant.Value)
	}

	// Topological sort using iterative approach
	var result []ConstantItem
	visited := make(map[string]bool)
	processing := make(map[string]bool)

	var visit func(string) error
	visit = func(name string) error {
		if processing[name] {
			return fmt.Errorf("circular dependency detected for constant: %s", name)
		}
		if visited[name] {
			return nil
		}

		processing[name] = true
		defer func() { processing[name] = false }()

		// Visit all dependencies first
		for _, dep := range dependencies[name] {
			if constantMap[dep].Name != "" { // Only visit if it's a known constant
				if err := visit(dep); err != nil {
					return err
				}
			}
		}

		visited[name] = true
		result = append(result, constantMap[name])
		return nil
	}

	// Visit all constants
	for name := range bg.constants {
		if !visited[name] {
			if err := visit(name); err != nil {
				// If there's a circular dependency, just return in original order
				fmt.Printf("Warning: %v, using original order\n", err)
				var fallback []ConstantItem
				for _, constant := range bg.constants {
					fallback = append(fallback, constant)
				}
				return fallback
			}
		}
	}

	return result
}

// extractConstantDependencies extracts constant names from a constant value
func (bg *BindingGenerator) extractConstantDependencies(value string) []string {
	var deps []string

	// Use regex to find potential constant names
	// Look for word boundaries to avoid partial matches
	re := regexp.MustCompile(`\b([A-Z][A-Z0-9_]*)\b`)
	matches := re.FindAllStringSubmatch(value, -1)

	for _, match := range matches {
		constantName := match[1]
		// Check if this is actually a constant we know about
		if _, exists := bg.constants[constantName]; exists {
			deps = append(deps, constantName)
		}
	}

	return deps
}

// generateNatureBindings generates Nature binding code
func (bg *BindingGenerator) generateNatureBindings() string {
	var sb strings.Builder

	// Header comment
	sb.WriteString("// Generated Nature bindings\n")
	sb.WriteString("// This file was automatically generated by naturebindgen\n\n")

	// Generate constants in dependency order
	if len(bg.constants) > 0 {
		sb.WriteString("// Constants\n")
		sortedConstants := bg.sortConstantsByDependencies()
		for _, constant := range sortedConstants {
			sb.WriteString(fmt.Sprintf("%s %s = %s\n", constant.Type, constant.Name, constant.Value))
		}
		sb.WriteString("\n")
	}

	// Generate enum constants
	if len(bg.enums) > 0 {
		sb.WriteString("// Enum constants\n")
		for _, enum := range bg.enums {
			for _, member := range enum.Members {
				sb.WriteString(fmt.Sprintf("int %s_C_ENUM_%s = %d\n", enum.Name, member.Name, member.Value))
			}
		}
		sb.WriteString("\n")
	}

	// Generate type definitions (including function pointer typedefs)
	if len(bg.typeMappings) > 0 {
		sb.WriteString("// Type definitions\n")
		for cType, mapping := range bg.typeMappings {
			// Skip basic type mappings that are just direct conversions
			if cType == mapping.NatureType {
				continue
			}
			// Only output function pointer typedefs and custom types
			if strings.HasPrefix(mapping.NatureType, "fn(") {
				sb.WriteString(fmt.Sprintf("type %s = %s\n", cType, mapping.NatureType))
			}
		}
		sb.WriteString("\n")
	}

	// Generate union definitions first (before structs that reference them)
	if len(bg.unions) > 0 {
		sb.WriteString("// Union definitions\n")
		for _, union := range bg.unions {
			sb.WriteString(union.ToNature(bg))
		}
		sb.WriteString("\n")
	}

	// Generate struct definitions
	if len(bg.structs) > 0 {
		sb.WriteString("// Struct definitions\n")
		for _, structDef := range bg.structs {
			sb.WriteString(fmt.Sprintf("type %s = struct {\n", structDef.Name))
			for _, field := range structDef.Fields {
				if field.Nested != nil {
					sb.WriteString(fmt.Sprintf("    %s %s\n", field.Nested.Name, field.Name))
				} else if field.IsUnion {
					// For union fields, we need to resolve the union type name
					unionTypeName := bg.resolveUnionTypeName(field.Type)
					sb.WriteString(fmt.Sprintf("    %s %s\n", unionTypeName, field.Name))
				} else {
					sb.WriteString(fmt.Sprintf("    %s %s\n", field.Type, field.Name))
				}
			}
			sb.WriteString("}\n\n")
		}
	}

	// Generate function bindings
	if len(bg.functions) > 0 {
		sb.WriteString("// Function bindings\n")
		for _, fn := range bg.functions {
			// Generate the #linkid tag
			sb.WriteString(fmt.Sprintf("#linkid %s\n", fn.CName))

			// Generate the function signature
			sb.WriteString(fmt.Sprintf("fn %s(", fn.Name))

			// Generate parameters
			for i, param := range fn.Parameters {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(fmt.Sprintf("%s %s", param.Type, param.Name))
			}

			sb.WriteString(")")

			// Generate return type
			if fn.ReturnType != "void" {
				sb.WriteString(fmt.Sprintf(":%s", fn.ReturnType))
			}

			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// resolveUnionTypeName resolves a string type to its actual union type name
func (bg *BindingGenerator) resolveUnionTypeName(typeName string) string {
	// Check if this is a known union type
	if unionSize, exists := bg.unionNames[typeName]; exists {
		// Return the union type name based on size
		return fmt.Sprintf("Union_%s_bytes", num2words.Convert(int(unionSize)))
	}

	// If not found, try to find by size in the unions map
	// This handles cases where the union name might not be in unionNames
	for size, union := range bg.unions {
		if union.Name == typeName {
			return fmt.Sprintf("Union_%s_bytes", num2words.Convert(int(size)))
		}
	}

	// If still not found, return the original type name
	return typeName
}

// printHeaderLog prints the header parsing log
func (bg *BindingGenerator) printHeaderLog() {
	fmt.Println("\n=== Header Parsing Log ===")
	fmt.Print(bg.headerLog.String())
	fmt.Println("=== End Header Log ===")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: naturebindgen <header-file> [options]")
		fmt.Println("Options:")
		fmt.Println("  -o, --output <file>     Output file (default: bindings.n)")
		fmt.Println("  -h, --help             Show this help message")
		os.Exit(1)
	}

	headerFile := os.Args[1]
	outputFile := "bindings.n"

	// Parse command line arguments
	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "-o", "--output":
			if i+1 < len(os.Args) {
				outputFile = os.Args[i+1]
				i++
			}
		case "-h", "--help":
			fmt.Println("naturebindgen - Generate Nature bindings from C headers")
			fmt.Println("Usage: naturebindgen <header-file> [options]")
			os.Exit(0)
		}
	}

	// Create binding generator
	bg := NewBindingGenerator()

	// Parse header file
	fmt.Printf("Parsing header file: %s\n", headerFile)
	if err := bg.parseHeaderFile(headerFile); err != nil {
		fmt.Printf("Error parsing header file: %v\n", err)
		os.Exit(1)
	}

	// Print debug information
	fmt.Println("\n=== DEBUG: Structs parsed ===")
	for name := range bg.structs {
		fmt.Println("struct:", name)
	}
	fmt.Println("=== DEBUG: Functions parsed ===")
	for name := range bg.functions {
		fmt.Println("function:", name)
	}
	fmt.Println("============================")

	// Generate bindings
	bindings := bg.generateNatureBindings()

	// Print the header parsing log
	bg.printHeaderLog()

	// Write bindings to file
	if err := os.WriteFile(outputFile, []byte(bindings), 0644); err != nil {
		fmt.Printf("Error writing bindings file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated bindings: %s\n", outputFile)
	fmt.Printf("Functions: %d\n", len(bg.functions))
	fmt.Printf("Structs: %d\n", len(bg.structs))
	fmt.Printf("Constants: %d\n", len(bg.constants))
	fmt.Printf("Const int declarations: %d\n", len(bg.constantValues))
	fmt.Printf("Enums: %d\n", len(bg.enums))
}
