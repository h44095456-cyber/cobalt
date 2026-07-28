package jit

import (
	"bytes"
	"cobalt/pkg/llvm"
	"cobalt/pkg/resolver"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Engine manages in-memory LLVM JIT compilation and instant execution.
type Engine struct{}

// New creates a new JIT Engine instance.
func New() *Engine {
	return &Engine{}
}

// ExecuteIR runs an LLVM IR string instantly using LLVM's ORC JIT engine (lli).
func (e *Engine) ExecuteIR(irCode string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "cobalt_jit_*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory for JIT: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	llPath := filepath.Join(tmpDir, "jit_module.ll")
	if err := os.WriteFile(llPath, []byte(irCode), 0644); err != nil {
		return "", fmt.Errorf("failed to write LLVM IR for JIT: %w", err)
	}

	// 1. Try lli (LLVM Execution Engine / ORC JIT Compiler)
	if lliPath, err := exec.LookPath("lli"); err == nil {
		cmd := exec.Command(lliPath, "-O3", llPath)
		cmd.Stdin = os.Stdin
		var stdoutBuf, stderrBuf bytes.Buffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		if err := cmd.Run(); err == nil {
			return stdoutBuf.String(), nil
		}
	}

	// 2. Fallback: Fast clang in-memory execution if lli is not present or failed
	binPath := filepath.Join(tmpDir, "jit_bin")
	clangCmd := exec.Command("clang", "-O0", "-o", binPath, llPath)
	if out, err := clangCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("JIT fallback compilation error: %v\nOutput: %s", err, string(out))
	}

	runCmd := exec.Command(binPath)
	runCmd.Stdin = os.Stdin
	var stdoutBuf, stderrBuf bytes.Buffer
	runCmd.Stdout = &stdoutBuf
	runCmd.Stderr = &stderrBuf

	if err := runCmd.Run(); err != nil {
		return stdoutBuf.String(), fmt.Errorf("JIT execution runtime error: %v\nStderr: %s", err, stderrBuf.String())
	}

	return stdoutBuf.String(), nil
}

// ExecuteFile resolves Cobalt modules, generates LLVM IR, and executes via JIT engine.
func (e *Engine) ExecuteFile(filePath string) (time.Duration, error) {
	startTime := time.Now()

	modResolver := resolver.New()
	prog, err := modResolver.ResolveProgram(filePath)
	if err != nil {
		return 0, fmt.Errorf("module resolution error: %w", err)
	}

	gen := llvm.New()
	irCode, err := gen.Generate(prog)
	if err != nil {
		return 0, fmt.Errorf("LLVM IR generation error: %w", err)
	}

	jitTime := time.Since(startTime)

	out, err := e.ExecuteIR(irCode)
	if err != nil {
		return jitTime, err
	}

	fmt.Print(out)
	return jitTime, nil
}
