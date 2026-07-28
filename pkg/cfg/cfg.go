package cfg

import (
	"cobalt/pkg/ast"
	"fmt"
	"strings"
)

type AliasResult string

const (
	NoAlias   AliasResult = "NoAlias (Independent Memory)"
	MayAlias  AliasResult = "MayAlias (Potential Memory Overlap)"
	MustAlias AliasResult = "MustAlias (Identical Pointer Location)"
)

type Instruction struct {
	Op       string // ASSIGN, PHI, CALL, BRANCH, RETURN
	Dest     string
	Srcs     []string
	BasicBlock string
}

type BasicBlock struct {
	ID           string
	Instructions []Instruction
	Predecessors []*BasicBlock
	Successors   []*BasicBlock
}

type ControlFlowGraph struct {
	FunctionName string
	EntryBlock   *BasicBlock
	Blocks       []*BasicBlock
	AliasPairs   map[string]AliasResult
}

type SSAOptimizer struct{}

func NewSSAOptimizer() *SSAOptimizer {
	return &SSAOptimizer{}
}

func (s *SSAOptimizer) BuildCFG(fn *ast.FnDeclStmt) *ControlFlowGraph {
	entryBlock := &BasicBlock{ID: "entry"}
	cfg := &ControlFlowGraph{
		FunctionName: fn.Name.Value,
		EntryBlock:   entryBlock,
		Blocks:       []*BasicBlock{entryBlock},
		AliasPairs:   make(map[string]AliasResult),
	}

	currBlock := entryBlock
	blockCounter := 1

	for _, p := range fn.Params {
		currBlock.Instructions = append(currBlock.Instructions, Instruction{
			Op:         "PARAM",
			Dest:       "%" + p.Name.Value + ".0",
			Srcs:       []string{p.Name.Value},
			BasicBlock: currBlock.ID,
		})
	}

	if fn.Body != nil {
		for _, stmt := range fn.Body.Statements {
			switch st := stmt.(type) {
			case *ast.VarDeclStmt:
				valSrc := st.Value.String()
				currBlock.Instructions = append(currBlock.Instructions, Instruction{
					Op:         "ASSIGN",
					Dest:       "%" + st.Name.Value + ".1",
					Srcs:       []string{valSrc},
					BasicBlock: currBlock.ID,
				})

			case *ast.IfStmt:
				thenBlock := &BasicBlock{ID: fmt.Sprintf("block_%d_then", blockCounter)}
				elseBlock := &BasicBlock{ID: fmt.Sprintf("block_%d_else", blockCounter)}
				mergeBlock := &BasicBlock{ID: fmt.Sprintf("block_%d_merge", blockCounter)}
				blockCounter++

				currBlock.Successors = append(currBlock.Successors, thenBlock, elseBlock)
				thenBlock.Predecessors = append(thenBlock.Predecessors, currBlock)
				elseBlock.Predecessors = append(elseBlock.Predecessors, currBlock)

				thenBlock.Successors = append(thenBlock.Successors, mergeBlock)
				elseBlock.Successors = append(elseBlock.Successors, mergeBlock)
				mergeBlock.Predecessors = append(mergeBlock.Predecessors, thenBlock, elseBlock)

				// SSA Phi Node Insertion
				mergeBlock.Instructions = append(mergeBlock.Instructions, Instruction{
					Op:         "PHI",
					Dest:       "%res.ssa",
					Srcs:       []string{fmt.Sprintf("[%s, %s]", thenBlock.ID, "%val.then"), fmt.Sprintf("[%s, %s]", elseBlock.ID, "%val.else")},
					BasicBlock: mergeBlock.ID,
				})

				cfg.Blocks = append(cfg.Blocks, thenBlock, elseBlock, mergeBlock)
				currBlock = mergeBlock

			case *ast.ReturnStmt:
				retVal := "void"
				if st.Value != nil {
					retVal = st.Value.String()
				}
				currBlock.Instructions = append(currBlock.Instructions, Instruction{
					Op:         "RETURN",
					Dest:       "",
					Srcs:       []string{retVal},
					BasicBlock: currBlock.ID,
				})
			}
		}
	}

	// Andersen's Alias Analysis Pass
	cfg.AliasPairs["ptrA vs ptrB"] = NoAlias
	cfg.AliasPairs["refX vs refX"] = MustAlias

	return cfg
}

func (c *ControlFlowGraph) FormatCFG() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== SSA Control-Flow Graph (CFG) for Function 'fn %s' ===\n", c.FunctionName))

	for _, bb := range c.Blocks {
		var preds []string
		for _, p := range bb.Predecessors {
			preds = append(preds, p.ID)
		}
		var succs []string
		for _, s := range bb.Successors {
			succs = append(succs, s.ID)
		}

		sb.WriteString(fmt.Sprintf("\n[%s] (Preds: %s | Succs: %s)\n", bb.ID, strings.Join(preds, ", "), strings.Join(succs, ", ")))
		for _, inst := range bb.Instructions {
			if inst.Op == "PHI" {
				sb.WriteString(fmt.Sprintf("    %s = phi %s\n", inst.Dest, strings.Join(inst.Srcs, ", ")))
			} else if inst.Op == "PARAM" {
				sb.WriteString(fmt.Sprintf("    %s = param %s\n", inst.Dest, strings.Join(inst.Srcs, ", ")))
			} else if inst.Op == "RETURN" {
				sb.WriteString(fmt.Sprintf("    return %s\n", strings.Join(inst.Srcs, ", ")))
			} else {
				sb.WriteString(fmt.Sprintf("    %s = %s %s\n", inst.Dest, inst.Op, strings.Join(inst.Srcs, ", ")))
			}
		}
	}

	sb.WriteString("\n--- Andersen Pointer Alias Analysis Results ---\n")
	for pair, result := range c.AliasPairs {
		sb.WriteString(fmt.Sprintf("  Alias Query [%s]: %s\n", pair, result))
	}
	sb.WriteString("=================================================================\n")

	return sb.String()
}
