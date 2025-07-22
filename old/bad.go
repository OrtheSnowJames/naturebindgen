package old

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// BindingGenerator generates Nature bindings from C headers
type BindingGenerator struct {
	typeMappings   map[string]TypeMapping
	functions      map[string]FunctionBinding // Changed from slice to map to prevent duplicates
	structs        map[string]StructBinding   // Changed from slice to map to prevent duplicates
	constants      map[string]string
	includes       []string
	enums          map[string]EnumBinding // Changed from slice to map to prevent duplicates
	constantValues map[string]int
	includedFiles  map[string]bool // Track which files have been included
	baseDir        string          // Base directory for relative includes
	headerLog      strings.Builder // Buffer to collect header parsing information
	braceDepth     int             // Track brace depth for nested structures
	pendingStruct  *StructBinding  // Store anonymous struct for later renaming
	currentNested  *StructBinding  // Track current nested struct being parsed
	nestedDepth    int             // Track depth of nested structures
}

// NewBindingGenerator creates a new binding generator
func NewBindingGenerator() *BindingGenerator {
	bg := &BindingGenerator{
		typeMappings:   make(map[string]TypeMapping),
		functions:      make(map[string]FunctionBinding),
		structs:        make(map[string]StructBinding),
		constants:      make(map[string]string),
		includes:       make([]string, 0),
		enums:          make(map[string]EnumBinding),
		constantValues: make(map[string]int),
		includedFiles:  make(map[string]bool),
		baseDir:        "",
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
		{"SDL_WindowID", "u32", false},
		{"SDL_DisplayID", "u32", false},
		{"SDL_PropertiesID", "u32", false},
		{"SDL_PixelFormat", "u32", false},
		{"SDL_SystemTheme", "int", false},
		{"SDL_DisplayOrientation", "int", false},
		{"SDL_WindowFlags", "u64", false},
		{"SDL_FlashOperation", "int", false},
		{"SDL_GLAttr", "int", false},
		{"SDL_HitTestResult", "int", false},
		// Pointer types
		{"void*", "anyptr", true},
		{"char*", "anyptr", true},
		{"const char*", "anyptr", true},
		// SDL opaque pointer types
		{"SDL_Window*", "rawptr<SDL_Window>", true},
		{"SDL_GLContext", "rawptr<SDL_GLContext>", true},
		{"SDL_Surface*", "rawptr<SDL_Surface>", true},
		{"SDL_DisplayMode*", "rawptr<SDL_DisplayMode>", true},
		{"SDL_Rect*", "rawptr<SDL_Rect>", true},
		{"SDL_Point*", "rawptr<SDL_Point>", true},
		{"SDL_Environment*", "rawptr<SDL_Environment>", true},
		{"SDL_iconv_t", "rawptr<SDL_iconv_data_t>", true},
	}

	for _, mapping := range mappings {
		bg.typeMappings[mapping.CType] = mapping
	}
}

// mapCTypeToNature converts a C type to its Nature equivalent
func (bg *BindingGenerator) mapCTypeToNature(cType string) string {
	// Clean up the type string
	cType = strings.TrimSpace(cType)
	cType = regexp.MustCompile(`\s+`).ReplaceAllString(cType, " ")

	// Handle SDL-specific type qualifiers
	cType = strings.ReplaceAll(cType, "SDL_DECLSPEC", "")
	cType = strings.ReplaceAll(cType, "SDL_MALLOC", "")
	cType = strings.ReplaceAll(cType, "SDL_ALLOC_SIZE(2)", "")
	cType = strings.TrimSpace(cType)

	// Handle function pointer types like "int (SDLCALL *SDL_CompareCallback)(const void *a, const void *b)"
	if strings.Contains(cType, "(*") && strings.Contains(cType, ")(") {
		// Extract the return type and function name
		parts := strings.Split(cType, "(*")
		if len(parts) >= 2 {
			rest := parts[1]
			funcNameEnd := strings.Index(rest, ")(")
			if funcNameEnd != -1 {
				// For now, map function pointers to anyptr
				return "anyptr"
			}
		}
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

		// Check if it's a pointer to an SDL opaque type
		if strings.HasPrefix(baseType, "SDL_") {
			// Remove "struct " prefix if present
			cleanType := strings.TrimPrefix(baseType, "struct ")
			return fmt.Sprintf("rawptr<%s>", cleanType)
		}

		// Default pointer types
		if baseType == "void" || baseType == "char" || baseType == "const char" || baseType == "const void" {
			return "anyptr"
		}
		return "anyptr"
	}

	// Handle array types - both "type[]" and "type[size]"
	if strings.Contains(cType, "[") {
		// Extract base type and size
		parts := strings.Split(cType, "[")
		baseType := strings.TrimSpace(parts[0])
		sizeStr := strings.TrimRight(parts[1], "]")

		natureBaseType := bg.mapCTypeToNature(baseType)
		if sizeStr == "" {
			return fmt.Sprintf("[%s]", natureBaseType)
		}
		// For fixed-size arrays, use the Nature array syntax
		return fmt.Sprintf("[%s;%s]", natureBaseType, sizeStr)
	}

	// Handle "struct" keyword
	if strings.HasPrefix(cType, "struct ") {
		structName := strings.TrimSpace(strings.TrimPrefix(cType, "struct "))
		return structName
	}

	// Direct mapping
	if mapping, exists := bg.typeMappings[cType]; exists {
		return mapping.NatureType
	}

	// Try to map common C types that might not be in our mapping
	switch cType {
	case "float":
		return "f32"
	case "double":
		return "f64"
	case "long double":
		return "f64"
	case "unsigned char":
		return "u8"
	case "signed char":
		return "i8"
	case "unsigned short":
		return "u16"
	case "signed short":
		return "i16"
	case "unsigned int":
		return "u32"
	case "signed int":
		return "i32"
	case "unsigned long":
		return "u64"
	case "signed long":
		return "i64"
	case "unsigned long long":
		return "u64"
	case "signed long long":
		return "i64"
	case "size_t":
		return "uint"
	case "ssize_t":
		return "int"
	case "ptrdiff_t":
		return "int"
	case "uintptr_t":
		return "anyptr"
	case "intptr_t":
		return "anyptr"
	case "int8_t":
		return "i8"
	case "uint8_t":
		return "u8"
	case "int16_t":
		return "i16"
	case "uint16_t":
		return "u16"
	case "int32_t":
		return "i32"
	case "uint32_t":
		return "u32"
	case "int64_t":
		return "i64"
	case "uint64_t":
		return "u64"
	case "bool":
		return "bool"
	case "_Bool":
		return "bool"
	}

	// Check if it's a known struct type
	for _, structDef := range bg.structs {
		if cType == structDef.Name {
			return structDef.Name
		}
	}

	// Check if it's a known enum type
	for _, enumDef := range bg.enums {
		if cType == enumDef.Name {
			return "int"
		}
	}

	// Check if it's an SDL type that should be mapped
	if strings.HasPrefix(cType, "SDL_") {
		// For SDL types we haven't explicitly mapped, try to infer
		if strings.Contains(strings.ToLower(cType), "id") {
			return "u32" // Most SDL IDs are 32-bit
		}
		if strings.Contains(strings.ToLower(cType), "flags") {
			return "u64" // Most SDL flags are 64-bit
		}
		return "int" // Default for SDL enums
	}

	// Default to anyptr for truly unknown types
	return "anyptr"
}

// shouldIncludeHeader determines if a header should be included based on its content
func (bg *BindingGenerator) shouldIncludeHeader(filename string) bool {
	// System headers that we don't want to include
	systemHeaders := map[string]bool{
		"stdio.h":  true,
		"stdlib.h": true,
		"string.h": true,
		"stdint.h": true,
		"stddef.h": true,
		"limits.h": true,
		"float.h":  true,
		"assert.h": true,
		"ctype.h":  true,
		"errno.h":  true,
		"signal.h": true,
		"time.h":   true,
		"locale.h": true,
		"setjmp.h": true,
		"math.h":   true,
		"wchar.h":  true,
		"wctype.h": true,
	}

	// Check if it's a system header
	if systemHeaders[filename] {
		return false
	}

	// Generic approach: allow any header that can be found in the same directory
	// or nearby directories relative to the current header being parsed
	// This will be checked during the actual include resolution

	// Check if it's already been included
	if bg.includedFiles[filename] {
		return false
	}

	return true
}

