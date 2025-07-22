# Issues Found in Nature Binding Generator

## Current Output vs Expected Output

### Current Output (cursed_final.n):
```nature
type CursedResult = struct {
    i32 status
    anyptr ptr
    anyptr name
    anyptr callback
    anyptr nested
    anyptr data
}

#linkid CursedCallback
fn CursedCallback(i32 arg0, anyptr arg1)

#linkid cursed_logf
fn cursed_logf(anyptr fmt):i32
```

### Expected Output (expected_cursed_result.n):
```nature
type CursedCallback = fn(anyptr, anyptr):int

type CursedNested = struct {
    anyptr name
    CursedCallback callback
}

type CursedResult = struct {
    int status
    anyptr ptr_
    CursedNested nested
}

#linkid cursed_logf
fn cursed_logf(anyptr fmt, ...[any] args):int
```

## Issues Identified:

### 1. Function Pointer Typedefs
**Problem**: `typedef void (*CursedCallback)(int, void*);` is being treated as a function binding instead of a type definition.

**Location**: Lines 630-640 in main.go
```go
// Create a function binding for the typedef
binding := FunctionBinding{
    Name:       funcPtrName,
    CName:      funcPtrName,
    Parameters: params,
    ReturnType: bg.mapCTypeToNature(returnType),
}
bg.functions[funcPtrName] = binding
```

**Fix**: Should create a type mapping instead:
```go
// Create a function pointer type mapping
natureType := fmt.Sprintf("fn(%s):%s", paramTypes, bg.mapCTypeToNature(returnType))
bg.typeMappings[funcPtrName] = TypeMapping{CType: funcPtrName, NatureType: natureType, IsPointer: false}
```

### 2. Union Handling
**Problem**: Union fields are being output as separate struct members instead of properly representing the union structure.

**Location**: Lines 1538-1551 in main.go
```go
if field.IsUnion {
    sb.WriteString(fmt.Sprintf("    // union %s\n", field.Name))
    for _, ufield := range field.UnionFields {
        // ... output union fields
    }
    continue
}
```

**Fix**: Should output union fields as separate struct members since Nature doesn't have unions:
```go
if field.IsUnion {
    // Output all union fields as separate struct members
    for _, ufield := range field.UnionFields {
        if ufield.Nested != nil {
            // Create separate type for nested struct
            sb.WriteString(fmt.Sprintf("type %s = struct {\n", ufield.Name))
            for _, nestedField := range ufield.Nested.Fields {
                sb.WriteString(fmt.Sprintf("    %s %s\n", nestedField.Type, nestedField.Name))
            }
            sb.WriteString("}\n\n")
            // Add field reference
            sb.WriteString(fmt.Sprintf("    %s %s\n", ufield.Name, ufield.Name))
        } else {
            sb.WriteString(fmt.Sprintf("    %s %s\n", ufield.Type, ufield.Name))
        }
    }
    continue
}
```

### 3. Variadic Functions
**Problem**: Variadic functions are missing the proper `...[any] args` syntax.

**Location**: Lines 1190-1210 in main.go (parseParameters function)
```go
// Handle variadic functions (...)
if strings.Contains(paramStr, "...") {
    // Remove the ... and any trailing comma
    paramStr = strings.ReplaceAll(paramStr, "...", "")
    // ... rest of handling
}
```

**Fix**: Should add the variadic parameter:
```go
// Handle variadic functions (...)
if strings.Contains(paramStr, "...") {
    // Remove the ... and any trailing comma
    paramStr = strings.ReplaceAll(paramStr, "...", "")
    paramStr = strings.TrimSuffix(strings.TrimSpace(paramStr), ",")
    paramStr = strings.TrimSpace(paramStr)
    
    // Add variadic parameter
    params = append(params, Parameter{
        Name: "args",
        Type: "...[any]",
    })
}
```

### 4. Nested Struct in Union
**Problem**: Nested structs inside unions are not being defined as separate types.

**Location**: Lines 964-1010 in main.go
```go
if strings.HasPrefix(trimmedLine, "struct ") && strings.Contains(trimmedLine, "{") {
    // Parse the nested struct fields
    nestedStruct := &StructBinding{
        Name:   "",
        Fields: make([]StructField, 0),
    }
    // ... parsing logic
}
```

**Fix**: Should create a proper type name for the nested struct and define it separately.

### 5. Union Field Naming
**Problem**: The union field name is being set correctly as `data`, but the union members are being output incorrectly.

**Location**: Lines 875-883 in main.go
```go
currentUnionField := StructField{
    Name:        bg.renameReservedKeywords(fieldName),
    Type:        fieldName,  // This should be the union field name
    IsUnion:     true,
    UnionFields: unionFields,
}
```

**Fix**: The union field should be named after the union's field name (e.g., `data`), and all union members should be output as separate struct fields.

## Recommended Fixes:

1. **Fix function pointer typedefs** to create type mappings instead of function bindings
2. **Fix union handling** to output all union fields as separate struct members
3. **Fix variadic functions** to include the proper `...[any] args` syntax
4. **Fix nested structs in unions** to be defined as separate types
5. **Fix union field naming** to properly represent the union structure in Nature syntax 