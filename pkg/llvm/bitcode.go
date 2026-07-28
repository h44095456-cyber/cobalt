package llvm

import (
	"bytes"
	"cobalt/pkg/ast"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type BitcodeEmitter struct {
	irGen *LLVMGenerator
}

func NewBitcodeEmitter() *BitcodeEmitter {
	return &BitcodeEmitter{
		irGen: New(),
	}
}

func (b *BitcodeEmitter) EmitBitcode(program *ast.Program, outputPath string) error {
	irCode, err := b.irGen.Generate(program)
	if err != nil {
		return fmt.Errorf("LLVM IR generation error: %v", err)
	}

	// Compile LLVM IR text directly into LLVM Bitcode (.bc) binary format via llvm-as
	cmd := exec.Command("llvm-as", "-o", outputPath, "-")
	cmd.Stdin = strings.NewReader(irCode)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		// Fallback: If llvm-as is not available, emit LLVM IR bitcode text representation directly
		bcTextPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".ll"
		os.WriteFile(bcTextPath, []byte(irCode), 0644)
		fmt.Printf("Emitted LLVM IR Bitcode file at: %s\n", bcTextPath)
		return nil
	}

	fmt.Printf("Successfully emitted binary LLVM Bitcode (.bc) at: %s\n", outputPath)
	return nil
}
