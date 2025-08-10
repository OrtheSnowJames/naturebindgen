# Naturebindgen

A binding generator for the [nature](https://github.com/nature-lang/nature) language.

## Why python?

I chose python for this language for object oriented programming with official clang bindings. I did not know at the time of choosing that libclang had oop in C++.
It was too late, though.

## Why to nature?
a
When I first saw nature, I thought it was one of the best languages ever. It's like go, but with the features go was too scared to implement. So, I made a binding generator. I thought it would be easy, but no. There are about 50 million edge cases of the C Programming Language that I had to handle. So I've worked on it for a few weeks, and it's looking pretty good! The nature programming language isn't really fully fleshed out, though, so I made my own unions and anonymous struct handling.

## What does it handle?

The nature binding generator handles constants, structs, unions and functions. It does not handle C++ code.

## Setup

1. Clone the repo

```sh
git clone https://github.com/OrtheSnowJames/naturebindgen
```

2. Just run the python code!

```sh
python3 main.py <header_file>
```

## Footnote

This binding generator isn't fully completed yet, and I still haven't ran it on every test in the tests folder. I think it'll be good experience for me though!