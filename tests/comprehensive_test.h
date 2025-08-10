// Comprehensive test header for naturebindgen

// Constants and macros
#define MAX_SIZE 100
#define PI 3.14159
#define STRING_CONST "Hello World"
#define STRUCT_MACRO (Point){10, 20}
#define FUNCTION_MACRO(x) ((x) * 2)

// Anonymous structs and unions
typedef struct {
    int x;
    int y;
} Point;

typedef union {
    int i;
    float f;
    char c;
} Value;

// Nested anonymous structs
typedef struct {
    struct {
        int a;
        int b;
    } nested;
    int outer;
} Container;

// Function pointer typedefs
typedef int (*CallbackFunc)(int, char*);
typedef void (*VoidFunc)(void);

// Regular structs
struct Rectangle {
    Point topLeft;
    Point bottomRight;
    int area;
};

// Enums
enum Color {
    RED = 0,
    GREEN = 1,
    BLUE = 2
};

// Functions
int add(int a, int b);
float multiply(float x, float y);
void printPoint(Point p);
int processArray(int arr[], int size);
void variadicFunc(int count, ...);

// Constants
const int DEFAULT_WIDTH = 800;
const int DEFAULT_HEIGHT = 600;
const char* DEFAULT_TITLE = "Window";

// Complex macros
#define CREATE_POINT(x, y) (Point){x, y}
#define MAX(a, b) ((a) > (b) ? (a) : (b))
#define ARRAY_SIZE(arr) (sizeof(arr) / sizeof(arr[0])) 