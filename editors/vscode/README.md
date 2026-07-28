# Cobalt Language Support for VS Code

Official Visual Studio Code extension for the **Cobalt (`.cb`)** programming language.

## Features

- **Syntax Highlighting**: Rich TextMate grammar support for keywords (`fn`, `rpc`, `struct`, `impl`, `trait`), types (`Option`, `Result`, `Channel`), intrinsics (`type_name`, `field_names`), strings, numbers, and operators.
- **Auto-Indentation**: Indentation rules configured for Cobalt's Python-like block syntax (`:`).
- **Code Snippets**: Quick templates for `fn`, `rpc fn`, `struct`, `impl`, `match` with guards, and `main()`.
- **Language Configuration**: Bracket matching, comment toggling (`#`), and auto-closing quotes/brackets.

## Installation

1. Copy the `editors/vscode` directory to `~/.vscode/extensions/cobalt-lang-1.9.0`.
2. Reload VS Code.
3. Open any `.cb` file to enjoy native syntax highlighting!