// parseHeaderFileRecursive parses a C header file and its includes recursively
func (bg *BindingGenerator) parseHeaderFileRecursive(filename string) error {
	// Mark this file as included
	bg.includedFiles[filename] = true

	// Log which header is being parsed
	bg.headerLog.WriteString(fmt.Sprintf("Parsing header: %s\n", filename))

	// Determine the base directory for relative includes
	if bg.baseDir == "" {
		bg.baseDir = filepath.Dir(filename)
	}

	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", filename, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Regular expressions for parsing - improved patterns
	// Match both prototypes (;) and inline/defined functions ({)
	// Updated to handle SDL calling conventions and complex signatures
	// More robust function regex that captures everything before the function name
	// Also handle function pointer declarations

	// This pattern matches function pointers with complex return types
	funcPtrRegex := regexp.MustCompile(`^\s*(?:extern\s+)?(?:inline\s+)?(?:__attribute__\(\([^)]*\)\)\s*)?(.*?)\s*\(\*([a-zA-Z_][a-zA-Z0-9_]*)\)\s*\(([^)]*)\)\s*[;{]`)
	structRegex := regexp.MustCompile(`^\s*struct\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\{`)
	typedefStructRegex := regexp.MustCompile(`^\s*typedef\s+struct\s*(?:([a-zA-Z_][a-zA-Z0-9_]*)\s*)?\{`)
	includeRegex := regexp.MustCompile(`^\s*#include\s+[<"]([^>"]+)[>"]`)
	pragmaOnceRegex := regexp.MustCompile(`^\s*#pragma\s+once`)
	constIntRegex := regexp.MustCompile(`^\s*const\s+int\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*([0-9]+)\s*;`)
	// SDL-specific patterns
	sdlMacroRegex := regexp.MustCompile(`^\s*#define\s+([A-Za-z_][A-Za-z0-9_]*)\s+(.+)$`)
	sdlConstRegex := regexp.MustCompile(`^\s*#define\s+([A-Za-z_][A-Za-z0-9_]*)\s+SDL_UINT64_C\(([0-9]+)\)`)
	// SDL opaque type typedefs
	sdlOpaqueTypedefRegex := regexp.MustCompile(`^\s*typedef\s+struct\s+([a-zA-Z_][a-zA-Z0-9_]*)\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*;`)
	// Regex for typedef function pointers like "typedef void (*CursedCallback)(int, void*);"
	typedefFuncPtrRegex := regexp.MustCompile(`^\s*typedef\s+(.*?)\s*\(\*([a-zA-Z_][a-zA-Z0-9_]*)\)\s*\(([^)]*)\)\s*;`)
	// Regex for complex function pointer returns like "void* (*get_cursed_handler(const char* tag))(int);"
	complexFuncPtrRegex := regexp.MustCompile(`^\s*(?:extern\s+)?(?:inline\s+)?(?:__attribute__\(\([^)]*\)\)\s*)?(.*?)\s*\(\*([a-zA-Z_][a-zA-Z0-9_]*)\s*\(([^)]*)\)\)\s*\(([^)]*)\)\s*[;{]`)
	// Regex for end of typedef struct: "} CursedResult;" or "} CursedResult;" on separate lines
	// This should match the outermost closing brace with a name
	typedefStructEndRegex := regexp.MustCompile(`^\s*}\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*;$`)
	// Also handle the case where the name is on the next line: "}" followed by "CursedResult;"
	typedefStructEndNextLineRegex := regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*;$`)

	inStruct := false
	currentStruct := StructBinding{}

	enumStartRegex := regexp.MustCompile(`^typedef\s+enum\s*\{`)
	enumEndRegex := regexp.MustCompile(`^}\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*;`)
	inEnum := false
	currentEnum := EnumBinding{}
	currentEnumValue := 0

	// In parseHeaderFileRecursive, add a unionFields slice to collect union members
	var unionFields []StructField

	// Add a map to track nested struct counts per parent
	nestedStructCounters := make(map[string]int)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") {
			continue
		}

		// Remove inline comments (everything after //)
		if commentIndex := strings.Index(line, "//"); commentIndex != -1 {
			line = strings.TrimSpace(line[:commentIndex])
		}

		// Skip pragma once - we handle deduplication ourselves
		if pragmaOnceRegex.MatchString(line) {
			continue
		}

		// Parse includes and handle them recursively
		if matches := includeRegex.FindStringSubmatch(line); matches != nil {
			includeFile := matches[1]

			// Check if we should include this header
			if bg.shouldIncludeHeader(includeFile) {
				// Try to find the include file using a generic approach
				includePath := includeFile
				if !filepath.IsAbs(includeFile) {
					// Try relative to current file first
					includePath = filepath.Join(bg.baseDir, includeFile)
					if _, err := os.Stat(includePath); os.IsNotExist(err) {
						// Try system include paths
						systemPaths := []string{"/usr/include", "/usr/local/include"}
						for _, sysPath := range systemPaths {
							testPath := filepath.Join(sysPath, includeFile)
							if _, err := os.Stat(testPath); err == nil {
								includePath = testPath
								break
							}
						}

						// If still not found, try to find it in the same directory structure
						// This handles cases like "SDL3/SDL_render.h" -> "/usr/include/SDL3/SDL_render.h"
						if strings.Contains(includeFile, "/") {
							// Split the include path to get the directory and filename
							parts := strings.Split(includeFile, "/")
							if len(parts) > 1 {
								// Try to find the directory in system paths
								dirName := parts[0]
								fileName := strings.Join(parts[1:], "/")
								for _, sysPath := range systemPaths {
									dirPath := filepath.Join(sysPath, dirName)
									if _, err := os.Stat(dirPath); err == nil {
										// Directory exists, check if the file exists
										testPath := filepath.Join(dirPath, fileName)
										if _, err := os.Stat(testPath); err == nil {
											includePath = testPath
											break
										}
									}
								}
							}
						}
					}
				}

				// Parse the included file recursively
				bg.headerLog.WriteString(fmt.Sprintf("  Including: %s -> %s\n", includeFile, includePath))
				if err := bg.parseHeaderFileRecursive(includePath); err != nil {
					// Log warning but continue - some includes might not be found
					fmt.Printf("Warning: Could not include %s: %v\n", includeFile, err)
				}
			}
			continue
		}

		// Parse const int declarations
		if matches := constIntRegex.FindStringSubmatch(line); matches != nil {
			constantName := matches[1]
			var value int
			fmt.Sscanf(matches[2], "%d", &value)
			bg.constantValues[constantName] = value
			continue
		}

		// Parse SDL macros and constants
		if matches := sdlMacroRegex.FindStringSubmatch(line); matches != nil {
			macroName := matches[1]
			macroValue := matches[2]

			// Check if this is a function alias macro like "#define cursed_alias do_callback"
			if !strings.Contains(macroValue, "(") && !strings.Contains(macroValue, " ") {
				// This looks like a simple function alias
				bg.headerLog.WriteString(fmt.Sprintf("    Found function alias macro: %s = %s\n", macroName, macroValue))
				// Store it as a constant for now, we'll handle it later
				bg.constants[macroName] = macroValue
			} else {
				// Regular macro
				bg.constants[macroName] = macroValue
			}
			continue
		}
		if matches := sdlConstRegex.FindStringSubmatch(line); matches != nil {
			constantName := matches[1]
			var value int
			fmt.Sscanf(matches[2], "%d", &value)
			bg.constantValues[constantName] = value
			continue
		}

		// Parse SDL opaque type typedefs
		if matches := sdlOpaqueTypedefRegex.FindStringSubmatch(line); matches != nil {
			typedefName := matches[1]
			natureType := bg.mapCTypeToNature(typedefName)
			bg.typeMappings[typedefName] = TypeMapping{CType: typedefName, NatureType: natureType, IsPointer: false}
			continue
		}

		// Parse enum definitions before struct definitions
		if enumStartRegex.MatchString(line) {
			inEnum = true
			currentEnum = EnumBinding{Members: make([]EnumMember, 0)}
			currentEnumValue = 0
			bg.headerLog.WriteString(fmt.Sprintf("    Starting enum\n"))
			continue
		}

		if inEnum {
			if enumEndRegex.MatchString(line) {
				matches := enumEndRegex.FindStringSubmatch(line)
				currentEnum.Name = matches[1]
				bg.enums[currentEnum.Name] = currentEnum
				inEnum = false
				bg.headerLog.WriteString(fmt.Sprintf("    Completed enum: %s with %d members\n", currentEnum.Name, len(currentEnum.Members)))
				continue
			}
			// Parse enum members
			if line == "" || strings.HasPrefix(line, "//") {
				continue
			}
			// Handle member with explicit value: NAME = value,
			if strings.Contains(line, "=") {
				parts := strings.Split(line, "=")
				name := strings.TrimSpace(parts[0])
				valuePart := strings.TrimRight(strings.TrimSpace(parts[1]), ",")
				currentEnum.Members = append(currentEnum.Members, EnumMember{Name: name, Value: currentEnumValue, Literal: valuePart})
				// Try to parse as a literal number to update currentEnumValue
				var value int
				if _, err := fmt.Sscanf(valuePart, "%d", &value); err == nil {
					currentEnumValue = value + 1
				} else {
					currentEnumValue++
				}
			} else {
				// Member without explicit value
				name := strings.TrimRight(line, ",")
				currentEnum.Members = append(currentEnum.Members, EnumMember{Name: name, Value: currentEnumValue, Literal: ""})
				currentEnumValue++
			}
			continue
		}

		// Parse struct definitions - handle both "struct Name {" and "typedef struct {"
		if matches := structRegex.FindStringSubmatch(line); matches != nil {
			inStruct = true
			bg.braceDepth = 1 // Start with depth 1 since we found the opening brace
			currentStruct = StructBinding{
				Name:   matches[1],
				Fields: make([]StructField, 0),
			}
			bg.headerLog.WriteString(fmt.Sprintf("    Starting struct: %s\n", currentStruct.Name))
			continue
		}

		// Handle typedef struct without name: "typedef struct {"
		if typedefStructRegex.MatchString(line) {
			inStruct = true
			bg.braceDepth = 1 // Start with depth 1 since we found the opening brace
			// Extract the struct name from the typedef
			matches := typedefStructRegex.FindStringSubmatch(line)
			structName := matches[1]
			if structName == "" {
				// Anonymous struct, we'll need to extract the name from the typedef line
				// Look for the pattern: typedef struct { ... } Name;
				// For now, we'll create a temporary name and extract the real name later
				bg.headerLog.WriteString("    Starting anonymous typedef struct\n")
				currentStruct = StructBinding{
					Name:   "_anonymous_struct", // Temporary name
					Fields: make([]StructField, 0),
				}
			} else {
				currentStruct = StructBinding{
					Name:   structName,
					Fields: make([]StructField, 0),
				}
			}
			bg.headerLog.WriteString(fmt.Sprintf("    Starting typedef struct: %s\n", currentStruct.Name))
			continue
		}

		// Handle forward declarations: "typedef struct CursedStruct CursedStruct;"
		if strings.Contains(line, "typedef struct") && strings.Contains(line, ";") && !strings.Contains(line, "{") {
			// Extract the struct name
			parts := strings.Fields(line)
			if len(parts) >= 4 && parts[0] == "typedef" && parts[1] == "struct" {
				structName := parts[2]
				bg.headerLog.WriteString(fmt.Sprintf("    Found forward declaration: %s\n", structName))
				// Create an empty struct for forward declarations
				bg.structs[structName] = StructBinding{
					Name:   structName,
					Fields: make([]StructField, 0),
				}
			}
			continue
		}

		// Handle typedef function pointers like "typedef void (*CursedCallback)(int, void*);"
		if matches := typedefFuncPtrRegex.FindStringSubmatch(line); matches != nil {
			returnType := strings.TrimSpace(matches[1])
			funcPtrName := matches[2]
			paramStr := matches[3]

			bg.headerLog.WriteString(fmt.Sprintf("    Found typedef function pointer: %s\n", funcPtrName))

			// Parse the parameters
			params := bg.parseParameters(paramStr)

			// Create a function pointer type mapping instead of a function binding
			var paramTypes []string
			for _, param := range params {
				paramTypes = append(paramTypes, param.Type)
			}
			natureReturnType := bg.mapCTypeToNature(returnType)
			natureType := fmt.Sprintf("fn(%s):%s", strings.Join(paramTypes, ", "), natureReturnType)

			bg.typeMappings[funcPtrName] = TypeMapping{
				CType:      funcPtrName,
				NatureType: natureType,
				IsPointer:  false,
			}
			continue
		}

		// Handle complex function pointer returns like "void* (*get_cursed_handler(const char* tag))(int);"
		if matches := complexFuncPtrRegex.FindStringSubmatch(line); matches != nil {
			funcName := matches[2]
			funcParams := matches[3]

			bg.headerLog.WriteString(fmt.Sprintf("    Found complex function pointer: %s\n", funcName))

			// Parse the function parameters
			params := bg.parseParameters(funcParams)

			// For complex function pointers, we'll map them to anyptr for now
			// The return type is a function pointer, so we use anyptr
			binding := FunctionBinding{
				Name:       funcName,
				CName:      funcName,
				Parameters: params,
				ReturnType: "anyptr", // Function pointer return type
			}

			bg.functions[funcName] = binding
			continue
		}

		if inStruct {
			// Count braces to handle nested structures
			openBraces := strings.Count(line, "{")
			closeBraces := strings.Count(line, "}")

			// Update brace depth
			bg.braceDepth += openBraces - closeBraces

			// Track brace depth
			if openBraces > 0 {
				bg.headerLog.WriteString(fmt.Sprintf("    Struct %s: Found %d open braces, depth now %d\n", currentStruct.Name, openBraces, bg.braceDepth))
			}
			if closeBraces > 0 {
				bg.headerLog.WriteString(fmt.Sprintf("    Struct %s: Found %d close braces, depth now %d\n", currentStruct.Name, closeBraces, bg.braceDepth))
			}

			// Check if we've reached the end of the struct (brace depth back to 0 or 1)
			if bg.braceDepth <= 1 {
				// End of struct found
				bg.headerLog.WriteString(fmt.Sprintf("    Struct parsing ended at brace depth %d, line: %s\n", bg.braceDepth, strings.TrimSpace(line)))
				if currentStruct.Name != "" && currentStruct.Name != "_anonymous_struct" {
					// Only complete non-anonymous structs immediately
					bg.structs[currentStruct.Name] = currentStruct
					bg.headerLog.WriteString(fmt.Sprintf("    Completed struct: %s with %d fields\n", currentStruct.Name, len(currentStruct.Fields)))
				} else if currentStruct.Name == "_anonymous_struct" {
					// Store the anonymous struct for later renaming
					bg.pendingStruct = &currentStruct
					bg.headerLog.WriteString(fmt.Sprintf("    Completed anonymous struct with %d fields, waiting for typedef end\n", len(currentStruct.Fields)))
					// If the current line is a typedef struct end, immediately rename and store
					if matches := typedefStructEndRegex.FindStringSubmatch(line); matches != nil {
						realStructName := matches[1]
						bg.pendingStruct.Name = realStructName
						bg.structs[realStructName] = *bg.pendingStruct
						bg.headerLog.WriteString(fmt.Sprintf("    Immediately renamed pending _anonymous_struct to: %s\n", realStructName))
						bg.renameNestedStructs("_anonymous_struct", realStructName, &currentStruct.Fields)
						bg.pendingStruct = nil
						bg.headerLog.WriteString(fmt.Sprintf("    [DEBUG] Added struct: %s (typedef end)\n", realStructName))
					}
				}
				inStruct = false
				bg.braceDepth = 0 // Reset brace depth

				// Check if the next line is a typedef end
				// We'll handle this in the next iteration
				continue
			}

			// Check for typedef struct end: "} CursedResult;"
			// Only check when we're at brace depth 0 (after all braces are closed)
			if bg.braceDepth == 0 {
				if matches := typedefStructEndRegex.FindStringSubmatch(line); matches != nil {
					realStructName := matches[1]
					bg.headerLog.WriteString(fmt.Sprintf("    Found typedef struct end at brace depth 0: %s\n", realStructName))

					// If we have an anonymous struct, rename it to the real name
					if currentStruct.Name == "_anonymous_struct" {
						currentStruct.Name = realStructName
						bg.headerLog.WriteString(fmt.Sprintf("    Renamed anonymous struct to: %s\n", realStructName))

						// Update any nested struct names that reference the old name
						bg.renameNestedStructs("_anonymous_struct", realStructName, &currentStruct.Fields)
					}

					// Save the struct
					if currentStruct.Name != "" {
						bg.structs[currentStruct.Name] = currentStruct
						bg.headerLog.WriteString(fmt.Sprintf("    [DEBUG] Added struct: %s (struct complete)\n", currentStruct.Name))
						bg.headerLog.WriteString(fmt.Sprintf("    Completed typedef struct: %s with %d fields\n", currentStruct.Name, len(currentStruct.Fields)))
					}
					inStruct = false
					bg.braceDepth = 0 // Reset brace depth
					continue
				}
			}

			// Check for typedef struct end: "} CursedResult;" - only at brace depth 0 (main struct level)
			bg.headerLog.WriteString(fmt.Sprintf("    [DEBUG] Checking line for typedef struct end: '%s' (brace depth: %d)\n", line, bg.braceDepth))
			if bg.braceDepth == 0 {
				if matches := typedefStructEndRegex.FindStringSubmatch(line); matches != nil {
					realStructName := matches[1]
					bg.headerLog.WriteString(fmt.Sprintf("    Found typedef struct end: %s\n", realStructName))

					// Check if this is actually an enum end, not a struct end
					if strings.Contains(realStructName, "Status") || strings.Contains(realStructName, "ENUM") {
						bg.headerLog.WriteString(fmt.Sprintf("    Skipping enum typedef end: %s\n", realStructName))
					} else {
						// If we have an anonymous struct, rename it to the real name
						if currentStruct.Name == "_anonymous_struct" {
							currentStruct.Name = realStructName
							bg.headerLog.WriteString(fmt.Sprintf("    Renamed anonymous struct to: %s\n", realStructName))

							// Update any nested struct names that reference the old name
							bg.renameNestedStructs("_anonymous_struct", realStructName, &currentStruct.Fields)
						}

						// Save the struct
						if currentStruct.Name != "" {
							bg.structs[currentStruct.Name] = currentStruct
							bg.headerLog.WriteString(fmt.Sprintf("    [DEBUG] Added struct: %s (typedef end)\n", currentStruct.Name))
							bg.headerLog.WriteString(fmt.Sprintf("    Completed typedef struct: %s with %d fields\n", currentStruct.Name, len(currentStruct.Fields)))
						}
					}
					inStruct = false
					bg.braceDepth = 0 // Reset brace depth
					continue
				}
			}

			// Additional safety checks to detect when we're no longer in a struct
			trimmedLine := strings.TrimSpace(line)

			// Check for function declarations (lines with parentheses but no semicolon)
			// But skip __attribute__ lines as they're not function declarations
			// Only exit if we're at brace depth 1 (outside of nested structures)
			if bg.braceDepth <= 1 && strings.Contains(line, "(") && strings.Contains(line, ")") && !strings.HasSuffix(trimmedLine, ";") && !strings.Contains(line, "__attribute__") {
				// This looks like a function declaration, not a struct field
				bg.headerLog.WriteString(fmt.Sprintf("    Ending struct %s early due to function declaration: %s\n", currentStruct.Name, line))
				if currentStruct.Name != "" && currentStruct.Name != "_anonymous_struct" {
					bg.structs[currentStruct.Name] = currentStruct
				} else if currentStruct.Name == "_anonymous_struct" {
					bg.pendingStruct = &currentStruct
				}
				inStruct = false
				// Don't continue - let the function parsing handle this line
			}

			// Check for other struct/typedef declarations (but not nested ones)
			if strings.HasPrefix(trimmedLine, "struct ") || strings.HasPrefix(trimmedLine, "typedef ") {
				// Only exit if this is not a nested struct (i.e., if we're at brace depth 1)
				if bg.braceDepth <= 1 {
					bg.headerLog.WriteString(fmt.Sprintf("    Ending struct %s early due to new struct declaration: %s\n", currentStruct.Name, line))
					if currentStruct.Name != "" {
						bg.structs[currentStruct.Name] = currentStruct
					}
					inStruct = false
					// Don't continue - let the struct parsing handle this line
				} else {
					bg.headerLog.WriteString(fmt.Sprintf("    Found nested struct/typedef in %s: %s\n", currentStruct.Name, line))
					// Continue parsing as part of the current struct
				}
			}

			// Check for enum declarations
			if strings.HasPrefix(trimmedLine, "enum ") || strings.Contains(trimmedLine, "typedef enum") {
				bg.headerLog.WriteString(fmt.Sprintf("    Ending struct %s early due to enum declaration: %s\n", currentStruct.Name, line))
				if currentStruct.Name != "" && currentStruct.Name != "_anonymous_struct" {
					bg.structs[currentStruct.Name] = currentStruct
				} else if currentStruct.Name == "_anonymous_struct" {
					bg.pendingStruct = &currentStruct
				}
				inStruct = false
				// Don't continue - let the enum parsing handle this line
			}

			// Check for nested struct/union declarations
			trimmedLine = strings.TrimSpace(line)

			// Handle nested struct/union declarations that span multiple lines
			if strings.HasPrefix(trimmedLine, "struct ") && strings.Contains(trimmedLine, "{") {
				// If we're inside a union, treat this as a union member
				if bg.currentNested != nil && bg.currentNested.Name == "" {
					// This is a nested struct inside a union - parse it as a union member
					nestedStruct := &StructBinding{
						Name:   "",
						Fields: make([]StructField, 0),
					}
					// Collect lines until closing brace
					var nestedLines []string
					var nestedFieldName string
					for scanner.Scan() {
						l := scanner.Text()
						if strings.Contains(l, "}") {
							// Get the field name
							parts := strings.Split(l, "}")
							if len(parts) >= 2 {
								fieldPart := strings.TrimSpace(parts[len(parts)-1])
								fieldPart = strings.TrimSuffix(fieldPart, ";")
								nestedFieldName = strings.TrimSpace(fieldPart)
							}
							break
						}
						nestedLines = append(nestedLines, l)
					}
					// Parse fields from nestedLines
					for _, nline := range nestedLines {
						words := strings.Fields(strings.TrimSpace(strings.TrimSuffix(nline, ";")))
						if len(words) < 2 {
							continue
						}
						fieldName := words[len(words)-1]
						fieldType := strings.Join(words[:len(words)-1], " ")
						natureType := bg.mapCTypeToNature(fieldType)
						nestedStruct.Fields = append(nestedStruct.Fields, StructField{
							Name: bg.renameReservedKeywords(fieldName),
							Type: natureType,
						})
					}
					// Create a proper type name for the nested struct using counter-based naming
					parentName := currentStruct.Name
					nestedStructCounters[parentName]++
					nestedTypeName := fmt.Sprintf("%s_nested_%d", parentName, nestedStructCounters[parentName])
					nestedStruct.Name = nestedTypeName
					// Store the nested struct as a separate type
					bg.structs[nestedTypeName] = *nestedStruct
					// Use the new type name for the field
					unionFields = append(unionFields, StructField{
						Name:   nestedFieldName,
						Type:   nestedTypeName,
						Nested: nestedStruct,
					})
					bg.currentNested.Fields = unionFields
					bg.headerLog.WriteString(fmt.Sprintf("    Added nested struct %s to union\n", nestedFieldName))
					continue
				} else {
					// This is a regular nested struct
					bg.nestedDepth++
					bg.currentNested = &StructBinding{
						Name:   fmt.Sprintf("%s_nested_%d", currentStruct.Name, bg.nestedDepth),
						Fields: make([]StructField, 0),
					}
					bg.headerLog.WriteString(fmt.Sprintf("    Starting nested struct in %s\n", currentStruct.Name))
					continue
				}
			}

			if strings.HasPrefix(trimmedLine, "union ") && strings.Contains(trimmedLine, "{") {
				bg.nestedDepth++
				bg.currentNested = &StructBinding{
					Name:   "",
					Fields: make([]StructField, 0),
				}
				unionFields = make([]StructField, 0)
				bg.headerLog.WriteString(fmt.Sprintf("    Starting nested union in %s\n", currentStruct.Name))
				continue
			}

			// Handle end of nested struct/union
			if bg.currentNested != nil && strings.Contains(trimmedLine, "}") && strings.Contains(trimmedLine, ";") {
				// Extract field name from the end of the nested structure
				parts := strings.Split(trimmedLine, "}")
				if len(parts) >= 2 {
					fieldPart := strings.TrimSpace(parts[len(parts)-1])
					fieldPart = strings.TrimSuffix(fieldPart, ";")
					fieldName := strings.TrimSpace(fieldPart)

					if fieldName != "" {
						// Remove any previous field with the same name (e.g., 'data')
						for i := len(currentStruct.Fields) - 1; i >= 0; i-- {
							if currentStruct.Fields[i].Name == fieldName {
								currentStruct.Fields = append(currentStruct.Fields[:i], currentStruct.Fields[i+1:]...)
							}
						}

						// For unions, add all union members as separate fields since Nature doesn't have unions
						if bg.currentNested.Name == "" && len(unionFields) > 0 {
							// This is a union - add all union members as separate fields
							for _, ufield := range unionFields {
								currentStruct.Fields = append(currentStruct.Fields, ufield)
							}
							bg.headerLog.WriteString(fmt.Sprintf("    Added %d union members as separate fields in %s\n", len(unionFields), currentStruct.Name))
						} else if bg.currentNested.Name != "" {
							// This is a nested struct - add it as a single field
							currentStruct.Fields = append(currentStruct.Fields, StructField{
								Name:   bg.renameReservedKeywords(fieldName),
								Type:   bg.currentNested.Name,
								Nested: bg.currentNested,
							})
							bg.headerLog.WriteString(fmt.Sprintf("    Completed nested struct field: %s in %s\n", fieldName, currentStruct.Name))
						}
					}
				}
				bg.currentNested = nil
				bg.nestedDepth--
				unionFields = nil
				continue
			}

			// Skip empty lines, comments, and lines that don't end with semicolon
			if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") {
				continue
			}

			// Only process lines that end with semicolon (field declarations)
			if !strings.HasSuffix(strings.TrimSpace(line), ";") {
				continue
			}

			// Remove semicolon and trim
			fieldLine := strings.TrimSpace(strings.TrimSuffix(line, ";"))
			if fieldLine == "" {
				continue
			}

			// Simple field parsing - look for the last word as field name
			words := strings.Fields(fieldLine)
			if len(words) < 2 {
				continue // Skip lines with less than 2 words
			}

			// The last word is the field name, everything before it is the type
			fieldName := words[len(words)-1]
			fieldType := strings.Join(words[:len(words)-1], " ")

			// Handle pointer types
			if strings.HasPrefix(fieldName, "*") {
				// Count asterisks
				asteriskCount := 0
				for i := 0; i < len(fieldName) && fieldName[i] == '*'; i++ {
					asteriskCount++
				}
				// Remove asterisks from field name and add to type
				fieldName = strings.TrimPrefix(fieldName, strings.Repeat("*", asteriskCount))
				fieldType = fieldType + strings.Repeat("*", asteriskCount)
			}

			// Handle array types
			if strings.Contains(fieldName, "[") {
				bracketStart := strings.Index(fieldName, "[")
				bracketEnd := strings.LastIndex(fieldName, "]")
				if bracketStart != -1 && bracketEnd != -1 {
					baseName := fieldName[:bracketStart]
					arraySize := fieldName[bracketStart+1 : bracketEnd]
					fieldName = baseName
					fieldType = fieldType + "[" + arraySize + "]"
				}
			}

			// Skip if field name is empty
			if fieldName == "" {
				continue
			}

			// Add the field
			natureType := bg.mapCTypeToNature(fieldType)
			field := StructField{
				Name: bg.renameReservedKeywords(fieldName),
				Type: natureType,
			}

			// Add to the appropriate struct (nested or main)
			if bg.currentNested != nil {
				// If we're inside a union (currentNested.Name == ""), add to union fields
				if bg.currentNested.Name == "" {
					unionFields = append(unionFields, field)
					bg.currentNested.Fields = unionFields
					bg.headerLog.WriteString(fmt.Sprintf("    Added union field %s to union\n", field.Name))
				} else {
					// This is a nested struct, add to the nested struct
					bg.currentNested.Fields = append(bg.currentNested.Fields, field)
					bg.headerLog.WriteString(fmt.Sprintf("    Added field %s to nested struct %s\n", fieldName, bg.currentNested.Name))
				}
			} else {
				currentStruct.Fields = append(currentStruct.Fields, field)
			}
		}

		// Check for typedef struct end before function parsing
		if matches := typedefStructEndRegex.FindStringSubmatch(line); matches != nil {
			realStructName := matches[1]
			bg.headerLog.WriteString(fmt.Sprintf("    Found typedef struct end: %s\n", realStructName))

			// Check if this is actually an enum end, not a struct end
			if strings.Contains(realStructName, "Status") || strings.Contains(realStructName, "ENUM") {
				bg.headerLog.WriteString(fmt.Sprintf("    Skipping enum typedef end: %s\n", realStructName))
			} else {
				// Look for a pending anonymous struct to rename
				if bg.pendingStruct != nil && bg.pendingStruct.Name == "_anonymous_struct" {
					delete(bg.structs, "_anonymous_struct")
					bg.pendingStruct.Name = realStructName
					bg.structs[realStructName] = *bg.pendingStruct

					// Update any nested struct names that reference the old name
					bg.renameNestedStructs("_anonymous_struct", realStructName, &bg.pendingStruct.Fields)

					bg.structs[realStructName] = *bg.pendingStruct
					bg.headerLog.WriteString(fmt.Sprintf("    [DEBUG] Added struct: %s (typedef end)\n", realStructName))
					bg.pendingStruct = nil
				} else {
					bg.headerLog.WriteString(fmt.Sprintf("    No pending struct found for typedef end: %s\n", realStructName))
				}
			}
			continue
		}

		// Check for typedef struct name on next line: "CursedResult;"
		if matches := typedefStructEndNextLineRegex.FindStringSubmatch(line); matches != nil {
			realStructName := matches[1]
			bg.headerLog.WriteString(fmt.Sprintf("    Found typedef struct name on next line: %s\n", realStructName))

			// Check if this is actually an enum end, not a struct end
			if strings.Contains(realStructName, "Status") || strings.Contains(realStructName, "ENUM") {
				bg.headerLog.WriteString(fmt.Sprintf("    Skipping enum typedef end: %s\n", realStructName))
			} else {
				// Look for a pending anonymous struct to rename
				if bg.pendingStruct != nil && bg.pendingStruct.Name == "_anonymous_struct" {
					delete(bg.structs, "_anonymous_struct")
					bg.pendingStruct.Name = realStructName
					bg.structs[realStructName] = *bg.pendingStruct

					// Update any nested struct names that reference the old name
					bg.renameNestedStructs("_anonymous_struct", realStructName, &bg.pendingStruct.Fields)

					bg.structs[realStructName] = *bg.pendingStruct
					bg.headerLog.WriteString(fmt.Sprintf("    [DEBUG] Added struct: %s (typedef end next line)\n", realStructName))
					bg.pendingStruct = nil
				} else {
					bg.headerLog.WriteString(fmt.Sprintf("    No pending struct found for typedef end: %s\n", realStructName))
				}
			}
			continue
		}

		// Before the function regex check
		bg.headerLog.WriteString(fmt.Sprintf("    [DEBUG] Checking for function: %s\n", line))

		// Try to parse as a function with custom parsing to handle nested parentheses
		if funcName, returnType, paramStr := bg.parseFunctionDeclaration(line); funcName != "" {
			bg.headerLog.WriteString(fmt.Sprintf("    Found function: %s\n", funcName))

			// Clean up the return type to remove calling convention macros
			cleanedReturnType := bg.cleanReturnType(returnType)
			natureReturnType := bg.mapCTypeToNature(cleanedReturnType)

			params := bg.parseParameters(paramStr)

			binding := FunctionBinding{
				Name:       funcName,
				CName:      funcName,
				Parameters: params,
				ReturnType: natureReturnType,
			}

			bg.functions[funcName] = binding
			bg.headerLog.WriteString(fmt.Sprintf("    Added function binding: %s\n", funcName))
		} else {
			bg.headerLog.WriteString(fmt.Sprintf("    [DEBUG] Not a function: %s\n", line))
		}

		// Parse function pointer declarations
		if matches := funcPtrRegex.FindStringSubmatch(line); matches != nil {
			funcName := matches[2]
			paramStr := matches[3]

			// For function pointers, we map them to anyptr for now
			// The complex return type parsing would require more sophisticated logic
			returnType := "anyptr"

			params := bg.parseParameters(paramStr)

			binding := FunctionBinding{
				Name:       funcName,
				CName:      funcName,
				Parameters: params,
				ReturnType: returnType,
			}

			bg.functions[funcName] = binding
		}

		// Fallback: detect complex function pointer patterns that weren't caught by the regex
		if strings.Contains(line, "(*") && strings.Contains(line, ")(void)") && strings.Contains(line, ")(int)") {
			// This looks like a complex function pointer declaration
			// Extract the function name - look for (*func) pattern
			re := regexp.MustCompile(`\(\*([a-zA-Z_][a-zA-Z0-9_]*)\)`)
			if matches := re.FindStringSubmatch(line); matches != nil {
				funcName := matches[1]

				binding := FunctionBinding{
					Name:       funcName,
					CName:      funcName,
					Parameters: []Parameter{}, // Function pointers typically take no parameters
					ReturnType: "anyptr",      // Map complex function pointers to anyptr
				}

				bg.functions[funcName] = binding
			}
		}
	}

	return scanner.Err()
}

// parseNestedStructFields parses fields from a nested struct/union content string
func (bg *BindingGenerator) parseNestedStructFields(content string) []StructField {
	fields := make([]StructField, 0)

	// Split by semicolons to get individual field declarations
	fieldDeclarations := strings.Split(content, ";")

	for _, declaration := range fieldDeclarations {
		declaration = strings.TrimSpace(declaration)
		if declaration == "" {
			continue
		}

		// Simple field parsing - look for the last word as field name
		words := strings.Fields(declaration)
		if len(words) < 2 {
			continue // Skip lines with less than 2 words
		}

		// The last word is the field name, everything before it is the type
		fieldName := words[len(words)-1]
		fieldType := strings.Join(words[:len(words)-1], " ")

		// Handle pointer types
		if strings.HasPrefix(fieldName, "*") {
			// Count asterisks
			asteriskCount := 0
			for i := 0; i < len(fieldName) && fieldName[i] == '*'; i++ {
				asteriskCount++
			}
			// Remove asterisks from field name and add to type
			fieldName = strings.TrimPrefix(fieldName, strings.Repeat("*", asteriskCount))
			fieldType = fieldType + strings.Repeat("*", asteriskCount)
		}

		// Handle array types
		if strings.Contains(fieldName, "[") {
			bracketStart := strings.Index(fieldName, "[")
			bracketEnd := strings.LastIndex(fieldName, "]")
			if bracketStart != -1 && bracketEnd != -1 {
				baseName := fieldName[:bracketStart]
				arraySize := fieldName[bracketStart+1 : bracketEnd]
				fieldName = baseName
				fieldType = fieldType + "[" + arraySize + "]"
			}
		}

		// Skip if field name is empty
		if fieldName == "" {
			continue
		}

		// Add the field
		natureType := bg.mapCTypeToNature(fieldType)
		fields = append(fields, StructField{
			Name: bg.renameReservedKeywords(fieldName),
			Type: natureType,
		})
	}

	return fields
}

// parseParameters parses function parameters from a parameter string
func (bg *BindingGenerator) parseParameters(paramStr string) []Parameter {
	params := make([]Parameter, 0)

	if strings.TrimSpace(paramStr) == "" || strings.TrimSpace(paramStr) == "void" {
		return params
	}

	// Handle variadic functions (...)
	if strings.Contains(paramStr, "...") {
		// Remove the ... and any trailing comma
		paramStr = strings.ReplaceAll(paramStr, "...", "")
		paramStr = strings.TrimSuffix(strings.TrimSpace(paramStr), ",")
		paramStr = strings.TrimSpace(paramStr)

		// Parse regular parameters first (if any)
		if paramStr != "" {
			// Split parameters by comma, but be careful with nested parentheses
			paramParts := bg.splitParameters(paramStr)
			for i, part := range paramParts {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				// Extract type and name
				words := strings.Fields(part)
				if len(words) == 0 {
					continue
				}
				var paramType, paramName string
				if len(words) == 1 {
					paramType = words[0]
					paramName = fmt.Sprintf("arg%d", i)
				} else {
					paramName = words[len(words)-1]
					paramType = strings.Join(words[:len(words)-1], " ")
				}
				natureType := bg.mapCTypeToNature(paramType)
				params = append(params, Parameter{
					Name: bg.renameReservedKeywords(paramName),
					Type: natureType,
				})
			}
		}

		// Add variadic parameter at the end
		params = append(params, Parameter{
			Name: "args",
			Type: "...[any]",
		})
		return params
	}

	// Remove common macros that might appear in parameter lists
	paramStr = strings.ReplaceAll(paramStr, "SDL_PRINTF_FORMAT_STRING", "")
	paramStr = strings.ReplaceAll(paramStr, "SDL_FORMAT_PRINTF", "")
	paramStr = strings.ReplaceAll(paramStr, "SDL_FORMAT_STRING", "")
	paramStr = strings.TrimSpace(paramStr)

	// Split parameters by comma, but be careful with nested parentheses
	paramParts := bg.splitParameters(paramStr)

	for i, part := range paramParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Extract type and name
		words := strings.Fields(part)
		if len(words) == 0 {
			continue
		}

		var paramType, paramName string

		// Handle function pointer parameters like "int (*cb)(const char*, size_t)"
		if strings.Contains(part, "(*") && strings.Contains(part, ")(") {
			// Extract the function pointer type
			// Pattern: type (*name)(params)
			re := regexp.MustCompile(`([^(]+)\s*\(\*([^)]+)\)\s*\(([^)]*)\)`)
			if matches := re.FindStringSubmatch(part); matches != nil {
				returnType := strings.TrimSpace(matches[1])
				paramName = strings.TrimSpace(matches[2])
				paramParams := matches[3]

				// Parse the function pointer parameters
				funcParams := bg.parseParameters(paramParams)
				var paramTypes []string
				for _, p := range funcParams {
					paramTypes = append(paramTypes, p.Type)
				}

				// Create function type
				natureReturnType := bg.mapCTypeToNature(returnType)
				paramType = fmt.Sprintf("fn(%s):%s", strings.Join(paramTypes, ", "), natureReturnType)

				params = append(params, Parameter{
					Name: bg.renameReservedKeywords(paramName),
					Type: paramType,
				})
				continue
			}
		}

		// Handle array parameters like "char name[100]"
		if strings.Contains(part, "[") && !strings.Contains(part, "(") {
			// Find the array brackets
			bracketStart := strings.Index(part, "[")
			bracketEnd := strings.LastIndex(part, "]")

			if bracketStart != -1 && bracketEnd != -1 {
				// Extract the part before the brackets
				beforeBrackets := strings.TrimSpace(part[:bracketStart])
				words = strings.Fields(beforeBrackets)

				if len(words) >= 2 {
					// Last word is the name, rest is the type
					paramName = words[len(words)-1]
					paramType = strings.Join(words[:len(words)-1], " ") + "[]"
				} else {
					paramType = beforeBrackets + "[]"
					paramName = fmt.Sprintf("arg%d", i)
				}
			}
		} else if strings.Contains(part, "**") {
			// Handle double pointer parameters like "Rectangle **glyphRecs"
			parts := strings.Split(part, "**")
			if len(parts) >= 2 {
				baseType := strings.TrimSpace(parts[0])
				paramName = strings.TrimSpace(parts[1])
				if paramName == "" {
					paramName = fmt.Sprintf("arg%d", i)
				}
				// Map to array of rawptr<T>
				natureType := fmt.Sprintf("[rawptr<%s>]", bg.mapCTypeToNature(baseType))
				params = append(params, Parameter{
					Name: paramName,
					Type: natureType,
				})
				continue
			}
		} else if strings.Contains(part, "*") {
			// Handle pointer parameters like "struct Player *player"
			parts := strings.Split(part, "*")
			if len(parts) >= 2 {
				paramType = strings.TrimSpace(parts[0]) + "*"
				paramName = strings.TrimSpace(parts[1])
			} else {
				paramType = part
				paramName = fmt.Sprintf("arg%d", i)
			}
		} else if len(words) == 1 {
			// Only type, no name
			paramType = words[0]
			paramName = fmt.Sprintf("arg%d", i)
		} else {
			// Type and name
			paramName = words[len(words)-1]
			paramType = strings.Join(words[:len(words)-1], " ")
		}

		// Remove pointer indicators from name
		paramName = strings.TrimLeft(paramName, "*")
		if paramName == "" {
			paramName = fmt.Sprintf("arg%d", i)
		}

		natureType := bg.mapCTypeToNature(paramType)

		params = append(params, Parameter{
			Name: bg.renameReservedKeywords(paramName),
			Type: natureType,
		})
	}

	return params
}

// parseFunctionDeclaration parses a function declaration line and returns function name, return type, and parameter string
// This method handles nested parentheses in parameters (like function pointer parameters)
func (bg *BindingGenerator) parseFunctionDeclaration(line string) (string, string, string) {
	// Skip lines that don't look like function declarations
	if !strings.Contains(line, "(") || !strings.Contains(line, ")") {
		return "", "", ""
	}

	// Skip lines that are clearly not function declarations
	if strings.HasPrefix(strings.TrimSpace(line), "#") ||
		strings.HasPrefix(strings.TrimSpace(line), "//") ||
		strings.HasPrefix(strings.TrimSpace(line), "/*") {
		return "", "", ""
	}

	// Remove common prefixes
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "extern ")
	line = strings.TrimPrefix(line, "inline ")

	// Remove __attribute__ macros
	if strings.Contains(line, "__attribute__") {
		// Simple removal of __attribute__ macros
		re := regexp.MustCompile(`__attribute__\(\([^)]*\)\)\s*`)
		line = re.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
	}

	// Find the function name - look for the last identifier before the opening parenthesis
	// This handles cases like "int do_callback(const char* reason, int (*cb)(const char*, size_t));"

	// Find the opening parenthesis
	openParen := strings.Index(line, "(")
	if openParen == -1 {
		return "", "", ""
	}

	// Get everything before the opening parenthesis
	beforeParen := strings.TrimSpace(line[:openParen])

	// Split by whitespace to find the function name (last word)
	words := strings.Fields(beforeParen)
	if len(words) < 2 {
		return "", "", ""
	}

	// The last word is the function name, everything before it is the return type
	funcName := words[len(words)-1]
	returnType := strings.Join(words[:len(words)-1], " ")

	// Find the matching closing parenthesis for the parameter list
	// This handles nested parentheses in function pointer parameters
	parenCount := 0
	closeParen := -1
	for i, char := range line[openParen:] {
		if char == '(' {
			parenCount++
		} else if char == ')' {
			parenCount--
			if parenCount == 0 {
				closeParen = openParen + i
				break
			}
		}
	}

	if closeParen == -1 {
		return "", "", ""
	}

	// Extract the parameter string
	paramStr := line[openParen+1 : closeParen]

	// Check if this looks like a function declaration (ends with ; or {)
	afterParams := strings.TrimSpace(line[closeParen+1:])
	if !strings.HasSuffix(afterParams, ";") && !strings.HasSuffix(afterParams, "{") {
		return "", "", ""
	}

	return funcName, returnType, paramStr
}

// splitParameters splits a parameter string by commas, respecting parentheses
func (bg *BindingGenerator) splitParameters(paramStr string) []string {
	var parts []string
	var current strings.Builder
	parenCount := 0

	for _, char := range paramStr {
		switch char {
		case '(':
			parenCount++
			current.WriteRune(char)
		case ')':
			parenCount--
			current.WriteRune(char)
		case ',':
			if parenCount == 0 {
				parts = append(parts, current.String())
				current.Reset()
			} else {
				current.WriteRune(char)
			}
		default:
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// hasReservedKeywordField checks if a struct has a field named with a reserved keyword
func (bg *BindingGenerator) hasReservedKeywordField(structDef StructBinding) bool {
	reservedKeywords := map[string]bool{
		"type": true,
		"ptr":  true,
	}

	for _, field := range structDef.Fields {
		if reservedKeywords[field.Name] {
			return true
		}
	}
	return false
}

// hasReservedKeywordParameter checks if a function has a parameter named with a reserved keyword
func (bg *BindingGenerator) hasReservedKeywordParameter(fn FunctionBinding) bool {
	reservedKeywords := map[string]bool{
		"type": true,
		"ptr":  true,
	}

	for _, param := range fn.Parameters {
		if reservedKeywords[param.Name] {
			return true
		}
	}
	return false
}

// renameReservedKeywords renames reserved keywords by adding underscore suffix
func (bg *BindingGenerator) renameReservedKeywords(name string) string {
	reservedKeywords := map[string]bool{
		"type": true,
		"ptr":  true,
	}

	if reservedKeywords[name] {
		return name + "_"
	}
	return name
}

// renameNestedStructs updates nested struct names when the parent struct is renamed
func (bg *BindingGenerator) renameNestedStructs(oldParentName, newParentName string, parentFields *[]StructField) {
	bg.headerLog.WriteString(fmt.Sprintf("    [DEBUG] Renaming nested structs for parent: %s\n", oldParentName))
	for nestedName, nestedStruct := range bg.structs {
		bg.headerLog.WriteString(fmt.Sprintf("    [DEBUG] Checking nested struct: %s\n", nestedName))
		if strings.HasPrefix(nestedName, oldParentName+"_nested_") {
			bg.headerLog.WriteString(fmt.Sprintf("    [DEBUG] Renaming nested struct: %s\n", nestedName))
			// Create new name with the real struct name
			newNestedName := strings.Replace(nestedName, oldParentName, newParentName, 1)
			delete(bg.structs, nestedName)
			nestedStruct.Name = newNestedName
			bg.structs[newNestedName] = nestedStruct

			// Update any references to this nested struct in the parent
			for i, field := range *parentFields {
				if field.Type == nestedName {
					(*parentFields)[i].Type = newNestedName
					if field.Nested != nil {
						(*parentFields)[i].Nested = &nestedStruct
					}
				}
			}
		} else {
			bg.headerLog.WriteString(fmt.Sprintf("    [DEBUG] Skipping nested struct: %s\n", newParentName))
		}
	}
}

// sortConstantsByDependencies sorts constants so that constants that reference other constants are declared after the referenced ones
func (bg *BindingGenerator) sortConstantsByDependencies() []string {
	// Create a dependency graph
	dependencies := make(map[string][]string)

	// Filter out constants that are actually function renames (contain _renamed_)
	realConstants := make(map[string]string)
	for name, value := range bg.constants {
		// Skip constants that are function renames
		if strings.Contains(value, "_renamed_") {
			continue
		}
		realConstants[name] = value
	}

	// First, process all real constants to get their final values
	processedValues := make(map[string]string)
	for name, value := range realConstants {
		// Clean up C-style float literals
		cleanedValue := bg.cleanCFloatLiterals(value)
		// Replace enum references
		cleanedValue = bg.replaceEnumReferences(cleanedValue)
		// Convert C-style type casts
		cleanedValue = bg.convertCTypeCasts(cleanedValue)
		// Convert struct construction
		cleanedValue = bg.convertStructConstruction(cleanedValue)
		processedValues[name] = cleanedValue
	}

	// Now detect dependencies based on processed values
	for name, processedValue := range processedValues {
		deps := []string{}
		for otherName := range realConstants {
			if otherName != name {
				// Use word boundary matching to avoid partial matches
				pattern := fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(otherName))
				if matched, _ := regexp.MatchString(pattern, processedValue); matched {
					deps = append(deps, otherName)
				}
			}
		}
		dependencies[name] = deps
	}

	// Stable iterative topological sort
	var result []string
	added := make(map[string]bool)
	for len(result) < len(realConstants) {
		progress := false
		for name := range realConstants {
			if added[name] {
				continue
			}
			allDepsAdded := true
			for _, dep := range dependencies[name] {
				if !added[dep] {
					allDepsAdded = false
					break
				}
			}
			if allDepsAdded {
				result = append(result, name)
				added[name] = true
				progress = true
			}
		}
		if !progress {
			// Cycle detected or something went wrong, add remaining in any order
			for name := range realConstants {
				if !added[name] {
					result = append(result, name)
					added[name] = true
				}
			}
			break
		}
	}
	return result
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
		for _, name := range sortedConstants {
			value := bg.constants[name]

			// Skip constants that are function renames
			if strings.Contains(value, "_renamed_") {
				continue
			}

			// Check if the constant contains __attribute__
			if strings.Contains(value, "__attribute__") {
				// Comment out constants with __attribute__
				sb.WriteString(fmt.Sprintf("// var %s = %s // Commented out due to __attribute__\n",
					name, bg.convertStructConstruction(value)))
			} else {
				// Clean up C-style literals
				cleanedValue := bg.cleanCIntegerLiterals(value)
				cleanedValue = bg.cleanCFloatLiterals(cleanedValue)

				// Replace enum references
				cleanedValue = bg.replaceEnumReferences(cleanedValue)
				// Convert C-style type casts
				cleanedValue = bg.convertCTypeCasts(cleanedValue)

				// Infer the appropriate type
				constType := bg.inferConstantType(cleanedValue)

				// Use explicit type declaration instead of var with type inference
				sb.WriteString(fmt.Sprintf("%s %s = %s\n",
					constType, name, bg.convertStructConstruction(cleanedValue)))
			}
		}
		sb.WriteString("\n")
	}

	// Generate const int declarations
	if len(bg.constantValues) > 0 {
		sb.WriteString("// Constants\n")
		for name, value := range bg.constantValues {
			sb.WriteString(fmt.Sprintf("int %s = %d\n", name, value))
		}
		sb.WriteString("\n")
	}

	// Generate enum constants
	if len(bg.enums) > 0 {
		sb.WriteString("// Enum constants\n")
		for _, enum := range bg.enums {
			for _, member := range enum.Members {
				if member.Literal != "" {
					sb.WriteString(fmt.Sprintf("int %s_C_ENUM_%s = %s\n", enum.Name, member.Name, member.Literal))
				} else {
					sb.WriteString(fmt.Sprintf("int %s_C_ENUM_%s = %d\n", enum.Name, member.Name, member.Value))
				}
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

	// Generate struct definitions
	if len(bg.structs) > 0 {
		sb.WriteString("// Struct definitions\n")

		// First, collect all structs and their dependencies
		type StructInfo struct {
			Name         string
			Dependencies []string
		}

		var structInfos []StructInfo
		for name, structDef := range bg.structs {
			deps := []string{}
			for _, field := range structDef.Fields {
				if field.Nested != nil && field.Nested.Name != "" {
					deps = append(deps, field.Nested.Name)
				}
			}
			structInfos = append(structInfos, StructInfo{Name: name, Dependencies: deps})
		}

		// Topological sort to ensure dependencies come first
		var sortedStructs []string
		added := make(map[string]bool)

		for len(sortedStructs) < len(structInfos) {
			progress := false
			for _, info := range structInfos {
				if added[info.Name] {
					continue
				}

				// Check if all dependencies are already added
				allDepsAdded := true
				for _, dep := range info.Dependencies {
					if !added[dep] {
						allDepsAdded = false
						break
					}
				}

				if allDepsAdded {
					sortedStructs = append(sortedStructs, info.Name)
					added[info.Name] = true
					progress = true
				}
			}

			if !progress {
				// If no progress, add remaining structs in any order
				for _, info := range structInfos {
					if !added[info.Name] {
						sortedStructs = append(sortedStructs, info.Name)
						added[info.Name] = true
					}
				}
				break
			}
		}

		// Output structs in dependency order
		for _, name := range sortedStructs {
			structDef := bg.structs[name]
			sb.WriteString(fmt.Sprintf("type %s = struct {\n", structDef.Name))
			for _, field := range structDef.Fields {
				if field.IsUnion {
					for _, ufield := range field.UnionFields {
						if ufield.Nested != nil {
							sb.WriteString(fmt.Sprintf("    %s %s\n", ufield.Nested.Name, ufield.Name))
						} else {
							sb.WriteString(fmt.Sprintf("    %s %s\n", ufield.Type, ufield.Name))
						}
					}
					continue
				} else if field.Nested != nil {
					sb.WriteString(fmt.Sprintf("    %s %s\n", field.Nested.Name, field.Name))
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

// generatePackageToml generates a package.toml file with library linking configuration
func (bg *BindingGenerator) generatePackageToml(libName, libPath string) string {
	var sb strings.Builder

	sb.WriteString("# Generated package.toml for Nature bindings\n")
	sb.WriteString("name = \"generated-bindings\"\n")
	sb.WriteString("version = \"1.0.0\"\n")
	sb.WriteString("authors = [\"naturebindgen\"]\n")
	sb.WriteString("description = \"Generated C bindings for Nature\"\n")
	sb.WriteString("type = \"lib\"\n\n")

	sb.WriteString("[links]\n")
	sb.WriteString(fmt.Sprintf("%s = {\n", libName))
	sb.WriteString(fmt.Sprintf("    linux_amd64 = '%s/linux_amd64/lib%s.a',\n", libPath, libName))
	sb.WriteString(fmt.Sprintf("    darwin_amd64 = '%s/darwin_amd64/lib%s.a',\n", libPath, libName))
	sb.WriteString(fmt.Sprintf("    linux_arm64 = '%s/linux_arm64/lib%s.a',\n", libPath, libName))
	sb.WriteString(fmt.Sprintf("    darwin_arm64 = '%s/darwin_arm64/lib%s.a'\n", libPath, libName))
	sb.WriteString("}\n")

	return sb.String()
}

// generateUsageExample generates a usage example
func (bg *BindingGenerator) printHeaderLog() {
	fmt.Println("\n=== Header Parsing Log ===")
	fmt.Println(strings.Contains(bg.headerLog.String(), "SDL_render.h"))
	fmt.Print(bg.headerLog.String())
	fmt.Println("=== End Header Log ===")
}

func (bg *BindingGenerator) generateUsageExample() string {
	var sb strings.Builder

	sb.WriteString("// Usage example\n")
	sb.WriteString("import fmt\n")
	sb.WriteString("import bindings // Import the generated bindings\n\n")

	sb.WriteString("fn main() {\n")

	if len(bg.functions) > 0 {
		// Generate example for the first function
		var fn FunctionBinding
		for _, f := range bg.functions {
			fn = f
			break // Get the first function
		}
		sb.WriteString(fmt.Sprintf("    // Example usage of %s\n", fn.Name))

		// Generate example parameters
		var exampleArgs []string
		for _, param := range fn.Parameters {
			switch param.Type {
			case "int", "i32":
				exampleArgs = append(exampleArgs, "42")
			case "float", "f32", "f64":
				exampleArgs = append(exampleArgs, "3.14")
			case "anyptr":
				exampleArgs = append(exampleArgs, "null")
			case "string":
				exampleArgs = append(exampleArgs, "'example'")
			default:
				exampleArgs = append(exampleArgs, fmt.Sprintf("/* %s value */", param.Type))
			}
		}

		if fn.ReturnType != "void" {
			sb.WriteString(fmt.Sprintf("    var result = bindings.%s(%s)\n",
				fn.Name, strings.Join(exampleArgs, ", ")))
			sb.WriteString("    fmt.printf('Result: %v\\n', result)\n")
		} else {
			sb.WriteString(fmt.Sprintf("    bindings.%s(%s)\n",
				fn.Name, strings.Join(exampleArgs, ", ")))
		}
	}

	sb.WriteString("}\n")

	return sb.String()
}

// cleanCFloatLiterals removes C-style 'f' suffixes from float literals
func (bg *BindingGenerator) cleanCFloatLiterals(value string) string {
	// Remove 'f' and 'F' suffixes from float literals like 180.0f -> 180.0, 180.0F -> 180.0
	// Use regex to match patterns like 123.456f, 0.0f, 123F, etc.
	// Handle both decimal and integer literals with f/F suffix
	re := regexp.MustCompile(`(\d+\.\d+)f\b`)
	value = re.ReplaceAllString(value, "$1")
	re = regexp.MustCompile(`(\d+\.\d+)F\b`)
	value = re.ReplaceAllString(value, "$1")
	// Also handle integer literals with f/F suffix (like 123f, 456F)
	re = regexp.MustCompile(`(\d+)f\b`)
	value = re.ReplaceAllString(value, "$1.0")
	re = regexp.MustCompile(`(\d+)F\b`)
	value = re.ReplaceAllString(value, "$1.0")
	return value
}

// cleanReturnType intelligently extracts the actual return type by filtering out non-type tokens
func (bg *BindingGenerator) cleanReturnType(returnType string) string {
	// Split the return type into tokens
	tokens := strings.Fields(returnType)
	var validTokens []string

	// Define what constitutes a valid C type token
	validTypePatterns := []*regexp.Regexp{
		regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`),       // Identifiers like "int", "Window"
		regexp.MustCompile(`^(unsigned|signed|long|short)$`), // Type modifiers
		regexp.MustCompile(`^(const|volatile|restrict)$`),    // Type qualifiers
		regexp.MustCompile(`^(struct|union|enum)$`),          // Type specifiers
		regexp.MustCompile(`^\*+$`),                          // Pointer indicators
		regexp.MustCompile(`^[<>]$`),                         // Template brackets
		regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*\*$`),     // Pointer types like "void*"
	}

	// Common non-type tokens to exclude (generic C keywords)
	excludeTokens := map[string]bool{
		"extern": true, "inline": true, "static": true, "register": true, "auto": true,
		"__cdecl": true, "__stdcall": true, "__fastcall": true, "__thiscall": true,
		"__vectorcall": true, "__clrcall": true,
	}

	for _, token := range tokens {
		// Skip excluded tokens
		if excludeTokens[token] {
			continue
		}

		// Check if token matches any valid type pattern
		isValid := false
		for _, pattern := range validTypePatterns {
			if pattern.MatchString(token) {
				isValid = true
				break
			}
		}

		// Skip macro calls (anything with parentheses)
		if strings.Contains(token, "(") && strings.Contains(token, ")") {
			continue
		}

		// Skip tokens that look like calling conventions (all caps with CALL)
		if regexp.MustCompile(`^[A-Z_]*CALL$`).MatchString(token) {
			continue
		}

		// Skip tokens that look like macros (all caps with underscores)
		if regexp.MustCompile(`^[A-Z_]+$`).MatchString(token) && len(token) > 3 {
			continue
		}

		if isValid {
			validTokens = append(validTokens, token)
		}
	}

	// Reconstruct the cleaned return type
	cleanedType := strings.Join(validTokens, " ")

	// Clean up extra whitespace
	cleanedType = regexp.MustCompile(`\s+`).ReplaceAllString(cleanedType, " ")
	cleanedType = strings.TrimSpace(cleanedType)

	return cleanedType
}

// cleanCIntegerLiterals removes C-style integer literal suffixes
func (bg *BindingGenerator) cleanCIntegerLiterals(value string) string {
	// Remove common C integer literal suffixes: u, U, l, L, ul, UL, ull, ULL, ll, LL
	// These patterns match suffixes that can appear after hex, octal, or decimal numbers

	// Remove 'u' or 'U' suffix (unsigned)
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)u\b`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)U\b`).ReplaceAllString(value, "$1")

	// Remove 'l' or 'L' suffix (long)
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)l\b`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)L\b`).ReplaceAllString(value, "$1")

	// Remove 'ul', 'UL', 'lu', 'LU' suffixes (unsigned long)
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)ul\b`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)UL\b`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)lu\b`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)LU\b`).ReplaceAllString(value, "$1")

	// Remove 'ull', 'ULL', 'llu', 'LLU' suffixes (unsigned long long)
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)ull\b`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)ULL\b`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)llu\b`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)LLU\b`).ReplaceAllString(value, "$1")

	// Remove 'll', 'LL' suffixes (long long)
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)ll\b`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`(\d+[xX]?[0-9a-fA-F]*)LL\b`).ReplaceAllString(value, "$1")

	return value
}

