# ⚡ Cobalt Programming Language Ecosystem (v1.9.0)

[![Go Report Card](https://goreportcard.com/badge/github.com/h44095456-cyber/cobalt)](https://goreportcard.com/report/github.com/h44095456-cyber/cobalt)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Version](https://img.shields.io/badge/version-1.9.0-brightgreen.svg)](https://github.com/h44095456-cyber/cobalt)
[![Build Status](https://img.shields.io/badge/build-passing-success.svg)](https://github.com/h44095456-cyber/cobalt)

**Cobalt** is a next-generation systems programming language designed for ultra-high performance, mathematical safety guarantees, and peak developer ergonomics. It combines Python-like readable syntax with bare-metal native compilation, deterministic memory management (zero garbage collector pauses), and a production-grade compiler pipeline powered by C++20 and LLVM IR.

---

## 🌟 Key Architectural Features

- **⚡ Dual-Backend Parity (C++20 & LLVM IR)**: 100% execution output equality across transpiled C++20 (`g++ -O3`) and pure native LLVM IR (`clang -O3`).
- **🚀 Instantaneous LLVM ORC JIT Engine (`cobalt jit`)**: Executes scripts instantly in memory with **8.09x faster startup latency** than traditional AOT compilation loops.
- **🔬 Z3 SMT-LIB2 Theorem Prover (`cobalt verify`)**: Mathematically proves function pre-conditions (`@requires`) and post-conditions (`@ensures`) via First-Order Logic SMT formulas.
- **⚙️ SSA Control-Flow Graph & Alias Analysis (`cobalt cfg`)**: Lowers code into SSA BasicBlocks with explicit `phi` nodes and Andersen pointer alias analysis (`MustAlias`, `MayAlias`, `NoAlias`).
- **🧠 Bi-Directional Hindley-Milner Type Inference (`cobalt infer`)**: Automatically deduces function signatures and principal type schemas without requiring verbose type annotations.
- **📦 Direct LLVM Bitcode Emitter (`cobalt emit -backend=bc`)**: Assembles binary `.bc` LLVM Bitcode directly for sub-millisecond build pipelines.
- **⚡ Ultra-Low Latency Arena Allocators (`std::alloc`)**: O(1) region memory allocation and instantaneous resets for high-frequency trading (HFT) and game loops.
- **🌐 Native Web Framework (`std::web`) & GPU Canvas (`std::gui`)**: Multi-threaded HTTP router, JSON middleware, and GPU-accelerated canvas graphics.
- **♻️ 100% Cobalt Self-Hosting Bootstrap (`cobalt self-host`)**: Compiles the Cobalt compiler completely in 100% Cobalt code.

---

## 🛠️ Installation & Building

```bash
# Clone the repository
git clone https://github.com/h44095456-cyber/cobalt.git
cd cobalt

# Build static release binary
go build -o cobalt ./cmd/cobalt

# Verify installation
./cobalt --version
```

---

## 🚀 CLI Command Reference

### 1. Dual-Backend Execution & JIT
```bash
# Execute via C++20 native backend
./cobalt run -backend=cpp examples/async_demo.cb

# Execute via LLVM IR native backend
./cobalt run -backend=llvm examples/async_demo.cb

# Instantaneous LLVM JIT execution
./cobalt jit examples/simple.cb
```

### 2. Deep Compiler Subsystems & Formal Verification
```bash
# Z3 SMT Contract Theorem Prover
./cobalt verify examples/smt_contract_demo.cb

# SSA Control-Flow Graph & Pointer Alias Analysis
./cobalt cfg examples/advanced_roadmap_demo.cb

# Hindley-Milner Type Inference Engine
./cobalt infer examples/advanced_roadmap_demo.cb

# Direct LLVM Bitcode Binary Emission
./cobalt emit -backend=bc examples/simple.cb
```

### 3. Developer Infrastructure & Package Management
```bash
# Format and lint Cobalt code
./cobalt fmt examples/simple.cb

# Generate C/C++ Interface Header File (.h)
./cobalt header examples/math_utils.cb

# Launch Package Registry Web Dashboard
./cobalt registry --web

# Launch Interactive JIT REPL
./cobalt repl --jit

# Self-Hosting Bootstrap Execution
./cobalt self-host
```

---

## 📦 Standard Library Overview

| Module | Description |
| :--- | :--- |
| **`std/web.cb`** | High-performance web framework with HTTP routing & JSON middleware |
| **`std/alloc.cb`** | Region-based Arena and O(1) Bump Memory Allocators for low-latency tasks |
| **`std/gui.cb`** | GPU-accelerated Canvas graphics and interactive WebAssembly UI toolkit |
| **`std/http.cb`** | Multi-threaded HTTP/1.1 web server and client engine |
| **`std/json.cb`** | Recursive JSON parser, validator, and pretty-formatter |
| **`std/crypto.cb`** | Cryptographic hashing (SHA-256, HMAC, Base64) |
| **`std/path.cb`** | Cross-platform filesystem path manipulation |

---

## 📄 License

Cobalt is open-source software licensed under the **[MIT License](LICENSE)**.
