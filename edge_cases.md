# Edge Cases

## Enums

This binding generator was built before the nature language received enum grammar support. It defines enum constants and then wants you to use them later.

```c
typedef enum {
    RED = 1,
    GREEN = 2,
    BLUE = 3,
} Color;
```

Generates:
```nature
int Color_C_ENUM_RED = 1
int Color_C_ENUM_GREEN = 2
int Color_C_ENUM_BLUE = 3
```

## Pointers

It uses rawptr<T> for defined struct pointers, and it uses anyptr for anything that isn't defined in the bindings or is an enum.

```c
typedef struct {
    int x;
    int y;
} Point;

void draw_point(Point* p);           // rawptr<Point>
void draw_anything(void* data);      // anyptr
void draw_string(const char* str);   // anyptr
```

## Unions

The generator makes a wrapper around an array of bytes with the size of the largest element. The generator makes a new struct for each new union size and adds fields from all the unions of that size to the union. Then you have .get_(type), .set_(type) for each field. Then you have .get_ptr to get an anyptr to the data. Then .to_c() for getting the c union.

```c
typedef union {
    int i;
    float f;
    char str[8];
} Data;

typedef union {
    uint64_t big;
    double d;
} BigData;
```

Generates:
```nature
type Union_eight_bytes = struct {
    [u8;8] data
}

fn Union_eight_bytes.get_i_i32():i32 { ... }
fn Union_eight_bytes.set_i_i32(i32 value) { ... }
fn Union_eight_bytes.get_f_f32():f32 { ... }
fn Union_eight_bytes.set_f_f32(f32 value) { ... }
fn Union_eight_bytes.get_str__i8_8():[i8;8] { ... }
fn Union_eight_bytes.set_str__i8_8([i8;8] value) { ... }
fn Union_eight_bytes.to_c():[u8;8] { ... }
```

## Anonymous Structs

The generator gives anonymous structs descriptive names based on where they're found. If it's inside a field, it gets named like AnonymousStruct_1_fieldname_parentstruct.

```c
typedef struct {
    struct {
        int x;
        int y;
    } point;
    int id;
} Container;
```

Generates:
```nature
type AnonymousStruct_1_point_Container = struct {
    i32 x
    i32 y
}

type Container = struct {
    AnonymousStruct_1_point_Container point
    i32 id
}
```

## Function Pointers

The generator makes function pointer typedefs into Nature function types.

```c
typedef int (*Callback)(int, void*);
typedef void (*Handler)(const char*);
```

Generates:
```nature
type Callback = fn(i32, anyptr):i32
type Handler = fn(anyptr):void
```

## Variadic Functions

The generator detects variadic functions and adds the variadic parameter.

```c
int printf(const char* fmt, ...);
int sprintf(char* buf, const char* fmt, ...);
```

Generates:
```nature
fn printf(anyptr fmt, ...[anyptr] args):i32
fn sprintf(anyptr buf, anyptr fmt, ...[anyptr] args):i32
```

## Nested Unions

When unions are nested inside structs, the generator handles them properly.

```c
typedef struct {
    union {
        int status;
        char* error;
    } result;
    int id;
} Response;
```

Generates:
```nature
type Union_eight_bytes = struct {
    [u8;8] data
}

type Response = struct {
    Union_eight_bytes result
    i32 id
}
```

## Macros

The generator tries to read macro values from the source file and defines them as constants.

```c
#define VERSION "1.0.0"
#define MAX_SIZE 1024
#define FLAG_ENABLED 1
```

Generates:
```nature
string VERSION = "1.0.0"
int MAX_SIZE = 1024
int FLAG_ENABLED = 1
```