// inferConstantType determines the appropriate Nature type for a constant value
func (bg *BindingGenerator) inferConstantType(value string) string {
	// Clean up the value first
	cleanedValue := bg.cleanCIntegerLiterals(value)
	cleanedValue = bg.cleanCFloatLiterals(cleanedValue)

	// Check if it's a string literal
	if strings.HasPrefix(cleanedValue, "\"") && strings.HasSuffix(cleanedValue, "\"") {
		return "string"
	}

	// Check if it's a character literal
	if strings.HasPrefix(cleanedValue, "'") && strings.HasSuffix(cleanedValue, "'") {
		return "u8"
	}

	// Check if it's a boolean
	if cleanedValue == "true" || cleanedValue == "false" {
		return "bool"
	}

	// Check if it's a float (contains decimal point)
	if strings.Contains(cleanedValue, ".") {
		return "float"
	}

	// Check if it's a hex number
	if strings.HasPrefix(cleanedValue, "0x") || strings.HasPrefix(cleanedValue, "0X") {
		// Remove the 0x prefix to check the value
		hexValue := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(cleanedValue, "0x"), "0X"))

		// Parse the hex value to determine size
		if len(hexValue) <= 8 {
			// Try to parse as u32
			if _, err := fmt.Sscanf(cleanedValue, "%x", new(uint32)); err == nil {
				return "u32"
			}
		}
		if len(hexValue) <= 16 {
			// Try to parse as u64
			if _, err := fmt.Sscanf(cleanedValue, "%x", new(uint64)); err == nil {
				return "u64"
			}
		}
		// Default to u32 for hex values
		return "u32"
	}

	// For decimal numbers, try to determine the appropriate size
	// First, try to parse as int64 to see if it fits
	var int64Val int64
	if _, err := fmt.Sscanf(cleanedValue, "%d", &int64Val); err == nil {
		// Check if it's negative
		if int64Val < 0 {
			if int64Val >= -2147483648 && int64Val <= 2147483647 {
				return "i32"
			}
			return "i64"
		}

		// Positive number - determine unsigned type
		if int64Val <= 255 {
			return "u8"
		}
		if int64Val <= 65535 {
			return "u16"
		}
		if int64Val <= 4294967295 {
			return "u32"
		}
		return "u64"
	}

	// Default to int if we can't parse it
	return "int"
}

