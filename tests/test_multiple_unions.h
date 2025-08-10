#include <stdint.h>

typedef union {
    int i;
    float f;
} Union1;

typedef union {
    char str[4];
    int32_t num;
} Union2;

typedef struct {
    Union1 u1;
    Union2 u2;
} TestStruct; 