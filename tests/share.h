#pragma once
#include <stdio.h>
#include "othershare.h"

typedef struct{
    int id;
    char name[100];
    int age;
    char team[100];
} Player;

const int Color_RED = 0;
const int Color_BLUE = 1;
const int Color_GREEN = 2;

inline void print_player(Player *player) {
    printf("Player: %s, Age: %d, Team: %s\n", player->name, player->age, player->team);
}

typedef enum{
    RED = Color_RED,
    BLUE = Color_BLUE,
    GREEN = Color_GREEN,
} Color;

void print_color(Color color) {
    switch (color) {
        case RED:
            printf("Red\n");
            break;
        case BLUE:
            printf("Blue\n");
            break;
        case GREEN:
            printf("Green\n");
            break;
    }
}
// this is a comment
// this is another comment
// this is a third comment
// this is a fourth comment
// this is a fifth comment
// this is a sixth comment
// this is a seventh comment
// this is a eighth comment