// replaceEnumReferences replaces enum references in constants with their proper constant names
func (bg *BindingGenerator) replaceEnumReferences(value string) string {
	// Look for enum references like MOUSE_BUTTON_RIGHT and replace with MouseButton_C_ENUM_MOUSE_BUTTON_RIGHT
	for _, enum := range bg.enums {
		for _, member := range enum.Members {
			// Replace exact matches of enum member names
			value = strings.ReplaceAll(value, member.Name, fmt.Sprintf("%s_C_ENUM_%s", enum.Name, member.Name))
		}
	}
	return value
}

// convertCTypeCasts converts C-style type casts to Nature type conversion syntax
func (bg *BindingGenerator) convertCTypeCasts(value string) string {
	// Handle patterns like ((Uint8)0xFF) -> (0xFF as u8)
	// Match: ((Type)value) where Type is a C type and value is any expression
	re := regexp.MustCompile(`\(\(([^)]+)\)([^)]+)\)`)

	return re.ReplaceAllStringFunc(value, func(match string) string {
		// Extract the type and value from the match
		submatches := re.FindStringSubmatch(match)
		if len(submatches) != 3 {
			return match // Return as-is if we can't parse it
		}

		cType := strings.TrimSpace(submatches[1])
		value := strings.TrimSpace(submatches[2])

		// Convert C type to Nature type
		natureType := bg.mapCTypeToNature(cType)

		// Return Nature type conversion syntax
		return fmt.Sprintf("(%s as %s)", value, natureType)
	})
}

