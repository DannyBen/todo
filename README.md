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
todo edit ID[/ID...] [TEXT...]
todo del ID[/ID...]|+TAG
```

`todo new`, `todo ls`, and `todo rm` are aliases for `todo add`, `todo list`,
and `todo del` respectively.

Filters are combined with AND. Plain words are case-insensitive description
substrings. A number selects a task by ID, and slash-separated IDs such as `7/1`
select either task. Slash-separated positive tags such as `+low/+maybe` require
either tag. Separate filters remain AND, so `+low/+maybe -defer` means
`(low OR maybe) AND NOT defer`. `-tag` excludes a tag, `+12` requires a
connection, and `-12` excludes it. The `=` prefix is reserved for future use.

Editing with ordinary text replaces the description. `+tag` and `-tag` add and
remove tags without changing the description; `+12` and `-12` do the same for
connections. Connections are symmetric and can be removed from either task.
These operations mean the same thing for every command, so a
removal during `todo add` is accepted and has no effect. Prefixes are recognized
only when the entire token is valid syntax: `+tag` adds a tag, while `+ tag` is
ordinary text. After the sign, an all-digit value is a connection; otherwise a
tag must begin with a letter or digit and may also contain hyphens or underscores.
Invalid operations and standalone `+`, `-`, or `--` remain ordinary description
text. The `@` character has no special meaning.

Running `todo edit ID` without text opens the task expression in `$EDITOR`, or
`vi` when it is unset. Saving applies exactly the same syntax as a direct edit.
Newlines and other whitespace are normalized to spaces so tasks remain one line.
Slash-separated IDs apply one non-interactive edit to every selected task in a
single transaction.

Deleting prints every removed task so it can be recreated if needed. Passing an
exact positive tag filter deletes all matching tasks atomically; for example,
`todo del +done`. Slash-separated IDs are also deleted atomically. Use
`todo list +done` or `todo list 7/1` to preview the affected tasks.

Tasks are stored in `.todo.sqlite` at the nearest Git root. Set `TODO_DB_FILE`
to use an exact database path instead.

In a terminal, task IDs are bold yellow, tags are bold blue, and connections are
bold magenta. Redirected output remains plain text, and setting `NO_COLOR`
disables color explicitly. Long terminal task output wraps to the available
width with continuation lines aligned after the widest displayed task ID;
redirected output remains one task per line.
