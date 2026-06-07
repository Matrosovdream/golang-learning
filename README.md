# golang-learning

My notes and practice for learning Go, aimed at building backend / REST API services.

## The lessons

26 lessons in [learning-plan/](learning-plan/), numbered 01 to 26. They run from the basics
(types, control flow, functions) through interfaces, concurrency, testing, and end with a small
REST API. Each lesson is one markdown file with notes and exercises. Most lessons also have a set
of small runnable examples under [learning-plan/examples/](learning-plan/examples/), graded easy →
medium → hard.

Where I'm at is tracked in [learning-plan/PROGRESS.md](learning-plan/PROGRESS.md).

## How to use it

1. Open the next lesson, e.g. [learning-plan/04-types-constants.md](learning-plan/04-types-constants.md).
2. Read it, then retype the examples into a scratch folder and run them:
   ```bash
   mkdir -p /tmp/go-ex && cd /tmp/go-ex
   go mod init scratch        # first time only
   # paste an example into main.go, then:
   go run .
   ```
3. Write your own answers to the exercises in [practice/](practice/) — one folder per lesson.
4. Update PROGRESS.md when a lesson is done.

## Adding more practice examples

The examples live in `learning-plan/examples/<lesson>/`, split across `1-easy.md`, `2-medium.md`,
and `3-hard.md`. To add more, just keep the numbering going in the matching tier file and add the
entry to that lesson's `README.md` index. Each example is a full `package main` program with its
real output underneath — run it before adding it.
