#ifndef CURSED_H
#define CURSED_H

#include <stdint.h>
#include <stddef.h>

#define CURSED_VERSION "1.0.0" // start off slow

// multiline macro with comments (you will suffer)
#define CURSED_MACRO(x, y) \
    (((x) * (y)) + ( \
        (x) << 2 /* shift left */ \
    ))

// forward-declared struct
typedef struct CursedStruct CursedStruct;

// typedef'd function pointer in a typedef'd struct inside a typedef'd union
typedef void (*CursedCallback)(int, void*);
typedef struct {
    union {
        int status;
        void* ptr;
        struct {
            const char* name;
            CursedCallback callback;
        } nested;
    } data;
} CursedResult;

// C++ extern fence + __attribute__ + inline + pointer to function returning pointer to function
#ifdef __cplusplus
extern "C" {
#endif

__attribute__((hot, deprecated("DO NOT USE THIS FUNCTION")))
inline void* (*get_cursed_handler(const char* tag))(int);

// variadic with attributes and deeply strange naming
__attribute__((format(printf, 1, 2)))
int cursed_logf(const char* fmt, ...);

// function with nested function pointer as param
int do_callback(const char* reason, int (*cb)(const char*, size_t));

// macro-defined function-like madness
#define cursed_alias do_callback

// typedef of a function pointer with complex return type
typedef int* (*weird_return_func)(const char**, int32_t);

// enum with gaps
typedef enum {
    CURSED_OK = 0,
    CURSED_MAYBE = 42,
    CURSED_NOPE = 9001,
} CursedStatus;

#ifdef __cplusplus
}
#endif

#endif // CURSED_H
