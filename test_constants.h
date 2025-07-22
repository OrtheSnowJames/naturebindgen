#ifndef TEST_CONSTANTS_H
#define TEST_CONSTANTS_H

// Constants with dependencies
#define FIVE 5
#define SIX (FIVE + 1)
#define SEVEN (SIX + 1)
#define TEN (FIVE * 2)
#define TWENTY (TEN * 2)

// Some constants without dependencies
#define MAX_SIZE 100
#define MIN_SIZE 1

// More complex dependencies
#define HALF_MAX (MAX_SIZE / 2)
#define QUARTER_MAX (HALF_MAX / 2)

#endif // TEST_CONSTANTS_H 