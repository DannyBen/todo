# Todo

![repocard](https://repocard.dannyben.com/svg/todo.svg)

A lightweight project todo list shared by humans and coding agents.

Todo keeps a compact action queue at the root of each project. Its four commands
share one expression syntax for adding, finding, editing, and deleting tasks.

```console
$ todo add Prepare for deployment +production +now
1 Prepare for deployment +now +production

$ todo edit 1 -now +done
1 Prepare for deployment +done +production

$ todo list +done -blocked
1 Prepare for deployment +done +production
```

Run `todo help` for the complete syntax and copy-ready examples.

## Install

With [eget](https://github.com/zyedidia/eget):

```bash
eget dannyben/todo
```

Or install from source:

```bash
go install github.com/dannyben/todo@latest
```

Prebuilt archives for macOS and Linux are available on the repository's
[Releases page](https://github.com/DannyBen/todo/releases).

## Why Todo?

- **Obsessively simple.** Four commands, no flags, ordinary task descriptions,
  and single-line tasks instead of stories or comment threads. Connect related
  tasks instead.
- **Agent-friendly.** No skill or MCP server is required. The compact,
  searchable output is token-efficient, and `todo help` contains everything a
  human or agent needs to operate it.
- **Ephemeral by design.** The task list is an action queue, not a historical
  record. Deleted tasks are truly removed, not archived.
- **SQLite by choice.** The non-textual database discourages manual edits and
  accidental commits. Use the CLI as the shared interface.
- **Your conventions.** Todo defines no priorities or workflow. Choose tags such
  as `+high`, `+low`, `+p1`, `+now`, and `+done`, then teach your agent the same
  vocabulary.

## Usage

```
todo add TEXT...
todo list FILTER...
todo edit ID [TEXT...]
todo del FILTER...
todo help
```

### Adding tasks

Add tasks with plain text and optional tags

<img src="support/vhs/add.gif" width="500">

### Editing tasks

Edit task text, tags, or both directly from the command line.

<img src="support/vhs/edit.gif" width="500">

Run `todo edit ID` without text to edit the complete task interactively.

<img src="support/vhs/edit-interactive.gif" width="500">

### Listing and filtering tasks

Filter by text, ID, or tag, with AND, OR, and exclusions

<img src="support/vhs/list.gif" width="500">

### Deleting tasks

Delete by ID or tag, with OR for multiple filters

<img src="support/vhs/del.gif" width="500">

## Contributing / Support

If you experience any issue, have a question or a suggestion, or if you wish to
contribute, feel free to [open an issue](https://github.com/DannyBen/todo/issues).
