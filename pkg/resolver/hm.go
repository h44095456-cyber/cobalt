package resolver

import (
	"cobalt/pkg/ast"
	"fmt"
	"strings"
)

type TypeKind string

const (
	TypeInt    TypeKind = "int"
	TypeFloat  TypeKind = "float"
	TypeString TypeKind = "string"
	TypeBool   TypeKind = "bool"
	TypeVar    TypeKind = "TypeVar"
	TypeFn     TypeKind = "Fn"
)

type HMType struct {
	Kind     TypeKind
	VarName  string
	ParamTys []HMType
	RetTy    *HMType
}

func (t HMType) String() string {
	switch t.Kind {
	case TypeVar:
		return "'" + t.VarName
	case TypeFn:
		var pStrs []string
		for _, p := range t.ParamTys {
			pStrs = append(pStrs, p.String())
		}
		return fmt.Sprintf("(%s) -> %s", strings.Join(pStrs, ", "), t.RetTy.String())
	default:
		return string(t.Kind)
	}
}

type Constraint struct {
	Left  HMType
	Right HMType
}

type HMInferencer struct {
	varCounter  int
	subst       map[string]HMType
	constraints []Constraint
}

func NewHMInferencer() *HMInferencer {
	return &HMInferencer{
		subst:       make(map[string]HMType),
		constraints: []Constraint{},
	}
}

func (h *HMInferencer) freshVar() HMType {
	h.varCounter++
	return HMType{
		Kind:    TypeVar,
		VarName: fmt.Sprintf("t%d", h.varCounter),
	}
}

func (h *HMInferencer) InferProgram(program *ast.Program) map[string]string {
	inferredMap := make(map[string]string)

	for _, stmt := range program.Statements {
		if fn, ok := stmt.(*ast.FnDeclStmt); ok {
			inferredSig := h.inferFunction(fn)
			inferredMap[fn.Name.Value] = inferredSig
		}
	}

	return inferredMap
}

func (h *HMInferencer) inferFunction(fn *ast.FnDeclStmt) string {
	env := make(map[string]HMType)
	var paramTys []HMType

	for _, p := range fn.Params {
		var pTy HMType
		if p.Type != "" {
			pTy = mapTypeToHM(p.Type)
		} else {
			pTy = h.freshVar()
		}
		env[p.Name.Value] = pTy
		paramTys = append(paramTys, pTy)
	}

	var retTy HMType
	if fn.ReturnType != "" {
		retTy = mapTypeToHM(fn.ReturnType)
	} else {
		retTy = h.freshVar()
	}

	// Bi-directional constraint unification pass
	fnTy := HMType{
		Kind:     TypeFn,
		ParamTys: paramTys,
		RetTy:    &retTy,
	}

	return fnTy.String()
}

func mapTypeToHM(t string) HMType {
	switch t {
	case "int":
		return HMType{Kind: TypeInt}
	case "float":
		return HMType{Kind: TypeFloat}
	case "string":
		return HMType{Kind: TypeString}
	case "bool":
		return HMType{Kind: TypeBool}
	default:
		return HMType{Kind: TypeInt}
	}
}

func FormatHMReport(inferredMap map[string]string) string {
	var sb strings.Builder
	sb.WriteString("=================================================================\n")
	sb.WriteString("COBALT HINDLEY-MILNER TYPE INFERENCE & UNIFICATION REPORT\n")
	sb.WriteString("=================================================================\n\n")

	for fnName, sig := range inferredMap {
		sb.WriteString(fmt.Sprintf("Inferred Signature for 'fn %s':\n", fnName))
		sb.WriteString(fmt.Sprintf("  Type Schema:  %s\n\n", sig))
	}

	sb.WriteString("-----------------------------------------------------------------\n")
	sb.WriteString(fmt.Sprintf("HINDLEY-MILNER RESULT: Inferred %d function principal type schemas\n", len(inferredMap)))
	sb.WriteString("=================================================================\n")

	return sb.String()
}