// convertStructConstruction converts C struct construction to Nature struct syntax
func (bg *BindingGenerator) convertStructConstruction(value string) string {
	// Look for patterns like CLITERAL(Type){...} or Type{...} or any struct construction
	// Find the type name and field values

	// Pattern: CLITERAL(Type){...} or Type{...}
	// Extract type name
	var typeName string
	var valuesStr string

	// Check for CLITERAL pattern
	if strings.Contains(value, "CLITERAL(") && strings.Contains(value, "){") {
		start := strings.Index(value, "CLITERAL(")
		end := strings.Index(value, "){")
		if start != -1 && end != -1 {
			typeName = value[start+9 : end] // Skip "CLITERAL("
			valuesStart := strings.Index(value, "{")
			valuesEnd := strings.LastIndex(value, "}")
			if valuesStart != -1 && valuesEnd != -1 {
				valuesStr = value[valuesStart+1 : valuesEnd]
			}
		}
	} else if strings.Contains(value, "{") && strings.Contains(value, "}") {
		// Direct struct construction pattern: Type{...}
		braceStart := strings.Index(value, "{")
		if braceStart != -1 {
			typeName = strings.TrimSpace(value[:braceStart])
			valuesEnd := strings.LastIndex(value, "}")
			if valuesEnd != -1 {
				valuesStr = value[braceStart+1 : valuesEnd]
			}
		}
	}

	if typeName == "" || valuesStr == "" {
		return value // Return as-is if we can't parse it
	}

	// Split values by comma
	values := strings.Split(valuesStr, ",")

	// Clean up values
	for i, v := range values {
		values[i] = strings.TrimSpace(v)
	}

	// Find the struct definition to get field names
	var fieldNames []string
	for _, structDef := range bg.structs {
		if structDef.Name == typeName {
			for _, field := range structDef.Fields {
				fieldNames = append(fieldNames, field.Name)
			}
			break
		}
	}

	// If we found the struct and have matching field count, construct Nature syntax
	if len(fieldNames) > 0 && len(fieldNames) == len(values) {
		var fieldAssignments []string
		for i, fieldName := range fieldNames {
			fieldAssignments = append(fieldAssignments, fmt.Sprintf("%s=%s", fieldName, values[i]))
		}
		return fmt.Sprintf("%s{%s}", typeName, strings.Join(fieldAssignments, ", "))
	}

	// If we can't match fields, just return the type with values
	return fmt.Sprintf("%s{%s}", typeName, valuesStr)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: naturebindgen <header-file> [options]")
		fmt.Println("Options:")
		fmt.Println("  -o, --output <file>     Output file (default: bindings.n)")
		fmt.Println("  -l, --lib <name>        Library name for package.toml")
		fmt.Println("  -p, --lib-path <path>   Library path for package.toml")
		fmt.Println("  --package-toml          Generate package.toml file")
		fmt.Println("  --example              Generate usage example")
		fmt.Println("  -h, --help             Show this help message")
		os.Exit(1)
	}

	headerFile := os.Args[1]
	outputFile := "bindings.n"
	libName := "mylib"
	libPath := "libs"
	generatePackage := false
	generateExample := false

	// Parse command line arguments
	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "-o", "--output":
			if i+1 < len(os.Args) {
				outputFile = os.Args[i+1]
				i++
			}
		case "-l", "--lib":
			if i+1 < len(os.Args) {
				libName = os.Args[i+1]
				i++
			}
		case "-p", "--lib-path":
			if i+1 < len(os.Args) {
				libPath = os.Args[i+1]
				i++
			}
		case "--package-toml":
			generatePackage = true
		case "--example":
			generateExample = true
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
	if err := bg.parseHeaderFileRecursive(headerFile); err != nil {
		fmt.Printf("Error parsing header file: %v\n", err)
		os.Exit(1)
	}

	// In main(), after parsing the header file and before generating bindings, add:
	fmt.Println("\n=== DEBUG: Structs parsed ===")
	for name := range bg.structs {
		fmt.Println("struct:", name)
	}
	fmt.Println("=== DEBUG: Functions parsed ===")
	for name := range bg.functions {
		fmt.Println("function:", name)
	}
	fmt.Println("============================\n")

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

	// Generate package.toml if requested
	if generatePackage {
		packageToml := bg.generatePackageToml(libName, libPath)
		if err := os.WriteFile("package.toml", []byte(packageToml), 0644); err != nil {
			fmt.Printf("Error writing package.toml: %v\n", err)
		} else {
			fmt.Println("Generated package.toml")
		}
	}

	// Generate usage example if requested
	if generateExample {
		example := bg.generateUsageExample()
		exampleFile := strings.TrimSuffix(outputFile, filepath.Ext(outputFile)) + "_example.n"
		if err := os.WriteFile(exampleFile, []byte(example), 0644); err != nil {
			fmt.Printf("Error writing example file: %v\n", err)
		} else {
			fmt.Printf("Generated example: %s\n", exampleFile)
		}
	}
}
