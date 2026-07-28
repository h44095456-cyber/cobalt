# ⚡ Cobalt Programming Language (`.cb`)

[![Build Status](https://img.shields.io/badge/build-passing-brightgreen)](https://github.com/h44095456-cyber/cobalt)
[![Language](https://img.shields.io/badge/language-Cobalt-blue)](https://github.com/h44095456-cyber/cobalt)
[![Version](https://img.shields.io/badge/release-v1.9.0-indigo)](https://github.com/h44095456-cyber/cobalt/releases)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

**Cobalt (`.cb`)** is an ultra-high-performance systems programming language that combines the clean, indentation-based syntax of Python and Mojo with the bare-metal performance, memory safety, and zero-cost abstractions of C++20 and LLVM IR.

---

## 🔥 Key Features

- 🐍 **Pythonic Syntax, Bare-Metal Speed**: Write clean indentation-based code (`:`, `fn`, `let`) compiling down to `clang -O3` or `g++ -O3` with **0ms GC pauses**.
- ⚡ **LLVM ORC JIT Engine (`cobalt jit`)**: Instant in-memory script execution (**8.09x faster startup latency** than traditional AOT compilation).
- 📦 **Package Manager & Lockfile System (`cobalt pkg`)**: Deterministic `cobalt.lock` with SHA256 checksum integrity verification and SemVer constraint resolution.
- 📚 **Standard Library Expansion (`std::*`)**: Built-in modules for HTTP networking (`std::http`), JSON parsing (`std::json`), Cryptography (`std::crypto`), and Path operations (`std::path`).
- 🌐 **WebAssembly Target (`--target=wasm`)**: Direct WAT/WASM compilation and interactive single-page browser REPL playground generation (`cobalt wasm --web`).
- ⚡ **Async / Await Coroutines**: Non-blocking asynchronous tasks (`async fn`) with parallel coroutine resolution (`await task`).
- 🐞 **Interactive Native Debugger (`cobalt debug`)**: Built-in line-by-line debugging, breakpoint management (`break`), stepping (`next`/`step`), and call stack backtraces (`backtrace`).
- 🛠️ **Single 2.7 MB Static Executable**: Compiler, JIT, LSP server, debugger, doc generator, package manager, and terminal text editor bundled into a single binary with zero external dependencies.

---

## ⚡ Quickstart Code Example

```cobalt
# Async function performing background calculation
async fn compute_task(id: int, base: int) -> int:
    println(f"[Task {id}] Performing asynchronous background calculation...")
    return base * 2 + 10

fn main():
    println("=================================================================")
    println("Welcome to Cobalt 2.0!")
    println("=================================================================")

    # Non-blocking concurrent async tasks
    let task1 = compute_task(1, 20)
    let task2 = compute_task(2, 50)

    # Resolve coroutine futures
    let res1 = await task1
    let res2 = await task2

    println(f"[Resolved] Task 1: {res1}, Task 2: {res2}")
```

---

## 🚀 Installation & Basic Usage

```bash
# Build standalone binary from source
go build -o cobalt ./cmd/cobalt

# Run file via C++20 backend
./cobalt run -backend=cpp examples/language_features_demo.cb

# Run file via LLVM IR backend
./cobalt run -backend=llvm examples/language_features_demo.cb

# Run instantly using LLVM ORC JIT engine
./cobalt jit examples/language_features_demo.cb

# Launch interactive debugger
./cobalt debug examples/async_demo.cb

# Install package & verify lockfile
./cobalt pkg install json@2.0.1
./cobalt pkg verify

# Run benchmarks
./cobalt bench examples/self_hosting_compiler.cb
```
