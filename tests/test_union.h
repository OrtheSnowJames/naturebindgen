typedef union {
    int i;
    float f;
    char str[20];
} Data;

typedef struct {
    int id;
    Data data;
} Container; 