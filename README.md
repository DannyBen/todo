# Todo

A lightweight project todo list shared by humans and coding agents.

```console
$ todo add Prepare for deployment +production +now
1 Prepare for deployment +now +production
$ todo edit 1 -now +done
1 Prepare for deployment +done +production
$ todo list +done -blocked
1 Prepare for deployment +done +production
```

## Usage

```text
todo add TEXT...
todo list FILTER...
todo edit ID TEXT...
todo del ID
```

`todo ls` is an alias for `todo list`.

Filters are combined with AND. Plain words are case-insensitive description
substrings. A number selects a task by ID, `+tag` requires a tag, `-tag`
excludes it, `@12` requires a reference, and `-@12` excludes it. The `=` prefix
is reserved for future use.

Editing with ordinary text replaces the description. `+tag` and `-tag` add and
remove tags without changing the description; `@12` and `-@12` do the same for
references. These operations mean the same thing for every command, so a
removal during `todo add` is accepted and has no effect. A standalone `-` or
`--` is not valid removal syntax and remains ordinary description text.

Tasks are stored in `.todo.sqlite` at the nearest Git root. Set `TODO_DB_FILE`
to use an exact database path instead.

In a terminal, task IDs are yellow and tags are bold magenta. Redirected output
remains plain text, and setting `NO_COLOR` disables color explicitly. Long
terminal task output wraps to the available width with continuation lines
aligned after the widest displayed task ID; redirected output remains one task
per line.
