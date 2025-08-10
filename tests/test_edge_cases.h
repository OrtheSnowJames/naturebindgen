#ifndef TEST_EDGE_CASES_H
#define TEST_EDGE_CASES_H

#include <stdint.h>
#include <stddef.h>
// Enums
typedef enum {
    RED = 1,
    GREEN = 2,
    BLUE = 3,
} Color;

// Macros
#define VERSION "1.0.0"
#define MAX_SIZE 1024
#define FLAG_ENABLED 1

// Basic structs for pointer testing
typedef struct {
    int x;
    int y;
} Point;

typedef struct {
    char name[50];
    int age;
} Person;

// Anonymous structs
typedef struct {
    struct {
        int x;
        int y;
    } point;
    int id;
} Container;

typedef struct {
    int x;
    int y;
} Point;

#define point Point{1, 2}

// Function pointers
typedef int (*Callback)(int, void*);
typedef void (*Handler)(const char*);
typedef int* (*ArrayProcessor)(int*, size_t);

// Unions
typedef union {
    int i;
    float f;
    char str[8];
} Data;

typedef union {
    uint64_t big;
    double d;
} BigData;

// Nested unions
typedef struct {
    union {
        int status;
        char* error;
    } result;
    int id;
} Response;

// Variadic functions
int printf(const char* fmt, ...);
int sprintf(char* buf, const char* fmt, ...);

// Function with function pointer parameter
int do_callback(const char* reason, Callback cb);

// Pointer functions
void draw_point(Point* p);
void draw_anything(void* data);
void draw_string(const char* str);
void process_person(Person* person);

// Complex nested structure
typedef struct {
    struct {
        union {
            int count;
            float ratio;
        } metric;
        char* description;
    } info;
    Point position;
} ComplexStruct;

// Deep nested compound literal test macro
#define COMPLEX_VAL ComplexStruct{ { { 42 }, "ok" }, (Point){3,4} }

#endif // TEST_EDGE_CASES_H 