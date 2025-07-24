#ifndef NESTED_TEST_H
#define NESTED_TEST_H

#include <stdint.h>
#include <stddef.h>
#include "share.h"

// Basic types for testing
typedef struct {
    int x;
    int y;
} Point2D;

typedef struct {
    float x, y, z;
} Point3D;

// Simple nested struct
typedef struct {
    struct {
        int width;
        int height;
    } dimensions;
    char* name;
} Rectangle;

// Nested struct with anonymous inner struct
typedef struct {
    struct {
        uint8_t r, g, b, a;
    } color;
    Point2D position;
} ColoredPoint;

// Complex nested structure with multiple levels
typedef struct {
    struct {
        struct {
            int x, y;
        } top_left;
        struct {
            int x, y;
        } bottom_right;
    } bounds;
    struct {
        char* title;
        int id;
    } metadata;
} ComplexShape;

// Union inside struct
typedef struct {
    union {
        int integer_value;
        float float_value;
        char* string_value;
    } data;
    int type;
} VariantData;

// Struct inside union
typedef struct {
    union {
        struct {
            int x, y;
        } point;
        struct {
            int width, height;
        } size;
        struct {
            char* name;
            int id;
        } info;
    } content;
    char tag;
} FlexibleData;

// Deep nesting with multiple anonymous structs
typedef struct {
    struct {
        struct {
            struct {
                int value;
                char* description;
            } inner;
            float factor;
        } middle;
        Point2D position;
    } outer;
    int count;
} DeepNested;

// Union with nested structs
typedef struct {
    union {
        struct {
            int x, y;
            char* label;
        } labeled_point;
        struct {
            float width, height;
            int color;
        } rectangle;
        struct {
            char* text;
            int font_size;
            int style;
        } text_info;
    } shape_data;
    int shape_type;
} ShapeUnion;

// Multiple unions in a struct
typedef struct {
    union {
        int i;
        float f;
        char* s;
    } primary;
    union {
        Point2D p2d;
        Point3D p3d;
    } position;
    union {
        struct {
            int r, g, b;
        } rgb;
        struct {
            float h, s, v;
        } hsv;
    } color;
} MultiUnionStruct;

// Anonymous structs and unions mixed
typedef struct {
    struct {
        union {
            int x;
            float fx;
        } coord_x;
        union {
            int y;
            float fy;
        } coord_y;
    } coordinates;
    struct {
        char* name;
        int id;
    } info;
} MixedAnonymous;

// Function pointer with nested struct parameter
typedef struct {
    int x, y;
    char* name;
} CallbackData;

typedef struct {
    int status;
    char* message;
} CallbackResult;

typedef int (*ComplexCallback)(CallbackData* data, CallbackResult result);

// Variadic function with nested struct
typedef struct {
    int count;
    char* format;
} LogConfig;

int log_message(LogConfig* config, ...);

// Nested enums
typedef enum {
    SHAPE_POINT = 0,
    SHAPE_LINE = 1,
    SHAPE_RECTANGLE = 2,
    SHAPE_CIRCLE = 3
} ShapeType;

typedef struct {
    ShapeType type;
    union {
        struct {
            Point2D start, end;
        } line;
        struct {
            Point2D center;
            float radius;
        } circle;
        struct {
            Point2D top_left;
            Point2D bottom_right;
        } rectangle;
    } data;
} Shape;

// Complex nested structure with arrays
typedef struct {
    struct {
        int count;
        struct {
            char* name;
            int value;
        } items[10];
    } container;
    struct {
        float matrix[3][3];
        Point3D origin;
    } transform;
} ArrayStruct;

// Function declarations for testing
void process_shape(Shape* shape);
void handle_variant(VariantData* variant);
int calculate_area(Rectangle* rect);
void transform_point(Point3D* point, float matrix[3][3]);
ComplexCallback register_callback(ComplexCallback callback);

#endif // NESTED_TEST_H 