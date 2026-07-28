package llvm

import (
	"bytes"
	"cobalt/pkg/ast"
	"fmt"
	"strings"
)

type LLVMGenerator struct {
	TargetTriple    string
	buf             bytes.Buffer
	regCounter      int
	labelCounter    int
	strCounter      int
	globalStrings   map[string]string // lit -> global var name
	globalStringLens map[string]int   // varName -> byte len
	symbolTable     map[string]string // var name -> type ("i64", "double", "i1", "i8*")
	varAllocaMap    map[string]string // var name -> LLVM pointer register (%x.addr)
	structTypes     map[string][]ast.StructField
	methodReceivers map[string]string // methodName -> structType ("area" -> "Rectangle")
	fnReturnTypes   map[string]string // fnName -> LLVM retType ("%struct.Option", "i64", etc.)
	fnParamTypes    map[string][]string
	tupleTypes      map[string][]string
	genericFns      map[string]*ast.FnDeclStmt
	fnDeclStmts     map[string]*ast.FnDeclStmt
	monoFns         map[string]bool
	currentFn       string
	currentLoopCond string
	currentLoopEnd  string
	deferredStmts   []ast.Expression
	topLevelBuf     bytes.Buffer
}

func New() *LLVMGenerator {
	return &LLVMGenerator{
		globalStrings:   make(map[string]string),
		globalStringLens: make(map[string]int),
		symbolTable:     make(map[string]string),
		varAllocaMap:    make(map[string]string),
		structTypes:     make(map[string][]ast.StructField),
		methodReceivers: make(map[string]string),
		fnReturnTypes:   make(map[string]string),
		fnParamTypes:    make(map[string][]string),
		tupleTypes:      make(map[string][]string),
		genericFns:      make(map[string]*ast.FnDeclStmt),
		fnDeclStmts:     make(map[string]*ast.FnDeclStmt),
		monoFns:         make(map[string]bool),
	}
}

func (g *LLVMGenerator) freshReg() string {
	g.regCounter++
	return fmt.Sprintf("%%r%d", g.regCounter)
}

func (g *LLVMGenerator) freshLabel(prefix string) string {
	g.labelCounter++
	return fmt.Sprintf("%s.%d", prefix, g.labelCounter)
}

func (g *LLVMGenerator) getGlobalString(lit string) string {
	if name, ok := g.globalStrings[lit]; ok {
		return name
	}
	g.strCounter++
	name := fmt.Sprintf("@.str.%d", g.strCounter)
	g.globalStrings[lit] = name
	g.globalStringLens[name] = len(lit) + 1
	return name
}

func (g *LLVMGenerator) Generate(program *ast.Program) (string, error) {
	g.buf.Reset()
	g.topLevelBuf.Reset()
	g.regCounter = 0
	g.labelCounter = 0
	g.strCounter = 0
	g.globalStrings = make(map[string]string)
	g.globalStringLens = make(map[string]int)
	g.methodReceivers = make(map[string]string)
	g.symbolTable["Some"] = "%struct.Option"
	g.symbolTable["None"] = "%struct.Option"
	g.symbolTable["Ok"] = "%struct.Result"
	g.symbolTable["Err"] = "%struct.Result"
	g.fnReturnTypes = make(map[string]string)
	g.tupleTypes = make(map[string][]string)
	g.genericFns = make(map[string]*ast.FnDeclStmt)
	g.monoFns = make(map[string]bool)

	var codeBuf bytes.Buffer

	// First pass: collect structs, method receivers, generic functions, and function return types
	for _, stmt := range program.Statements {
		if st, ok := stmt.(*ast.StructDeclStmt); ok {
			g.structTypes[st.Name.Value] = st.Fields
		}
		if fn, ok := stmt.(*ast.FnDeclStmt); ok {
			g.fnDeclStmts[fn.Name.Value] = fn
			var pTypes []string
			for _, p := range fn.Params {
				pTypes = append(pTypes, mapLLVMType(p.Type))
			}
			g.fnParamTypes[fn.Name.Value] = pTypes

			if len(fn.TypeParams) > 0 {
				g.genericFns[fn.Name.Value] = fn
			} else {
				retType := mapLLVMType(fn.ReturnType)
				fnName := fn.Name.Value
				if fn.Receiver != nil {
					fnName = fn.Receiver.Type + "_" + fn.Name.Value
					g.methodReceivers[fn.Name.Value] = fn.Receiver.Type
				} else if len(fn.Params) > 0 && fn.Params[0].Name.Value == "self" {
					fnName = fn.Params[0].Type + "_" + fn.Name.Value
					g.methodReceivers[fn.Name.Value] = fn.Params[0].Type
				}
				g.fnReturnTypes[fnName] = retType
			}
		}
		if imp, ok := stmt.(*ast.ImplDeclStmt); ok {
			for _, m := range imp.Methods {
				mName := imp.TargetType + "_" + m.Name.Value
				g.methodReceivers[m.Name.Value] = imp.TargetType
				g.fnReturnTypes[mName] = mapLLVMType(m.ReturnType)
			}
		}
		if ext, ok := stmt.(*ast.ExternFnStmt); ok {
			g.fnReturnTypes[ext.Name.Value] = mapLLVMType(ext.ReturnType)
			var pTypes []string
			for _, p := range ext.Params {
				pTypes = append(pTypes, mapLLVMType(p.Type))
			}
			g.fnParamTypes[ext.Name.Value] = pTypes
		}
	}

	// Generate statements (functions & structs)
	for _, stmt := range program.Statements {
		if err := g.generateStatement(&codeBuf, stmt); err != nil {
			return "", err
		}
	}

	targetTriple := "x86_64-pc-linux-gnu"
	if g.TargetTriple != "" {
		targetTriple = g.TargetTriple
	}
	g.buf.WriteString(fmt.Sprintf("target triple = \"%s\"\n\n", targetTriple))

	// Array struct type: { i64* data, i64 len, i64 cap }
	g.buf.WriteString("%struct.Array = type { i64*, i64, i64 }\n")
	g.buf.WriteString("%struct.Option = type { i1, i64 }\n")
	g.buf.WriteString("%struct.Result = type { i1, i64, i8* }\n\n")

	// Emit dynamic tuple types
	for tName, fieldTypes := range g.tupleTypes {
		g.buf.WriteString(fmt.Sprintf("%%%s = type { %s }\n", tName, strings.Join(fieldTypes, ", ")))
	}
	if len(g.tupleTypes) > 0 {
		g.buf.WriteString("\n")
	}

	// Emit struct type declarations
	for sName, fields := range g.structTypes {
		var fieldTypes []string
		for _, f := range fields {
			fieldTypes = append(fieldTypes, mapLLVMType(f.Type))
		}
		g.buf.WriteString(fmt.Sprintf("%%struct.%s = type { %s }\n", sName, strings.Join(fieldTypes, ", ")))
	}
	if len(g.structTypes) > 0 {
		g.buf.WriteString("\n")
	}

	// External C declarations for print helpers
	g.buf.WriteString("declare i32 @printf(i8*, ...)\n")
	g.buf.WriteString("declare i32 @puts(i8*)\n")
	g.buf.WriteString("declare i32 @sprintf(i8*, i8*, ...)\n")
	g.buf.WriteString("declare i32 @fflush(i8*)\n")
	g.buf.WriteString("declare i64 @strlen(i8*)\n")
	g.buf.WriteString("declare i32 @strcmp(i8*, i8*)\n")
	g.buf.WriteString("declare i8* @strcpy(i8*, i8*)\n")
	g.buf.WriteString("declare i8* @strncpy(i8*, i8*, i64)\n")
	g.buf.WriteString("declare i8* @strcat(i8*, i8*)\n")
	g.buf.WriteString("declare i8* @malloc(i64)\n")
	g.buf.WriteString("declare i8* @realloc(i8*, i64)\n")
	g.buf.WriteString("declare void @free(i8*)\n")
	g.buf.WriteString("declare i64 @time(i8*)\n")
	g.buf.WriteString("declare i8* @fopen(i8*, i8*)\n")
	g.buf.WriteString("declare i32 @fputs(i8*, i8*)\n")
	g.buf.WriteString("declare i32 @fclose(i8*)\n")
	g.buf.WriteString("declare i64 @fread(i8*, i64, i64, i8*)\n\n")

	g.buf.WriteString("@.mode_w = private unnamed_addr constant [2 x i8] c\"w\\00\"\n")
	g.buf.WriteString("@.mode_r = private unnamed_addr constant [2 x i8] c\"r\\00\"\n")
	g.buf.WriteString("@.fmt_open_bracket = private unnamed_addr constant [2 x i8] c\"[\\00\"\n")
	g.buf.WriteString("@.fmt_close_bracket = private unnamed_addr constant [3 x i8] c\"]\\0a\\00\"\n")
	g.buf.WriteString("@.fmt_comma_space = private unnamed_addr constant [3 x i8] c\", \\00\"\n")
	g.buf.WriteString("@.fmt_close_bracket_single = private unnamed_addr constant [2 x i8] c\"]\\00\"\n")
	g.buf.WriteString("@.fmt_arr_int = private unnamed_addr constant [5 x i8] c\"%lld\\00\"\n\n")

	g.buf.WriteString("define i8* @str_array(%struct.Array* %arr) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %p_len = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 1\n")
	g.buf.WriteString("    %len = load i64, i64* %p_len\n")
	g.buf.WriteString("    %buf_ptr = alloca i8*\n")
	g.buf.WriteString("    %open = getelementptr inbounds [2 x i8], [2 x i8]* @.fmt_open_bracket, i64 0, i64 0\n")
	g.buf.WriteString("    %mem = call i8* @malloc(i64 2048)\n")
	g.buf.WriteString("    %cp = call i8* @strcpy(i8* %mem, i8* %open)\n")
	g.buf.WriteString("    store i8* %mem, i8** %buf_ptr\n")
	g.buf.WriteString("    %i_ptr = alloca i64\n")
	g.buf.WriteString("    store i64 0, i64* %i_ptr\n")
	g.buf.WriteString("    br label %loop\n\n")
	g.buf.WriteString("loop:\n")
	g.buf.WriteString("    %i = load i64, i64* %i_ptr\n")
	g.buf.WriteString("    %cmp = icmp slt i64 %i, %len\n")
	g.buf.WriteString("    br i1 %cmp, label %body, label %end\n\n")
	g.buf.WriteString("body:\n")
	g.buf.WriteString("    %cur_b = load i8*, i8** %buf_ptr\n")
	g.buf.WriteString("    %val = call i64 @array_get(%struct.Array* %arr, i64 %i)\n")
	g.buf.WriteString("    %sval = call i8* @str_int(i64 %val)\n")
	g.buf.WriteString("    %cat1 = call i8* @strcat(i8* %cur_b, i8* %sval)\n")
	g.buf.WriteString("    %i_next = add i64 %i, 1\n")
	g.buf.WriteString("    %has_next = icmp slt i64 %i_next, %len\n")
	g.buf.WriteString("    br i1 %has_next, label %comma, label %next_loop\n\n")
	g.buf.WriteString("comma:\n")
	g.buf.WriteString("    %fmt_comma = getelementptr inbounds [3 x i8], [3 x i8]* @.fmt_comma_space, i64 0, i64 0\n")
	g.buf.WriteString("    %cat2 = call i8* @strcat(i8* %cur_b, i8* %fmt_comma)\n")
	g.buf.WriteString("    br label %next_loop\n\n")
	g.buf.WriteString("next_loop:\n")
	g.buf.WriteString("    store i64 %i_next, i64* %i_ptr\n")
	g.buf.WriteString("    br label %loop\n\n")
	g.buf.WriteString("end:\n")
	g.buf.WriteString("    %final_b = load i8*, i8** %buf_ptr\n")
	g.buf.WriteString("    %close_bracket = getelementptr inbounds [2 x i8], [2 x i8]* @.fmt_close_bracket_single, i64 0, i64 0\n")
	g.buf.WriteString("    %cat3 = call i8* @strcat(i8* %final_b, i8* %close_bracket)\n")
	g.buf.WriteString("    ret i8* %final_b\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define void @println_array(%struct.Array* %arr) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %p_len = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 1\n")
	g.buf.WriteString("    %len = load i64, i64* %p_len\n")
	g.buf.WriteString("    %fmt_open = getelementptr inbounds [2 x i8], [2 x i8]* @.fmt_open_bracket, i64 0, i64 0\n")
	g.buf.WriteString("    %r0 = call i32 (i8*, ...) @printf(i8* %fmt_open)\n")
	g.buf.WriteString("    %i_ptr = alloca i64\n")
	g.buf.WriteString("    store i64 0, i64* %i_ptr\n")
	g.buf.WriteString("    br label %loop\n\n")
	g.buf.WriteString("loop:\n")
	g.buf.WriteString("    %i = load i64, i64* %i_ptr\n")
	g.buf.WriteString("    %cmp = icmp slt i64 %i, %len\n")
	g.buf.WriteString("    br i1 %cmp, label %body, label %end\n\n")
	g.buf.WriteString("body:\n")
	g.buf.WriteString("    %val = call i64 @array_get(%struct.Array* %arr, i64 %i)\n")
	g.buf.WriteString("    %fmt_int = getelementptr inbounds [5 x i8], [5 x i8]* @.fmt_arr_int, i64 0, i64 0\n")
	g.buf.WriteString("    %r1 = call i32 (i8*, ...) @printf(i8* %fmt_int, i64 %val)\n")
	g.buf.WriteString("    %i_next = add i64 %i, 1\n")
	g.buf.WriteString("    %has_next = icmp slt i64 %i_next, %len\n")
	g.buf.WriteString("    br i1 %has_next, label %comma, label %next_loop\n\n")
	g.buf.WriteString("comma:\n")
	g.buf.WriteString("    %fmt_comma = getelementptr inbounds [3 x i8], [3 x i8]* @.fmt_comma_space, i64 0, i64 0\n")
	g.buf.WriteString("    %r2 = call i32 (i8*, ...) @printf(i8* %fmt_comma)\n")
	g.buf.WriteString("    br label %next_loop\n\n")
	g.buf.WriteString("next_loop:\n")
	g.buf.WriteString("    store i64 %i_next, i64* %i_ptr\n")
	g.buf.WriteString("    br label %loop\n\n")
	g.buf.WriteString("end:\n")
	g.buf.WriteString("    %fmt_close = getelementptr inbounds [3 x i8], [3 x i8]* @.fmt_close_bracket, i64 0, i64 0\n")
	g.buf.WriteString("    %r3 = call i32 (i8*, ...) @printf(i8* %fmt_close)\n")
	g.buf.WriteString("    %fl = call i32 @fflush(i8* null)\n")
	g.buf.WriteString("    ret void\n")
	g.buf.WriteString("}\n\n")

	// Helper function: sys_now_millis
	g.buf.WriteString("define i64 @sys_now_millis() {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %t = call i64 @time(i8* null)\n")
	g.buf.WriteString("    %ms = mul i64 %t, 1000\n")
	g.buf.WriteString("    ret i64 %ms\n")
	g.buf.WriteString("}\n\n")

	// Helper function: fs_write_file
	g.buf.WriteString("define void @fs_write_file(i8* %path, i8* %data) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %mode_str = getelementptr inbounds [2 x i8], [2 x i8]* @.mode_w, i64 0, i64 0\n")
	g.buf.WriteString("    %f = call i8* @fopen(i8* %path, i8* %mode_str)\n")
	g.buf.WriteString("    %cmp = icmp ne i8* %f, null\n")
	g.buf.WriteString("    br i1 %cmp, label %write_ok, label %write_end\n")
	g.buf.WriteString("write_ok:\n")
	g.buf.WriteString("    %fp = call i32 @fputs(i8* %data, i8* %f)\n")
	g.buf.WriteString("    %fc = call i32 @fclose(i8* %f)\n")
	g.buf.WriteString("    br label %write_end\n")
	g.buf.WriteString("write_end:\n")
	g.buf.WriteString("    ret void\n")
	g.buf.WriteString("}\n\n")

	// Helper function: fs_read_file
	g.buf.WriteString("define i8* @fs_read_file(i8* %path) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %mode_str = getelementptr inbounds [2 x i8], [2 x i8]* @.mode_r, i64 0, i64 0\n")
	g.buf.WriteString("    %f = call i8* @fopen(i8* %path, i8* %mode_str)\n")
	g.buf.WriteString("    %mem = call i8* @malloc(i64 4096)\n")
	g.buf.WriteString("    %nread = call i64 @fread(i8* %mem, i64 1, i64 4095, i8* %f)\n")
	g.buf.WriteString("    %null_ptr = getelementptr inbounds i8, i8* %mem, i64 %nread\n")
	g.buf.WriteString("    store i8 0, i8* %null_ptr\n")
	g.buf.WriteString("    %fc = call i32 @fclose(i8* %f)\n")
	g.buf.WriteString("    ret i8* %mem\n")
	g.buf.WriteString("}\n\n")

	// Empty string constant
	g.buf.WriteString("@.empty_str = private unnamed_addr constant [1 x i8] c\"\\00\"\n\n")

	// Helper function: concat strings
	g.buf.WriteString("define i8* @concat_strings(i8* %s1, i8* %s2) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %cmp1 = icmp eq i8* %s1, null\n")
	g.buf.WriteString("    br i1 %cmp1, label %use_empty1, label %chk2\n\n")
	g.buf.WriteString("use_empty1:\n")
	g.buf.WriteString("    %empty1 = getelementptr inbounds [1 x i8], [1 x i8]* @.empty_str, i64 0, i64 0\n")
	g.buf.WriteString("    br label %chk2\n\n")
	g.buf.WriteString("chk2:\n")
	g.buf.WriteString("    %p1 = phi i8* [ %s1, %entry ], [ %empty1, %use_empty1 ]\n")
	g.buf.WriteString("    %cmp2 = icmp eq i8* %s2, null\n")
	g.buf.WriteString("    br i1 %cmp2, label %use_empty2, label %do_concat\n\n")
	g.buf.WriteString("use_empty2:\n")
	g.buf.WriteString("    %empty2 = getelementptr inbounds [1 x i8], [1 x i8]* @.empty_str, i64 0, i64 0\n")
	g.buf.WriteString("    br label %do_concat\n\n")
	g.buf.WriteString("do_concat:\n")
	g.buf.WriteString("    %p2 = phi i8* [ %s2, %chk2 ], [ %empty2, %use_empty2 ]\n")
	g.buf.WriteString("    %l1 = call i64 @strlen(i8* %p1)\n")
	g.buf.WriteString("    %l2 = call i64 @strlen(i8* %p2)\n")
	g.buf.WriteString("    %tot = add i64 %l1, %l2\n")
	g.buf.WriteString("    %tot1 = add i64 %tot, 1\n")
	g.buf.WriteString("    %mem = call i8* @malloc(i64 %tot1)\n")
	g.buf.WriteString("    %cp = call i8* @strcpy(i8* %mem, i8* %p1)\n")
	g.buf.WriteString("    %cat = call i8* @strcat(i8* %mem, i8* %p2)\n")
	g.buf.WriteString("    ret i8* %mem\n")
	g.buf.WriteString("}\n\n")

	// Helper function: int to string
	g.buf.WriteString("define i8* @str_int(i64 %val) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %mem = call i8* @malloc(i64 32)\n")
	g.buf.WriteString("    %fmt = getelementptr inbounds [5 x i8], [5 x i8]* @.fmt_int_raw, i64 0, i64 0\n")
	g.buf.WriteString("    %sp = call i32 (i8*, i8*, ...) @sprintf(i8* %mem, i8* %fmt, i64 %val)\n")
	g.buf.WriteString("    ret i8* %mem\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define i8* @chr(i64 %code) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %mem = call i8* @malloc(i64 2)\n")
	g.buf.WriteString("    %ch = trunc i64 %code to i8\n")
	g.buf.WriteString("    store i8 %ch, i8* %mem\n")
	g.buf.WriteString("    %p1 = getelementptr i8, i8* %mem, i64 1\n")
	g.buf.WriteString("    store i8 0, i8* %p1\n")
	g.buf.WriteString("    ret i8* %mem\n")
	g.buf.WriteString("}\n\n")

	// Helper function: char to 1-char string or pointer
	g.buf.WriteString("define i8* @char_to_str(i64 %val) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %cmp = icmp ugt i64 %val, 65536\n")
	g.buf.WriteString("    br i1 %cmp, label %as_ptr, label %as_char\n")
	g.buf.WriteString("as_ptr:\n")
	g.buf.WriteString("    %ptr = inttoptr i64 %val to i8*\n")
	g.buf.WriteString("    ret i8* %ptr\n")
	g.buf.WriteString("as_char:\n")
	g.buf.WriteString("    %mem = call i8* @malloc(i64 2)\n")
	g.buf.WriteString("    %b = trunc i64 %val to i8\n")
	g.buf.WriteString("    store i8 %b, i8* %mem\n")
	g.buf.WriteString("    %p1 = getelementptr inbounds i8, i8* %mem, i64 1\n")
	g.buf.WriteString("    store i8 0, i8* %p1\n")
	g.buf.WriteString("    ret i8* %mem\n")
	g.buf.WriteString("}\n\n")

	// ARC Memory Management Helpers
	g.buf.WriteString("define void @rc_retain(i8* %ptr) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %cmp = icmp eq i8* %ptr, null\n")
	g.buf.WriteString("    br i1 %cmp, label %ret, label %do_retain\n\n")
	g.buf.WriteString("do_retain:\n")
	g.buf.WriteString("    %hptr = getelementptr inbounds i8, i8* %ptr, i64 -8\n")
	g.buf.WriteString("    %iptr = bitcast i8* %hptr to i64*\n")
	g.buf.WriteString("    %rc = load i64, i64* %iptr\n")
	g.buf.WriteString("    %rc1 = add i64 %rc, 1\n")
	g.buf.WriteString("    store i64 %rc1, i64* %iptr\n")
	g.buf.WriteString("    br label %ret\n\n")
	g.buf.WriteString("ret:\n")
	g.buf.WriteString("    ret void\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define void @rc_release(i8* %ptr) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %cmp = icmp eq i8* %ptr, null\n")
	g.buf.WriteString("    br i1 %cmp, label %ret, label %do_rel\n\n")
	g.buf.WriteString("do_rel:\n")
	g.buf.WriteString("    %hptr = getelementptr inbounds i8, i8* %ptr, i64 -8\n")
	g.buf.WriteString("    %iptr = bitcast i8* %hptr to i64*\n")
	g.buf.WriteString("    %rc = load i64, i64* %iptr\n")
	g.buf.WriteString("    %rc1 = sub i64 %rc, 1\n")
	g.buf.WriteString("    store i64 %rc1, i64* %iptr\n")
	g.buf.WriteString("    %zero = icmp sle i64 %rc1, 0\n")
	g.buf.WriteString("    br i1 %zero, label %free_blk, label %ret\n\n")
	g.buf.WriteString("free_blk:\n")
	g.buf.WriteString("    call void @free(i8* %hptr)\n")
	g.buf.WriteString("    br label %ret\n\n")
	g.buf.WriteString("ret:\n")
	g.buf.WriteString("    ret void\n")
	g.buf.WriteString("}\n\n")

	// Option helpers
	g.buf.WriteString("define %struct.Option @Some(i64 %v) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %opt = alloca %struct.Option\n")
	g.buf.WriteString("    %p_some = getelementptr inbounds %struct.Option, %struct.Option* %opt, i32 0, i32 0\n")
	g.buf.WriteString("    store i1 true, i1* %p_some\n")
	g.buf.WriteString("    %p_val = getelementptr inbounds %struct.Option, %struct.Option* %opt, i32 0, i32 1\n")
	g.buf.WriteString("    store i64 %v, i64* %p_val\n")
	g.buf.WriteString("    %res = load %struct.Option, %struct.Option* %opt\n")
	g.buf.WriteString("    ret %struct.Option %res\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define %struct.Option @None() {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %opt = alloca %struct.Option\n")
	g.buf.WriteString("    %p_some = getelementptr inbounds %struct.Option, %struct.Option* %opt, i32 0, i32 0\n")
	g.buf.WriteString("    store i1 false, i1* %p_some\n")
	g.buf.WriteString("    %p_val = getelementptr inbounds %struct.Option, %struct.Option* %opt, i32 0, i32 1\n")
	g.buf.WriteString("    store i64 0, i64* %p_val\n")
	g.buf.WriteString("    %res = load %struct.Option, %struct.Option* %opt\n")
	g.buf.WriteString("    ret %struct.Option %res\n")
	g.buf.WriteString("}\n\n")

	// Result helpers
	g.buf.WriteString("define %struct.Result @Ok(i64 %v) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %res_alloc = alloca %struct.Result\n")
	g.buf.WriteString("    %p_ok = getelementptr inbounds %struct.Result, %struct.Result* %res_alloc, i32 0, i32 0\n")
	g.buf.WriteString("    store i1 true, i1* %p_ok\n")
	g.buf.WriteString("    %p_val = getelementptr inbounds %struct.Result, %struct.Result* %res_alloc, i32 0, i32 1\n")
	g.buf.WriteString("    store i64 %v, i64* %p_val\n")
	g.buf.WriteString("    %p_err = getelementptr inbounds %struct.Result, %struct.Result* %res_alloc, i32 0, i32 2\n")
	g.buf.WriteString("    store i8* null, i8** %p_err\n")
	g.buf.WriteString("    %res = load %struct.Result, %struct.Result* %res_alloc\n")
	g.buf.WriteString("    ret %struct.Result %res\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define %struct.Result @Err(i8* %msg) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %res_alloc = alloca %struct.Result\n")
	g.buf.WriteString("    %p_ok = getelementptr inbounds %struct.Result, %struct.Result* %res_alloc, i32 0, i32 0\n")
	g.buf.WriteString("    store i1 false, i1* %p_ok\n")
	g.buf.WriteString("    %p_val = getelementptr inbounds %struct.Result, %struct.Result* %res_alloc, i32 0, i32 1\n")
	g.buf.WriteString("    store i64 0, i64* %p_val\n")
	g.buf.WriteString("    %p_err = getelementptr inbounds %struct.Result, %struct.Result* %res_alloc, i32 0, i32 2\n")
	g.buf.WriteString("    store i8* %msg, i8** %p_err\n")
	g.buf.WriteString("    %res = load %struct.Result, %struct.Result* %res_alloc\n")
	g.buf.WriteString("    ret %struct.Result %res\n")
	g.buf.WriteString("}\n\n")

	// Array runtime helpers
	g.buf.WriteString("define %struct.Array* @create_array(i64 %cap) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %arr_raw = call i8* @malloc(i64 24)\n")
	g.buf.WriteString("    %arr = bitcast i8* %arr_raw to %struct.Array*\n")
	g.buf.WriteString("    %cap_bytes = mul i64 %cap, 8\n")
	g.buf.WriteString("    %data_raw = call i8* @malloc(i64 %cap_bytes)\n")
	g.buf.WriteString("    %data = bitcast i8* %data_raw to i64*\n")
	g.buf.WriteString("    %p_data = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 0\n")
	g.buf.WriteString("    store i64* %data, i64** %p_data\n")
	g.buf.WriteString("    %p_len = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 1\n")
	g.buf.WriteString("    store i64 0, i64* %p_len\n")
	g.buf.WriteString("    %p_cap = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 2\n")
	g.buf.WriteString("    store i64 %cap, i64* %p_cap\n")
	g.buf.WriteString("    ret %struct.Array* %arr\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define void @array_append(%struct.Array* %arr, i64 %val) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %p_len = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 1\n")
	g.buf.WriteString("    %len = load i64, i64* %p_len\n")
	g.buf.WriteString("    %p_data = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 0\n")
	g.buf.WriteString("    %data = load i64*, i64** %p_data\n")
	g.buf.WriteString("    %elem_ptr = getelementptr inbounds i64, i64* %data, i64 %len\n")
	g.buf.WriteString("    store i64 %val, i64* %elem_ptr\n")
	g.buf.WriteString("    %new_len = add i64 %len, 1\n")
	g.buf.WriteString("    store i64 %new_len, i64* %p_len\n")
	g.buf.WriteString("    ret void\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define i64 @array_get(%struct.Array* %arr, i64 %idx) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %p_data = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 0\n")
	g.buf.WriteString("    %data = load i64*, i64** %p_data\n")
	g.buf.WriteString("    %elem_ptr = getelementptr inbounds i64, i64* %data, i64 %idx\n")
	g.buf.WriteString("    %val = load i64, i64* %elem_ptr\n")
	g.buf.WriteString("    ret i64 %val\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define i64 @array_len(%struct.Array* %arr) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %p_len = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 1\n")
	g.buf.WriteString("    %len = load i64, i64* %p_len\n")
	g.buf.WriteString("    ret i64 %len\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define void @array_sort(%struct.Array* %arr) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %p_len = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 1\n")
	g.buf.WriteString("    %len = load i64, i64* %p_len\n")
	g.buf.WriteString("    %i_ptr = alloca i64\n")
	g.buf.WriteString("    store i64 0, i64* %i_ptr\n")
	g.buf.WriteString("    br label %loop_i\n\n")
	g.buf.WriteString("loop_i:\n")
	g.buf.WriteString("    %i = load i64, i64* %i_ptr\n")
	g.buf.WriteString("    %cmp_i = icmp slt i64 %i, %len\n")
	g.buf.WriteString("    br i1 %cmp_i, label %body_i, label %end\n\n")
	g.buf.WriteString("body_i:\n")
	g.buf.WriteString("    %j_init = add i64 %i, 1\n")
	g.buf.WriteString("    %j_ptr = alloca i64\n")
	g.buf.WriteString("    store i64 %j_init, i64* %j_ptr\n")
	g.buf.WriteString("    br label %loop_j\n\n")
	g.buf.WriteString("loop_j:\n")
	g.buf.WriteString("    %j = load i64, i64* %j_ptr\n")
	g.buf.WriteString("    %cmp_j = icmp slt i64 %j, %len\n")
	g.buf.WriteString("    br i1 %cmp_j, label %body_j, label %next_i\n\n")
	g.buf.WriteString("body_j:\n")
	g.buf.WriteString("    %vi = call i64 @array_get(%struct.Array* %arr, i64 %i)\n")
	g.buf.WriteString("    %vj = call i64 @array_get(%struct.Array* %arr, i64 %j)\n")
	g.buf.WriteString("    %swap_cond = icmp sgt i64 %vi, %vj\n")
	g.buf.WriteString("    br i1 %swap_cond, label %do_swap, label %next_j\n\n")
	g.buf.WriteString("do_swap:\n")
	g.buf.WriteString("    %p_data = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 0\n")
	g.buf.WriteString("    %data = load i64*, i64** %p_data\n")
	g.buf.WriteString("    %pi = getelementptr inbounds i64, i64* %data, i64 %i\n")
	g.buf.WriteString("    %pj = getelementptr inbounds i64, i64* %data, i64 %j\n")
	g.buf.WriteString("    store i64 %vj, i64* %pi\n")
	g.buf.WriteString("    store i64 %vi, i64* %pj\n")
	g.buf.WriteString("    br label %next_j\n\n")
	g.buf.WriteString("next_j:\n")
	g.buf.WriteString("    %next_j_val = add i64 %j, 1\n")
	g.buf.WriteString("    store i64 %next_j_val, i64* %j_ptr\n")
	g.buf.WriteString("    br label %loop_j\n\n")
	g.buf.WriteString("next_i:\n")
	g.buf.WriteString("    %next_i_val = add i64 %i, 1\n")
	g.buf.WriteString("    store i64 %next_i_val, i64* %i_ptr\n")
	g.buf.WriteString("    br label %loop_i\n\n")
	g.buf.WriteString("end:\n")
	g.buf.WriteString("    ret void\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define void @array_reverse(%struct.Array* %arr) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %p_len = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 1\n")
	g.buf.WriteString("    %len = load i64, i64* %p_len\n")
	g.buf.WriteString("    %half = sdiv i64 %len, 2\n")
	g.buf.WriteString("    %i_ptr = alloca i64\n")
	g.buf.WriteString("    store i64 0, i64* %i_ptr\n")
	g.buf.WriteString("    br label %loop\n\n")
	g.buf.WriteString("loop:\n")
	g.buf.WriteString("    %i = load i64, i64* %i_ptr\n")
	g.buf.WriteString("    %cmp = icmp slt i64 %i, %half\n")
	g.buf.WriteString("    br i1 %cmp, label %body, label %end\n\n")
	g.buf.WriteString("body:\n")
	g.buf.WriteString("    %len_minus_1 = sub i64 %len, 1\n")
	g.buf.WriteString("    %j = sub i64 %len_minus_1, %i\n")
	g.buf.WriteString("    %vi = call i64 @array_get(%struct.Array* %arr, i64 %i)\n")
	g.buf.WriteString("    %vj = call i64 @array_get(%struct.Array* %arr, i64 %j)\n")
	g.buf.WriteString("    %p_data = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 0\n")
	g.buf.WriteString("    %data = load i64*, i64** %p_data\n")
	g.buf.WriteString("    %pi = getelementptr inbounds i64, i64* %data, i64 %i\n")
	g.buf.WriteString("    %pj = getelementptr inbounds i64, i64* %data, i64 %j\n")
	g.buf.WriteString("    store i64 %vj, i64* %pi\n")
	g.buf.WriteString("    store i64 %vi, i64* %pj\n")
	g.buf.WriteString("    %next_i = add i64 %i, 1\n")
	g.buf.WriteString("    store i64 %next_i, i64* %i_ptr\n")
	g.buf.WriteString("    br label %loop\n\n")
	g.buf.WriteString("end:\n")
	g.buf.WriteString("    ret void\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define i64 @array_find(%struct.Array* %arr, i64 %val) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %p_len = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 1\n")
	g.buf.WriteString("    %len = load i64, i64* %p_len\n")
	g.buf.WriteString("    %i_ptr = alloca i64\n")
	g.buf.WriteString("    store i64 0, i64* %i_ptr\n")
	g.buf.WriteString("    br label %loop\n\n")
	g.buf.WriteString("loop:\n")
	g.buf.WriteString("    %i = load i64, i64* %i_ptr\n")
	g.buf.WriteString("    %cmp = icmp slt i64 %i, %len\n")
	g.buf.WriteString("    br i1 %cmp, label %body, label %not_found\n\n")
	g.buf.WriteString("body:\n")
	g.buf.WriteString("    %curr = call i64 @array_get(%struct.Array* %arr, i64 %i)\n")
	g.buf.WriteString("    %match = icmp eq i64 %curr, %val\n")
	g.buf.WriteString("    br i1 %match, label %found, label %next\n\n")
	g.buf.WriteString("found:\n")
	g.buf.WriteString("    ret i64 %i\n\n")
	g.buf.WriteString("next:\n")
	g.buf.WriteString("    %next_i = add i64 %i, 1\n")
	g.buf.WriteString("    store i64 %next_i, i64* %i_ptr\n")
	g.buf.WriteString("    br label %loop\n\n")
	g.buf.WriteString("not_found:\n")
	g.buf.WriteString("    ret i64 -1\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define i64 @array_contains(%struct.Array* %arr, i64 %val) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %idx = call i64 @array_find(%struct.Array* %arr, i64 %val)\n")
	g.buf.WriteString("    %cmp = icmp ne i64 %idx, -1\n")
	g.buf.WriteString("    %res = zext i1 %cmp to i64\n")
	g.buf.WriteString("    ret i64 %res\n")
	g.buf.WriteString("}\n\n")

	g.buf.WriteString("define i8* @array_join(%struct.Array* %arr, i8* %sep) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %p_len = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 1\n")
	g.buf.WriteString("    %len = load i64, i64* %p_len\n")
	g.buf.WriteString("    %cmp = icmp eq i64 %len, 0\n")
	g.buf.WriteString("    br i1 %cmp, label %empty, label %build\n\n")
	g.buf.WriteString("empty:\n")
	g.buf.WriteString("    %e_mem = call i8* @malloc(i64 1)\n")
	g.buf.WriteString("    store i8 0, i8* %e_mem\n")
	g.buf.WriteString("    ret i8* %e_mem\n\n")
	g.buf.WriteString("build:\n")
	g.buf.WriteString("    %buf_ptr = alloca i8*\n")
	g.buf.WriteString("    %v0 = call i64 @array_get(%struct.Array* %arr, i64 0)\n")
	g.buf.WriteString("    %s0 = inttoptr i64 %v0 to i8*\n")
	g.buf.WriteString("    %s0_len = call i64 @strlen(i8* %s0)\n")
	g.buf.WriteString("    %alloc_len = add i64 %s0_len, 1024\n")
	g.buf.WriteString("    %mem = call i8* @malloc(i64 %alloc_len)\n")
	g.buf.WriteString("    %cp = call i8* @strcpy(i8* %mem, i8* %s0)\n")
	g.buf.WriteString("    store i8* %mem, i8** %buf_ptr\n")
	g.buf.WriteString("    %i_ptr = alloca i64\n")
	g.buf.WriteString("    store i64 1, i64* %i_ptr\n")
	g.buf.WriteString("    br label %loop\n\n")
	g.buf.WriteString("loop:\n")
	g.buf.WriteString("    %i = load i64, i64* %i_ptr\n")
	g.buf.WriteString("    %l_cmp = icmp slt i64 %i, %len\n")
	g.buf.WriteString("    br i1 %l_cmp, label %body, label %end\n\n")
	g.buf.WriteString("body:\n")
	g.buf.WriteString("    %cur_buf = load i8*, i8** %buf_ptr\n")
	g.buf.WriteString("    %cat1 = call i8* @strcat(i8* %cur_buf, i8* %sep)\n")
	g.buf.WriteString("    %vi = call i64 @array_get(%struct.Array* %arr, i64 %i)\n")
	g.buf.WriteString("    %si = inttoptr i64 %vi to i8*\n")
	g.buf.WriteString("    %cat2 = call i8* @strcat(i8* %cur_buf, i8* %si)\n")
	g.buf.WriteString("    %next_i = add i64 %i, 1\n")
	g.buf.WriteString("    store i64 %next_i, i64* %i_ptr\n")
	g.buf.WriteString("    br label %loop\n\n")
	g.buf.WriteString("end:\n")
	g.buf.WriteString("    %final_buf = load i8*, i8** %buf_ptr\n")
	g.buf.WriteString("    ret i8* %final_buf\n")
	g.buf.WriteString("}\n\n")

	// Map lookup helper: @map_get(%struct.Array* %arr, i8* %key)
	g.buf.WriteString("define i64 @map_get(%struct.Array* %arr, i8* %key) {\n")
	g.buf.WriteString("entry:\n")
	g.buf.WriteString("    %p_len = getelementptr inbounds %struct.Array, %struct.Array* %arr, i32 0, i32 1\n")
	g.buf.WriteString("    %len = load i64, i64* %p_len\n")
	g.buf.WriteString("    %idx_ptr = alloca i64\n")
	g.buf.WriteString("    store i64 0, i64* %idx_ptr\n")
	g.buf.WriteString("    br label %cond\n\n")
	g.buf.WriteString("cond:\n")
	g.buf.WriteString("    %cur_idx = load i64, i64* %idx_ptr\n")
	g.buf.WriteString("    %cond_reg = icmp slt i64 %cur_idx, %len\n")
	g.buf.WriteString("    br i1 %cond_reg, label %body, label %end\n\n")
	g.buf.WriteString("body:\n")
	g.buf.WriteString("    %k_val = call i64 @array_get(%struct.Array* %arr, i64 %cur_idx)\n")
	g.buf.WriteString("    %k_str = inttoptr i64 %k_val to i8*\n")
	g.buf.WriteString("    %cmp = call i32 @strcmp(i8* %k_str, i8* %key)\n")
	g.buf.WriteString("    %match = icmp eq i32 %cmp, 0\n")
	g.buf.WriteString("    br i1 %match, label %found, label %next\n\n")
	g.buf.WriteString("found:\n")
	g.buf.WriteString("    %v_idx = add i64 %cur_idx, 1\n")
	g.buf.WriteString("    %v_val = call i64 @array_get(%struct.Array* %arr, i64 %v_idx)\n")
	g.buf.WriteString("    ret i64 %v_val\n\n")
	g.buf.WriteString("next:\n")
	g.buf.WriteString("    %next_idx = add i64 %cur_idx, 2\n")
	g.buf.WriteString("    store i64 %next_idx, i64* %idx_ptr\n")
	g.buf.WriteString("    br label %cond\n\n")
	g.buf.WriteString("end:\n")
	g.buf.WriteString("    ret i64 0\n")
	g.buf.WriteString("}\n\n")

	// Format strings for printf & sprintf
	g.buf.WriteString("@.fmt_int = private unnamed_addr constant [6 x i8] c\"%lld\\0A\\00\"\n")
	g.buf.WriteString("@.fmt_int_raw = private unnamed_addr constant [5 x i8] c\"%lld\\00\"\n")
	g.buf.WriteString("@.fmt_float = private unnamed_addr constant [4 x i8] c\"%f\\0A\\00\"\n")
	g.buf.WriteString("@.fmt_str = private unnamed_addr constant [4 x i8] c\"%s\\0A\\00\"\n")
	g.buf.WriteString("@.fmt_str_raw = private unnamed_addr constant [3 x i8] c\"%s\\00\"\n\n")

	// Emit accumulated global string literals
	for lit, varName := range g.globalStrings {
		byteLen := len(lit) + 1
		g.buf.WriteString(fmt.Sprintf("%s = private unnamed_addr constant [%d x i8] c\"%s\\00\"\n", varName, byteLen, escapeLLVMString(lit)))
	}
	if len(g.globalStrings) > 0 {
		g.buf.WriteString("\n")
	}

	// Append generated functions
	g.buf.Write(codeBuf.Bytes())
	g.buf.Write(g.topLevelBuf.Bytes())

	return g.buf.String(), nil
}

func (g *LLVMGenerator) generateStatement(buf *bytes.Buffer, stmt ast.Statement) error {
	switch s := stmt.(type) {
	case *ast.FnDeclStmt:
		g.currentFn = s.Name.Value
		if len(s.TypeParams) > 0 {
			return nil
		}

		g.regCounter = 0
		g.symbolTable = make(map[string]string)
		g.varAllocaMap = make(map[string]string)

		retType := mapLLVMType(s.ReturnType)
		funcName := s.Name.Value
		if s.Name.Value == "main" {
			retType = "i32"
		}

		var params []string
		if s.Receiver != nil {
			funcName = s.Receiver.Type + "_" + s.Name.Value
			recvType := mapLLVMType(s.Receiver.Type)
			params = append(params, fmt.Sprintf("%s* %%%s.arg", recvType, s.Receiver.Name.Value))
		} else if len(s.Params) > 0 && s.Params[0].Name.Value == "self" {
			funcName = s.Params[0].Type + "_" + s.Name.Value
		}

		for _, p := range s.Params {
			pType := mapLLVMType(p.Type)
			params = append(params, fmt.Sprintf("%s %%%s.arg", pType, p.Name.Value))
		}

		inlineAttr := ""
		for _, dec := range s.Decorators {
			if dec == "inline" {
				inlineAttr = " alwaysinline"
			}
		}
		buf.WriteString(fmt.Sprintf("define %s @%s(%s)%s {\n", retType, funcName, strings.Join(params, ", "), inlineAttr))
		buf.WriteString("entry:\n")

		if s.Receiver != nil {
			recvType := mapLLVMType(s.Receiver.Type)
			ptrReg := fmt.Sprintf("%%%s.addr", s.Receiver.Name.Value)
			buf.WriteString(fmt.Sprintf("    %s = alloca %s*\n", ptrReg, recvType))
			buf.WriteString(fmt.Sprintf("    store %s* %%%s.arg, %s** %s\n", recvType, s.Receiver.Name.Value, recvType, ptrReg))
			g.symbolTable[s.Receiver.Name.Value] = recvType + "*"
			g.varAllocaMap[s.Receiver.Name.Value] = ptrReg
		}

		for _, p := range s.Params {
			pType := mapLLVMType(p.Type)
			ptrReg := fmt.Sprintf("%%%s.addr", p.Name.Value)
			buf.WriteString(fmt.Sprintf("    %s = alloca %s\n", ptrReg, pType))
			buf.WriteString(fmt.Sprintf("    store %s %%%s.arg, %s* %s\n", pType, p.Name.Value, pType, ptrReg))
			g.symbolTable[p.Name.Value] = pType
			g.varAllocaMap[p.Name.Value] = ptrReg
		}

		g.currentFn = funcName
		g.deferredStmts = []ast.Expression{}

		for _, bStmt := range s.Body.Statements {
			if err := g.generateBodyStatement(buf, bStmt); err != nil {
				return err
			}
		}

		g.emitDeferred(buf)

		if s.Name.Value == "main" {
			flReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i32 @fflush(i8* null)\n", flReg))
			buf.WriteString("    ret i32 0\n")
		} else if retType == "void" {
			buf.WriteString("    ret void\n")
		} else {
			buf.WriteString("    unreachable\n")
		}
		buf.WriteString("}\n\n")

		if s.IsRPC {
			rpcFuncName := funcName + "_rpc_call"
			g.fnReturnTypes[rpcFuncName] = retType
			g.regCounter = 0
			var rpcParams []string
			rpcParams = append(rpcParams, "i8* %node_endpoint")
			var callArgs []string
			for idx, p := range s.Params {
				pType := mapLLVMType(p.Type)
				rpcParams = append(rpcParams, fmt.Sprintf("%s %%p%d", pType, idx))
				callArgs = append(callArgs, fmt.Sprintf("%s %%p%d", pType, idx))
			}

			buf.WriteString(fmt.Sprintf("define %s @%s(%s) {\n", retType, rpcFuncName, strings.Join(rpcParams, ", ")))
			buf.WriteString("entry:\n")
			resCall := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call %s @%s(%s)\n", resCall, retType, funcName, strings.Join(callArgs, ", ")))
			buf.WriteString(fmt.Sprintf("    ret %s %s\n", retType, resCall))
			buf.WriteString("}\n\n")
		}

	case *ast.TraitDeclStmt:
		return nil

	case *ast.ImplDeclStmt:
		for _, m := range s.Methods {
			if err := g.generateStatement(buf, m); err != nil {
				return err
			}
		}
		return nil

	case *ast.SpawnStmt:
		fnPtr, _, err := g.generateExpression(buf, s.Call)
		if err != nil {
			return err
		}
		if strings.HasPrefix(fnPtr, "@") {
			buf.WriteString(fmt.Sprintf("    call void %s()\n", fnPtr))
		}
		return nil

	case *ast.ExternFnStmt:
		retType := mapLLVMType(s.ReturnType)
		var params []string
		for _, p := range s.Params {
			params = append(params, mapLLVMType(p.Type))
		}
		declStr := fmt.Sprintf("declare %s @%s(%s)\n", retType, s.Name.Value, strings.Join(params, ", "))
		builtinDecls := map[string]bool{
			"printf": true, "puts": true, "sprintf": true, "fflush": true,
			"strlen": true, "strcmp": true, "strcpy": true, "strcat": true,
			"malloc": true, "realloc": true, "free": true, "time": true,
			"fopen": true, "fputs": true, "fclose": true,
		}
		if !builtinDecls[s.Name.Value] && !strings.Contains(g.topLevelBuf.String(), "@"+s.Name.Value+"(") {
			g.topLevelBuf.WriteString(declStr)
		}
		return nil

	case *ast.StructDeclStmt:
		g.structTypes[s.Name.Value] = s.Fields

		// Process @derive(...) macros by synthesizing FnDeclStmt AST nodes
		for _, dec := range s.Decorators {
			if strings.HasPrefix(dec, "derive(") {
				inner := strings.TrimSuffix(strings.TrimPrefix(dec, "derive("), ")")
				traits := strings.Split(inner, ",")
				for _, t := range traits {
					trait := strings.TrimSpace(t)
					if trait == "Debug" {
						g.methodReceivers["to_string"] = s.Name.Value
						// Synthesize `fn (self: MyStruct) to_string() -> string:`
						var expr ast.Expression = &ast.StringLiteral{Value: s.Name.Value + "("}
						for idx, f := range s.Fields {
							expr = &ast.InfixExpr{
								Left:     expr,
								Operator: "+",
								Right:    &ast.StringLiteral{Value: f.Name.Value + "="},
							}
							strCall := &ast.CallExpr{
								Function:  &ast.Identifier{Value: "str"},
								Arguments: []ast.Expression{&ast.MemberExpr{Object: &ast.Identifier{Value: "self"}, Member: f.Name}},
							}
							expr = &ast.InfixExpr{
								Left:     expr,
								Operator: "+",
								Right:    strCall,
							}
							if idx < len(s.Fields)-1 {
								expr = &ast.InfixExpr{
									Left:     expr,
									Operator: "+",
									Right:    &ast.StringLiteral{Value: ", "},
								}
							}
						}
						expr = &ast.InfixExpr{
							Left:     expr,
							Operator: "+",
							Right:    &ast.StringLiteral{Value: ")"},
						}

						toStrFn := &ast.FnDeclStmt{
							Name:       &ast.Identifier{Value: "to_string"},
							Receiver:   &ast.Param{Name: &ast.Identifier{Value: "self"}, Type: s.Name.Value},
							ReturnType: "string",
							Body: &ast.BlockStmt{
								Statements: []ast.Statement{
									&ast.ReturnStmt{Value: expr},
								},
							},
						}
						g.fnReturnTypes[s.Name.Value+"_to_string"] = "i8*"
						_ = g.generateStatement(buf, toStrFn)
					} else if trait == "Clone" {
						g.methodReceivers["clone"] = s.Name.Value
						g.fnReturnTypes[s.Name.Value+"_clone"] = "%struct." + s.Name.Value
						// Synthesize `fn (self: MyStruct) clone() -> MyStruct:`
						var args []ast.Expression
						for _, f := range s.Fields {
							args = append(args, &ast.MemberExpr{Object: &ast.Identifier{Value: "self"}, Member: f.Name})
						}
						cloneFn := &ast.FnDeclStmt{
							Name:       &ast.Identifier{Value: "clone"},
							Receiver:   &ast.Param{Name: &ast.Identifier{Value: "self"}, Type: s.Name.Value},
							ReturnType: s.Name.Value,
							Body: &ast.BlockStmt{
								Statements: []ast.Statement{
									&ast.ReturnStmt{Value: &ast.CallExpr{Function: &ast.Identifier{Value: s.Name.Value}, Arguments: args}},
								},
							},
						}
						_ = g.generateStatement(buf, cloneFn)
					} else if trait == "Eq" {
						g.methodReceivers["equals"] = s.Name.Value
						g.fnReturnTypes[s.Name.Value+"_equals"] = "i1"
						// Synthesize `fn (self: MyStruct) equals(other: MyStruct) -> bool:`
						var cond ast.Expression = &ast.BoolLiteral{Value: true}
						if len(s.Fields) > 0 {
							for idx, f := range s.Fields {
								eqExpr := &ast.InfixExpr{
									Left:     &ast.MemberExpr{Object: &ast.Identifier{Value: "self"}, Member: f.Name},
									Operator: "==",
									Right:    &ast.MemberExpr{Object: &ast.Identifier{Value: "other"}, Member: f.Name},
								}
								if idx == 0 {
									cond = eqExpr
								} else {
									cond = &ast.InfixExpr{Left: cond, Operator: "&&", Right: eqExpr}
								}
							}
						}
						eqFn := &ast.FnDeclStmt{
							Name:       &ast.Identifier{Value: "equals"},
							Receiver:   &ast.Param{Name: &ast.Identifier{Value: "self"}, Type: s.Name.Value},
							Params:     []ast.Param{{Name: &ast.Identifier{Value: "other"}, Type: s.Name.Value}},
							ReturnType: "bool",
							Body: &ast.BlockStmt{
								Statements: []ast.Statement{
									&ast.ReturnStmt{Value: cond},
								},
							},
						}
						_ = g.generateStatement(buf, eqFn)
					}
				}
			}
		}
		return nil

	default:
		return g.generateBodyStatement(buf, stmt)
	}
	return nil
}

func (g *LLVMGenerator) generateBodyStatement(buf *bytes.Buffer, stmt ast.Statement) error {
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		valReg, valType, err := g.generateExpression(buf, s.Value)
		if err != nil {
			return err
		}

		if s.Type != "" {
			valType = mapLLVMType(s.Type)
		}

		g.labelCounter++
		ptrReg := fmt.Sprintf("%%%s_%d.addr", s.Name.Value, g.labelCounter)
		buf.WriteString(fmt.Sprintf("    %s = alloca %s\n", ptrReg, valType))
		buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", valType, valReg, valType, ptrReg))

		g.symbolTable[s.Name.Value] = valType
		g.varAllocaMap[s.Name.Value] = ptrReg

	case *ast.TupleVarDeclStmt:
		valReg, valType, err := g.generateExpression(buf, s.Value)
		if err != nil {
			return err
		}

		tupPtr := valReg
		if !strings.HasSuffix(valType, "*") {
			tupAlloc := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = alloca %s\n", tupAlloc, valType))
			buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", valType, valReg, valType, tupAlloc))
			tupPtr = tupAlloc
		}

		structTypeName := strings.TrimSuffix(valType, "*")

		for idx, nameId := range s.Names {
			fieldPtr := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %s, %s* %s, i32 0, i32 %d\n",
				fieldPtr, structTypeName, structTypeName, tupPtr, idx))

			var fieldTypes []string
			if fTypes, ok := g.tupleTypes[strings.TrimPrefix(structTypeName, "%")]; ok {
				fieldTypes = fTypes
			}
			fieldType := "i64"
			if idx < len(fieldTypes) {
				fieldType = fieldTypes[idx]
			}

			fieldVal := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = load %s, %s* %s\n", fieldVal, fieldType, fieldType, fieldPtr))

			ptrReg := fmt.Sprintf("%%%s.addr", nameId.Value)
			buf.WriteString(fmt.Sprintf("    %s = alloca %s\n", ptrReg, fieldType))
			buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", fieldType, fieldVal, fieldType, ptrReg))

			g.symbolTable[nameId.Value] = fieldType
			g.varAllocaMap[nameId.Value] = ptrReg
		}

	case *ast.DeferStmt:
		g.deferredStmts = append(g.deferredStmts, s.Expr)

	case *ast.ReturnStmt:
		g.emitDeferred(buf)
		if s.Value != nil {
			valReg, valType, err := g.generateExpression(buf, s.Value)
			if err != nil {
				return err
			}
			fnRetType := g.fnReturnTypes[g.currentFn]
			if strings.HasSuffix(valType, "*") && fnRetType != "" && !strings.HasSuffix(fnRetType, "*") && valType == fnRetType+"*" {
				loadedValReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = load %s, %s %s\n", loadedValReg, fnRetType, valType, valReg))
				valReg = loadedValReg
				valType = fnRetType
			}
			if g.currentFn == "main" {
				if valType == "i64" {
					i32Reg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = trunc i64 %s to i32\n", i32Reg, valReg))
					buf.WriteString(fmt.Sprintf("    ret i32 %s\n", i32Reg))
				} else {
					buf.WriteString(fmt.Sprintf("    ret %s %s\n", valType, valReg))
				}
			} else {
				buf.WriteString(fmt.Sprintf("    ret %s %s\n", valType, valReg))
			}
		} else {
			if g.currentFn == "main" {
				buf.WriteString("    %fl = call i32 @fflush(i8* null)\n")
				buf.WriteString("    ret i32 0\n")
			} else {
				buf.WriteString("    ret void\n")
			}
		}

	case *ast.IfStmt:
		condReg, condType, err := g.generateExpression(buf, s.Condition)
		if err != nil {
			return err
		}

		if condType != "i1" {
			cmpReg := g.freshReg()
			if condType == "i8*" {
				buf.WriteString(fmt.Sprintf("    %s = icmp ne i8* %s, null\n", cmpReg, condReg))
			} else {
				buf.WriteString(fmt.Sprintf("    %s = icmp ne %s %s, 0\n", cmpReg, condType, condReg))
			}
			condReg = cmpReg
		}

		thenLabel := g.freshLabel("if.then")
		elseLabel := g.freshLabel("if.else")
		mergeLabel := g.freshLabel("if.merge")

		targetElse := mergeLabel
		if s.Alternative != nil {
			targetElse = elseLabel
		}

		buf.WriteString(fmt.Sprintf("    br i1 %s, label %%%s, label %%%s\n", condReg, thenLabel, targetElse))

		buf.WriteString(fmt.Sprintf("%s:\n", thenLabel))
		for _, bStmt := range s.Consequence.Statements {
			if err := g.generateBodyStatement(buf, bStmt); err != nil {
				return err
			}
		}
		buf.WriteString(fmt.Sprintf("    br label %%%s\n", mergeLabel))

		if s.Alternative != nil {
			buf.WriteString(fmt.Sprintf("%s:\n", elseLabel))
			for _, bStmt := range s.Alternative.Statements {
				if err := g.generateBodyStatement(buf, bStmt); err != nil {
					return err
				}
			}
			buf.WriteString(fmt.Sprintf("    br label %%%s\n", mergeLabel))
		}

		buf.WriteString(fmt.Sprintf("%s:\n", mergeLabel))

	case *ast.WhileStmt:
		condLabel := g.freshLabel("while.cond")
		bodyLabel := g.freshLabel("while.body")
		endLabel := g.freshLabel("while.end")

		oldCond := g.currentLoopCond
		oldEnd := g.currentLoopEnd
		g.currentLoopCond = condLabel
		g.currentLoopEnd = endLabel

		buf.WriteString(fmt.Sprintf("    br label %%%s\n", condLabel))
		buf.WriteString(fmt.Sprintf("%s:\n", condLabel))

		condReg, _, err := g.generateExpression(buf, s.Condition)
		if err != nil {
			return err
		}

		buf.WriteString(fmt.Sprintf("    br i1 %s, label %%%s, label %%%s\n", condReg, bodyLabel, endLabel))

		buf.WriteString(fmt.Sprintf("%s:\n", bodyLabel))
		for _, bStmt := range s.Body.Statements {
			if err := g.generateBodyStatement(buf, bStmt); err != nil {
				return err
			}
		}
		buf.WriteString(fmt.Sprintf("    br label %%%s\n", condLabel))

		buf.WriteString(fmt.Sprintf("%s:\n", endLabel))

		g.currentLoopCond = oldCond
		g.currentLoopEnd = oldEnd

	case *ast.ForInStmt:
		if rng, ok := s.Iterable.(*ast.RangeExpr); ok {
			startReg, _, err := g.generateExpression(buf, rng.Start)
			if err != nil {
				return err
			}
			endReg, _, err := g.generateExpression(buf, rng.End)
			if err != nil {
				return err
			}

			idxPtr := fmt.Sprintf("%%var_%s_%s.addr", s.VarName.Value, strings.TrimPrefix(g.freshReg(), "%"))
			buf.WriteString(fmt.Sprintf("    %s = alloca i64\n", idxPtr))
			buf.WriteString(fmt.Sprintf("    store i64 %s, i64* %s\n", startReg, idxPtr))

			condLabel := g.freshLabel("for.range.cond")
			bodyLabel := g.freshLabel("for.range.body")
			endLabel := g.freshLabel("for.range.end")

			buf.WriteString(fmt.Sprintf("    br label %%%s\n", condLabel))
			buf.WriteString(fmt.Sprintf("%s:\n", condLabel))

			curIdx := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = load i64, i64* %s\n", curIdx, idxPtr))
			g.symbolTable[s.VarName.Value] = "i64"
			g.varAllocaMap[s.VarName.Value] = idxPtr

			condReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = icmp slt i64 %s, %s\n", condReg, curIdx, endReg))
			buf.WriteString(fmt.Sprintf("    br i1 %s, label %%%s, label %%%s\n", condReg, bodyLabel, endLabel))

			buf.WriteString(fmt.Sprintf("%s:\n", bodyLabel))
			for _, bStmt := range s.Body.Statements {
				if err := g.generateBodyStatement(buf, bStmt); err != nil {
					return err
				}
			}

			nextIdx := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = add i64 %s, 1\n", nextIdx, curIdx))
			buf.WriteString(fmt.Sprintf("    store i64 %s, i64* %s\n", nextIdx, idxPtr))
			buf.WriteString(fmt.Sprintf("    br label %%%s\n", condLabel))

			buf.WriteString(fmt.Sprintf("%s:\n", endLabel))
			return nil
		}

		arrReg, arrType, err := g.generateExpression(buf, s.Iterable)
		if err != nil {
			return err
		}
		if arrType == "i64" {
			ptrReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = inttoptr i64 %s to %%struct.Array*\n", ptrReg, arrReg))
			arrReg = ptrReg
		}

		var elemType = "i64"
		if mem, ok := s.Iterable.(*ast.MemberExpr); ok {
			objReg, objType, _ := g.generateExpression(buf, mem.Object)
			_ = objReg
			structTypeName := strings.TrimPrefix(strings.TrimSuffix(objType, "*"), "%struct.")
			fieldIdx := g.getStructFieldIndex(structTypeName, mem.Member.Value)
			if fields, ok := g.structTypes[structTypeName]; ok && fieldIdx < len(fields) {
				if fields[fieldIdx].Type == "string[]" {
					elemType = "i8*"
				}
			}
		}

		idxPtr := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = alloca i64\n", idxPtr))
		buf.WriteString(fmt.Sprintf("    store i64 0, i64* %s\n", idxPtr))

		lenReg := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = call i64 @array_len(%%struct.Array* %s)\n", lenReg, arrReg))

		varPtr := fmt.Sprintf("%%var_%s_%s.addr", s.VarName.Value, strings.TrimPrefix(g.freshReg(), "%"))
		buf.WriteString(fmt.Sprintf("    %s = alloca %s\n", varPtr, elemType))
		g.symbolTable[s.VarName.Value] = elemType
		g.varAllocaMap[s.VarName.Value] = varPtr

		condLabel := g.freshLabel("for.cond")
		bodyLabel := g.freshLabel("for.body")
		endLabel := g.freshLabel("for.end")

		buf.WriteString(fmt.Sprintf("    br label %%%s\n", condLabel))
		buf.WriteString(fmt.Sprintf("%s:\n", condLabel))

		curIdx := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = load i64, i64* %s\n", curIdx, idxPtr))

		condReg := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = icmp slt i64 %s, %s\n", condReg, curIdx, lenReg))
		buf.WriteString(fmt.Sprintf("    br i1 %s, label %%%s, label %%%s\n", condReg, bodyLabel, endLabel))

		buf.WriteString(fmt.Sprintf("%s:\n", bodyLabel))
		elemReg := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = call i64 @array_get(%%struct.Array* %s, i64 %s)\n", elemReg, arrReg, curIdx))
		if elemType == "i8*" {
			strReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = inttoptr i64 %s to i8*\n", strReg, elemReg))
			buf.WriteString(fmt.Sprintf("    store i8* %s, i8** %s\n", strReg, varPtr))
		} else {
			buf.WriteString(fmt.Sprintf("    store i64 %s, i64* %s\n", elemReg, varPtr))
		}

		for _, bStmt := range s.Body.Statements {
			if err := g.generateBodyStatement(buf, bStmt); err != nil {
				return err
			}
		}

		nextIdx := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = add i64 %s, 1\n", nextIdx, curIdx))
		buf.WriteString(fmt.Sprintf("    store i64 %s, i64* %s\n", nextIdx, idxPtr))
		buf.WriteString(fmt.Sprintf("    br label %%%s\n", condLabel))

		buf.WriteString(fmt.Sprintf("%s:\n", endLabel))

	case *ast.MatchStmt:
		matchValReg, matchValType, err := g.generateExpression(buf, s.Expr)
		if err != nil {
			return err
		}

		matchValPtr := matchValReg
		structTypeName := strings.TrimPrefix(strings.TrimSuffix(matchValType, "*"), "%struct.")

		if !strings.HasSuffix(matchValType, "*") {
			matchValAlloc := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = alloca %s\n", matchValAlloc, matchValType))
			buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", matchValType, matchValReg, matchValType, matchValAlloc))
			matchValPtr = matchValAlloc
		}

		endLabel := g.freshLabel("match.end")

		for idx, c := range s.Cases {
			var condReg string
			var bindVarName string
			var bindValReg string
			var bindValType string

			caseBodyLabel := g.freshLabel("match.case.body")
			nextCaseLabel := g.freshLabel("match.case.next")

			if call, ok := c.Pattern.(*ast.CallExpr); ok {
				if fnId, ok := call.Function.(*ast.Identifier); ok {
					if fnId.Value == "Some" {
						flagPtr := g.freshReg()
						buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.Option, %%struct.Option* %s, i32 0, i32 0\n", flagPtr, matchValPtr))
						flagVal := g.freshReg()
						buf.WriteString(fmt.Sprintf("    %s = load i1, i1* %s\n", flagVal, flagPtr))
						condReg = flagVal

						if argId, ok := call.Arguments[0].(*ast.Identifier); ok {
							bindVarName = argId.Value
							valPtr := g.freshReg()
							buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.Option, %%struct.Option* %s, i32 0, i32 1\n", valPtr, matchValPtr))
							extractedVal := g.freshReg()
							buf.WriteString(fmt.Sprintf("    %s = load i64, i64* %s\n", extractedVal, valPtr))
							bindValReg = extractedVal
							bindValType = "i64"
						}
					} else if fnId.Value == "Ok" {
						flagPtr := g.freshReg()
						buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.Result, %%struct.Result* %s, i32 0, i32 0\n", flagPtr, matchValPtr))
						flagVal := g.freshReg()
						buf.WriteString(fmt.Sprintf("    %s = load i1, i1* %s\n", flagVal, flagPtr))
						condReg = flagVal

						if argId, ok := call.Arguments[0].(*ast.Identifier); ok {
							bindVarName = argId.Value
							valPtr := g.freshReg()
							buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.Result, %%struct.Result* %s, i32 0, i32 1\n", valPtr, matchValPtr))
							extractedVal := g.freshReg()
							buf.WriteString(fmt.Sprintf("    %s = load i64, i64* %s\n", extractedVal, valPtr))
							bindValReg = extractedVal
							bindValType = "i64"
						}
					} else if fnId.Value == "Err" {
						flagPtr := g.freshReg()
						buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.Result, %%struct.Result* %s, i32 0, i32 0\n", flagPtr, matchValPtr))
						flagVal := g.freshReg()
						buf.WriteString(fmt.Sprintf("    %s = load i1, i1* %s\n", flagVal, flagPtr))
						notFlagVal := g.freshReg()
						buf.WriteString(fmt.Sprintf("    %s = xor i1 %s, true\n", notFlagVal, flagVal))
						condReg = notFlagVal

						if argId, ok := call.Arguments[0].(*ast.Identifier); ok {
							bindVarName = argId.Value
							valPtr := g.freshReg()
							buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.Result, %%struct.Result* %s, i32 0, i32 2\n", valPtr, matchValPtr))
							extractedVal := g.freshReg()
							buf.WriteString(fmt.Sprintf("    %s = load i8*, i8** %s\n", extractedVal, valPtr))
							bindValReg = extractedVal
							bindValType = "i8*"
						}
					}
				}
			} else if patRng, ok := c.Pattern.(*ast.RangeExpr); ok {
				startReg, _, _ := g.generateExpression(buf, patRng.Start)
				endReg, _, _ := g.generateExpression(buf, patRng.End)
				c1 := g.freshReg()
				c2 := g.freshReg()
				resC := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = icmp sge i64 %s, %s\n", c1, matchValReg, startReg))
				buf.WriteString(fmt.Sprintf("    %s = icmp sle i64 %s, %s\n", c2, matchValReg, endReg))
				buf.WriteString(fmt.Sprintf("    %s = and i1 %s, %s\n", resC, c1, c2))
				condReg = resC
			} else if id, ok := c.Pattern.(*ast.Identifier); ok {
				if id.Value == "None" {
					flagPtr := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.Option, %%struct.Option* %s, i32 0, i32 0\n", flagPtr, matchValPtr))
					flagVal := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = load i1, i1* %s\n", flagVal, flagPtr))
					notFlagVal := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = xor i1 %s, true\n", notFlagVal, flagVal))
					condReg = notFlagVal
				} else if id.Value == "_" {
					condReg = "true"
				} else {
					bindVarName = id.Value
					bindValReg = matchValReg
					bindValType = matchValType
					condReg = "true"
				}
			}

			if condReg == "" {
				patReg, _, err := g.generateExpression(buf, c.Pattern)
				if err != nil {
					return err
				}
				condReg = g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = icmp eq %s %s, %s\n", condReg, matchValType, matchValReg, patReg))
			}

			targetNext := endLabel
			if idx+1 < len(s.Cases) {
				targetNext = nextCaseLabel
			}

			if c.Guard != nil {
				evalGuardLabel := g.freshLabel("match.guard.eval")
				if condReg == "true" {
					buf.WriteString(fmt.Sprintf("    br label %%%s\n", evalGuardLabel))
				} else {
					buf.WriteString(fmt.Sprintf("    br i1 %s, label %%%s, label %%%s\n", condReg, evalGuardLabel, targetNext))
				}
				buf.WriteString(fmt.Sprintf("%s:\n", evalGuardLabel))

				if bindVarName != "" {
					bindPtr := fmt.Sprintf("%%var_%s_%s.addr", bindVarName, strings.TrimPrefix(g.freshReg(), "%"))
					buf.WriteString(fmt.Sprintf("    %s = alloca %s\n", bindPtr, bindValType))
					buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", bindValType, bindValReg, bindValType, bindPtr))
					g.symbolTable[bindVarName] = bindValType
					g.varAllocaMap[bindVarName] = bindPtr
				}

				guardReg, _, err := g.generateExpression(buf, c.Guard)
				if err != nil {
					return err
				}
				buf.WriteString(fmt.Sprintf("    br i1 %s, label %%%s, label %%%s\n", guardReg, caseBodyLabel, targetNext))

				buf.WriteString(fmt.Sprintf("%s:\n", caseBodyLabel))
				if err := g.generateBodyStatement(buf, c.Body); err != nil {
					return err
				}
			} else {
				if condReg == "true" {
					buf.WriteString(fmt.Sprintf("    br label %%%s\n", caseBodyLabel))
				} else {
					buf.WriteString(fmt.Sprintf("    br i1 %s, label %%%s, label %%%s\n", condReg, caseBodyLabel, targetNext))
				}

				buf.WriteString(fmt.Sprintf("%s:\n", caseBodyLabel))
				if bindVarName != "" {
					bindPtr := fmt.Sprintf("%%var_%s_%s.addr", bindVarName, strings.TrimPrefix(g.freshReg(), "%"))
					buf.WriteString(fmt.Sprintf("    %s = alloca %s\n", bindPtr, bindValType))
					buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", bindValType, bindValReg, bindValType, bindPtr))
					g.symbolTable[bindVarName] = bindValType
					g.varAllocaMap[bindVarName] = bindPtr
				}

				if err := g.generateBodyStatement(buf, c.Body); err != nil {
					return err
				}
			}
			if !isReturnStmt(c.Body) {
				buf.WriteString(fmt.Sprintf("    br label %%%s\n", endLabel))
			}

			if idx+1 < len(s.Cases) {
				buf.WriteString(fmt.Sprintf("%s:\n", nextCaseLabel))
			}
		}

		buf.WriteString(fmt.Sprintf("%s:\n", endLabel))
		_ = structTypeName

	case *ast.SpawnStmt:
		fnPtr, _, err := g.generateExpression(buf, s.Call)
		if err != nil {
			return err
		}
		if strings.HasPrefix(fnPtr, "@") {
			buf.WriteString(fmt.Sprintf("    call void %s()\n", fnPtr))
		}
		return nil

	case *ast.ExprStmt:
		if id, ok := s.Expr.(*ast.Identifier); ok {
			if id.Value == "break" && g.currentLoopEnd != "" {
				buf.WriteString(fmt.Sprintf("    br label %%%s\n", g.currentLoopEnd))
				return nil
			}
			if id.Value == "continue" && g.currentLoopCond != "" {
				buf.WriteString(fmt.Sprintf("    br label %%%s\n", g.currentLoopCond))
				return nil
			}
		}

		if infix, ok := s.Expr.(*ast.InfixExpr); ok && infix.Operator == "=" {
			if id, ok := infix.Left.(*ast.Identifier); ok {
				valReg, valType, err := g.generateExpression(buf, infix.Right)
				if err != nil {
					return err
				}
				ptrReg := g.varAllocaMap[id.Value]
				if strings.HasSuffix(valType, "*") && strings.HasPrefix(valType, "%struct.") {
					loadedVal := g.freshReg()
					pureType := strings.TrimSuffix(valType, "*")
					buf.WriteString(fmt.Sprintf("    %s = load %s, %s* %s\n", loadedVal, pureType, pureType, valReg))
					valReg = loadedVal
					valType = pureType
				}
				buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", valType, valReg, valType, ptrReg))
				return nil
			} else if mem, ok := infix.Left.(*ast.MemberExpr); ok {
				if objId, ok := mem.Object.(*ast.Identifier); ok {
					valReg, valType, err := g.generateExpression(buf, infix.Right)
					if err != nil {
						return err
					}
					varType := g.symbolTable[objId.Value]
					structTypeName := strings.TrimPrefix(strings.TrimSuffix(varType, "*"), "%struct.")
					fieldIdx := g.getStructFieldIndex(structTypeName, mem.Member.Value)
					fieldPtrReg := g.freshReg()
					objPtrReg, _, err := g.generateExpression(buf, objId)
					if err != nil {
						return err
					}
					buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.%s, %%struct.%s* %s, i32 0, i32 %d\n",
						fieldPtrReg, structTypeName, structTypeName, objPtrReg, fieldIdx))
					buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", valType, valReg, valType, fieldPtrReg))
					return nil
				}
			}
		}
		_, _, err := g.generateExpression(buf, s.Expr)
		return err

	default:
		return fmt.Errorf("unsupported LLVM statement: %T", stmt)
	}

	return nil
}

func (g *LLVMGenerator) getStructFieldIndex(structName, fieldName string) int {
	fields, ok := g.structTypes[structName]
	if !ok {
		return 0
	}
	for i, f := range fields {
		if f.Name.Value == fieldName {
			return i
		}
	}
	return 0
}

func isReturnStmt(stmt ast.Statement) bool {
	_, ok := stmt.(*ast.ReturnStmt)
	return ok
}

func (g *LLVMGenerator) generateExpression(buf *bytes.Buffer, expr ast.Expression) (string, string, error) {
	switch e := expr.(type) {
	case *ast.TryExpr:
		valReg, valType, err := g.generateExpression(buf, e.Expr)
		if err != nil {
			return "", "", err
		}

		structTypeName := strings.TrimPrefix(strings.TrimSuffix(valType, "*"), "%struct.")
		valPtr := valReg
		if !strings.HasSuffix(valType, "*") {
			valAlloc := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = alloca %s\n", valAlloc, valType))
			buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", valType, valReg, valType, valAlloc))
			valPtr = valAlloc
		}

		flagPtr := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.%s, %%struct.%s* %s, i32 0, i32 0\n",
			flagPtr, structTypeName, structTypeName, valPtr))
		flagVal := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = load i1, i1* %s\n", flagVal, flagPtr))

		okLabel := g.freshLabel("try.ok")
		failLabel := g.freshLabel("try.fail")

		buf.WriteString(fmt.Sprintf("    br i1 %s, label %%%s, label %%%s\n", flagVal, okLabel, failLabel))

		buf.WriteString(fmt.Sprintf("%s:\n", failLabel))
		resVal := g.freshReg()
		if g.currentFn == "main" {
			buf.WriteString("    ret i32 0\n")
		} else {
			buf.WriteString(fmt.Sprintf("    %s = load %s, %s* %s\n", resVal, strings.TrimSuffix(valType, "*"), strings.TrimSuffix(valType, "*"), valPtr))
			buf.WriteString(fmt.Sprintf("    ret %s %s\n", strings.TrimSuffix(valType, "*"), resVal))
		}

		buf.WriteString(fmt.Sprintf("%s:\n", okLabel))
		okValPtr := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.%s, %%struct.%s* %s, i32 0, i32 1\n",
			okValPtr, structTypeName, structTypeName, valPtr))
		okVal := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = load i64, i64* %s\n", okVal, okValPtr))

		return okVal, "i64", nil

	case *ast.IntegerLiteral:
		return fmt.Sprintf("%d", e.Value), "i64", nil

	case *ast.FloatLiteral:
		return fmt.Sprintf("%f", e.Value), "double", nil

	case *ast.BoolLiteral:
		if e.Value {
			return "true", "i1", nil
		}
		return "false", "i1", nil

	case *ast.StringLiteral:
		globName := g.getGlobalString(e.Value)
		lenBytes := g.globalStringLens[globName]
		if lenBytes == 0 {
			lenBytes = len(e.Value) + 1
		}
		reg := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds [%d x i8], [%d x i8]* %s, i64 0, i64 0\n",
			reg, lenBytes, lenBytes, globName))
		return reg, "i8*", nil

	case *ast.FStringLiteral:
		if len(e.Parts) == 0 {
			return g.generateExpression(buf, &ast.StringLiteral{Value: ""})
		}
		curReg, curType, err := g.generateExpression(buf, e.Parts[0])
		if err != nil {
			return "", "", err
		}
		if curType == "i64" {
			sReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i8* @str_int(i64 %s)\n", sReg, curReg))
			curReg = sReg
		} else if curType == "i1" {
			sReg := g.freshReg()
			trueGlob := g.getGlobalString("true")
			falseGlob := g.getGlobalString("false")
			tLen := g.globalStringLens[trueGlob]
			fLen := g.globalStringLens[falseGlob]
			tReg := g.freshReg()
			fReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds [%d x i8], [%d x i8]* %s, i64 0, i64 0\n", tReg, tLen, tLen, trueGlob))
			buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds [%d x i8], [%d x i8]* %s, i64 0, i64 0\n", fReg, fLen, fLen, falseGlob))
			buf.WriteString(fmt.Sprintf("    %s = select i1 %s, i8* %s, i8* %s\n", sReg, curReg, tReg, fReg))
			curReg = sReg
		} else if curType != "i8*" {
			sReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i8* @str_int(i64 1)\n", sReg))
			curReg = sReg
		}

		for i := 1; i < len(e.Parts); i++ {
			nextReg, nextType, err := g.generateExpression(buf, e.Parts[i])
			if err != nil {
				return "", "", err
			}
			if nextType == "i64" {
				sReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i8* @str_int(i64 %s)\n", sReg, nextReg))
				nextReg = sReg
			} else if nextType == "i1" {
				sReg := g.freshReg()
				trueGlob := g.getGlobalString("true")
				falseGlob := g.getGlobalString("false")
				tLen := g.globalStringLens[trueGlob]
				fLen := g.globalStringLens[falseGlob]
				tReg := g.freshReg()
				fReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds [%d x i8], [%d x i8]* %s, i64 0, i64 0\n", tReg, tLen, tLen, trueGlob))
				buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds [%d x i8], [%d x i8]* %s, i64 0, i64 0\n", fReg, fLen, fLen, falseGlob))
				buf.WriteString(fmt.Sprintf("    %s = select i1 %s, i8* %s, i8* %s\n", sReg, nextReg, tReg, fReg))
				nextReg = sReg
			} else if nextType != "i8*" {
				sReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i8* @str_int(i64 1)\n", sReg))
				nextReg = sReg
			}
			resReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i8* @concat_strings(i8* %s, i8* %s)\n", resReg, curReg, nextReg))
			curReg = resReg
		}
		return curReg, "i8*", nil

	case *ast.ArrayLiteral:
		capVal := len(e.Elements)
		if capVal < 4 {
			capVal = 4
		}
		arrReg := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = call %%struct.Array* @create_array(i64 %d)\n", arrReg, capVal))
		for _, elem := range e.Elements {
			elReg, elType, err := g.generateExpression(buf, elem)
			if err != nil {
				return "", "", err
			}
			if elType != "i64" {
				intReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = ptrtoint %s %s to i64\n", intReg, elType, elReg))
				elReg = intReg
			}
			buf.WriteString(fmt.Sprintf("    call void @array_append(%%struct.Array* %s, i64 %s)\n", arrReg, elReg))
		}
		return arrReg, "%struct.Array*", nil

	case *ast.MapLiteral:
		capVal := len(e.Keys) * 2
		if capVal < 4 {
			capVal = 4
		}
		arrReg := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = call %%struct.Array* @create_array(i64 %d)\n", arrReg, capVal))
		for i := 0; i < len(e.Keys); i++ {
			kReg, kType, err := g.generateExpression(buf, e.Keys[i])
			if err != nil {
				return "", "", err
			}
			vReg, _, err := g.generateExpression(buf, e.Values[i])
			if err != nil {
				return "", "", err
			}
			if kType == "i8*" {
				ptrInt := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = ptrtoint i8* %s to i64\n", ptrInt, kReg))
				kReg = ptrInt
			}
			buf.WriteString(fmt.Sprintf("    call void @array_append(%%struct.Array* %s, i64 %s)\n", arrReg, kReg))
			buf.WriteString(fmt.Sprintf("    call void @array_append(%%struct.Array* %s, i64 %s)\n", arrReg, vReg))
		}
		return arrReg, "%struct.Array*", nil

	case *ast.TupleLiteral:
		var elRegs []string
		var elTypes []string
		for _, el := range e.Elements {
			r, t, err := g.generateExpression(buf, el)
			if err != nil {
				return "", "", err
			}
			elRegs = append(elRegs, r)
			elTypes = append(elTypes, t)
		}

		tupTypeName := fmt.Sprintf("struct.Tuple_%d_%s", len(e.Elements), strings.Join(elTypes, "_"))
		tupTypeName = strings.ReplaceAll(tupTypeName, "*", "p")

		g.tupleTypes[tupTypeName] = elTypes
		structType := "%" + tupTypeName

		rawPtrReg := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = alloca %s\n", rawPtrReg, structType))
		for idx, r := range elRegs {
			t := elTypes[idx]
			fieldPtrReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %s, %s* %s, i32 0, i32 %d\n",
				fieldPtrReg, structType, structType, rawPtrReg, idx))
			buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", t, r, t, fieldPtrReg))
		}
		valReg := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = load %s, %s* %s\n", valReg, structType, structType, rawPtrReg))
		return valReg, structType, nil

	case *ast.TupleIndexExpr:
		tupReg, tupType, err := g.generateExpression(buf, e.Left)
		if err != nil {
			return "", "", err
		}

		structTypeName := strings.TrimSuffix(tupType, "*")
		tupPtr := tupReg
		if !strings.HasSuffix(tupType, "*") {
			tupAlloc := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = alloca %s\n", tupAlloc, structTypeName))
			buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", structTypeName, tupReg, structTypeName, tupAlloc))
			tupPtr = tupAlloc
		}

		fieldPtrReg := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %s, %s* %s, i32 0, i32 %d\n",
			fieldPtrReg, structTypeName, structTypeName, tupPtr, e.Index))

		var fieldTypes []string
		if fTypes, ok := g.tupleTypes[strings.TrimPrefix(structTypeName, "%")]; ok {
			fieldTypes = fTypes
		}
		fieldType := "i64"
		if e.Index < len(fieldTypes) {
			fieldType = fieldTypes[e.Index]
		}

		valReg := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = load %s, %s* %s\n", valReg, fieldType, fieldType, fieldPtrReg))
		return valReg, fieldType, nil

	case *ast.IndexExpr:
		arrReg, arrType, err := g.generateExpression(buf, e.Left)
		if err != nil {
			return "", "", err
		}
		if rng, ok := e.Index.(*ast.RangeExpr); ok {
			var startReg = "0"
			var endReg string
			if rng.Start != nil {
				sR, _, err := g.generateExpression(buf, rng.Start)
				if err != nil {
					return "", "", err
				}
				if sR != "" {
					startReg = sR
				}
			}
			if rng.End != nil {
				eR, _, err := g.generateExpression(buf, rng.End)
				if err != nil {
					return "", "", err
				}
				if eR != "" {
					endReg = eR
				}
			}
			if endReg == "" {
				if arrType == "i8*" {
					lenR := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = call i64 @strlen(i8* %s)\n", lenR, arrReg))
					endReg = lenR
				} else {
					lenR := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = call i64 @array_len(%%struct.Array* %s)\n", lenR, arrReg))
					endReg = lenR
				}
			}
			if arrType == "i8*" {
				subPtr := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds i8, i8* %s, i64 %s\n", subPtr, arrReg, startReg))
				lenReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = sub i64 %s, %s\n", lenReg, endReg, startReg))
				memReg := g.freshReg()
				lenPlus1 := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = add i64 %s, 1\n", lenPlus1, lenReg))
				buf.WriteString(fmt.Sprintf("    %s = call i8* @malloc(i64 %s)\n", memReg, lenPlus1))
				buf.WriteString(fmt.Sprintf("    %%tmp_cp_%d = call i8* @strncpy(i8* %s, i8* %s, i64 %s)\n", g.regCounter, memReg, subPtr, lenReg))
				g.regCounter++
				nullPtr := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds i8, i8* %s, i64 %s\n", nullPtr, memReg, lenReg))
				buf.WriteString(fmt.Sprintf("    store i8 0, i8* %s\n", nullPtr))
				return memReg, "i8*", nil
			} else {
				if arrType == "i64" {
					ptrReg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = inttoptr i64 %s to %%struct.Array*\n", ptrReg, arrReg))
					arrReg = ptrReg
				}
				lenReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = sub i64 %s, %s\n", lenReg, endReg, startReg))
				resArrReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call %%struct.Array* @create_array(i64 %s)\n", resArrReg, lenReg))

				idxPtr := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = alloca i64\n", idxPtr))
				buf.WriteString(fmt.Sprintf("    store i64 %s, i64* %s\n", startReg, idxPtr))

				condLabel := g.freshLabel("slice.cond")
				bodyLabel := g.freshLabel("slice.body")
				endLabel := g.freshLabel("slice.end")

				buf.WriteString(fmt.Sprintf("    br label %%%s\n", condLabel))
				buf.WriteString(fmt.Sprintf("%s:\n", condLabel))

				curIdx := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = load i64, i64* %s\n", curIdx, idxPtr))
				condReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = icmp slt i64 %s, %s\n", condReg, curIdx, endReg))
				buf.WriteString(fmt.Sprintf("    br i1 %s, label %%%s, label %%%s\n", condReg, bodyLabel, endLabel))

				buf.WriteString(fmt.Sprintf("%s:\n", bodyLabel))
				elemReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i64 @array_get(%%struct.Array* %s, i64 %s)\n", elemReg, arrReg, curIdx))
				buf.WriteString(fmt.Sprintf("    call void @array_append(%%struct.Array* %s, i64 %s)\n", resArrReg, elemReg))

				nextIdx := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = add i64 %s, 1\n", nextIdx, curIdx))
				buf.WriteString(fmt.Sprintf("    store i64 %s, i64* %s\n", nextIdx, idxPtr))
				buf.WriteString(fmt.Sprintf("    br label %%%s\n", condLabel))

				buf.WriteString(fmt.Sprintf("%s:\n", endLabel))
				return resArrReg, "%struct.Array*", nil
			}
		}

		idxReg, idxType, err := g.generateExpression(buf, e.Index)
		if err != nil {
			return "", "", err
		}
		valReg := g.freshReg()
		if arrType == "i8*" {
			ptrReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds i8, i8* %s, i64 %s\n", ptrReg, arrReg, idxReg))
			byteReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = load i8, i8* %s\n", byteReg, ptrReg))
			buf.WriteString(fmt.Sprintf("    %s = sext i8 %s to i64\n", valReg, byteReg))
			return valReg, "i64", nil
		}
		if arrType == "i64" {
			ptrReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = inttoptr i64 %s to %%struct.Array*\n", ptrReg, arrReg))
			arrReg = ptrReg
			arrType = "%struct.Array*"
		}
		if idxType == "i8*" {
			buf.WriteString(fmt.Sprintf("    %s = call i64 @map_get(%%struct.Array* %s, i8* %s)\n", valReg, arrReg, idxReg))
		} else {
			buf.WriteString(fmt.Sprintf("    %s = call i64 @array_get(%%struct.Array* %s, i64 %s)\n", valReg, arrReg, idxReg))
		}
		if mem, ok := e.Left.(*ast.MemberExpr); ok {
			objReg, objType, _ := g.generateExpression(buf, mem.Object)
			_ = objReg
			structTypeName := strings.TrimPrefix(strings.TrimSuffix(objType, "*"), "%struct.")
			fieldIdx := g.getStructFieldIndex(structTypeName, mem.Member.Value)
			if fields, ok := g.structTypes[structTypeName]; ok && fieldIdx < len(fields) {
				if fields[fieldIdx].Type == "string[]" {
					strReg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = inttoptr i64 %s to i8*\n", strReg, valReg))
					return strReg, "i8*", nil
				}
			}
		}
		return valReg, "i64", nil

	case *ast.Identifier:
		if e.Value == "_" {
			return "_", "wildcard", nil
		}
		if e.Value == "None" {
			resReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call %%struct.Option @None()\n", resReg))
			return resReg, "%struct.Option", nil
		}

		ptrReg, ok := g.varAllocaMap[e.Value]
		if !ok {
			return "", "", fmt.Errorf("undefined variable: %s", e.Value)
		}
		varType := g.symbolTable[e.Value]
		valReg := g.freshReg()
		if strings.HasPrefix(varType, "%struct.") && !strings.HasSuffix(varType, "*") {
			buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %s, %s* %s, i32 0\n", valReg, varType, varType, ptrReg))
			return valReg, varType + "*", nil
		}
		buf.WriteString(fmt.Sprintf("    %s = load %s, %s* %s\n", valReg, varType, varType, ptrReg))
		retType := varType
		if strings.HasPrefix(varType, "%struct.") && strings.HasSuffix(varType, "*") && varType != "%struct.Array*" {
			retType = strings.TrimSuffix(varType, "*")
		}
		return valReg, retType, nil

	case *ast.MemberExpr:
		objReg, objType, err := g.generateExpression(buf, e.Object)
		if err != nil {
			return "", "", err
		}

		if objType == "i8*" && e.Member.Value == "len" {
			lenReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i64 @strlen(i8* %s)\n", lenReg, objReg))
			return lenReg, "i64", nil
		}
		if objType == "%struct.Array*" && e.Member.Value == "len" {
			lenReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i64 @array_len(%%struct.Array* %s)\n", lenReg, objReg))
			return lenReg, "i64", nil
		}

		structTypeName := strings.TrimPrefix(strings.TrimSuffix(objType, "*"), "%struct.")
		if structTypeName == "i8" || structTypeName == "" || g.structTypes[structTypeName] == nil {
			if objId, ok := e.Object.(*ast.Identifier); ok {
				varType := g.symbolTable[objId.Value]
				if strings.HasPrefix(varType, "%struct.") {
					structTypeName = strings.TrimSuffix(strings.TrimPrefix(varType, "%struct."), "*")
				}
			}
		}
		fieldIdx := g.getStructFieldIndex(structTypeName, e.Member.Value)
		fieldPtrReg := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.%s, %%struct.%s* %s, i32 0, i32 %d\n",
			fieldPtrReg, structTypeName, structTypeName, objReg, fieldIdx))
		valReg := g.freshReg()
		fieldType := "i64"
		if fields, ok := g.structTypes[structTypeName]; ok && fieldIdx < len(fields) {
			fieldType = mapLLVMType(fields[fieldIdx].Type)
		}
		buf.WriteString(fmt.Sprintf("    %s = load %s, %s* %s\n", valReg, fieldType, fieldType, fieldPtrReg))
		return valReg, fieldType, nil

	case *ast.PrefixExpr:
		rightReg, rightType, err := g.generateExpression(buf, e.Right)
		if err != nil {
			return "", "", err
		}
		if e.Operator == "-" {
			resReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = sub i64 0, %s\n", resReg, rightReg))
			return resReg, rightType, nil
		}
		if e.Operator == "!" || e.Operator == "not" {
			if rightType == "i1" {
				resReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = xor i1 %s, true\n", resReg, rightReg))
				return resReg, "i1", nil
			} else {
				cmpReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = icmp eq %s %s, 0\n", cmpReg, rightType, rightReg))
				return cmpReg, "i1", nil
			}
		}
		return "", "", fmt.Errorf("unsupported prefix operator: %s", e.Operator)

	case *ast.AwaitExpr:
		valReg, valType, err := g.generateExpression(buf, e.Expr)
		if err != nil {
			return "", "", err
		}
		return valReg, valType, nil

	case *ast.AsmExpr:
		buf.WriteString(fmt.Sprintf("    call void asm sideeffect %q, \"\"()\n", e.Instruction))
		return "0", "i64", nil

	case *ast.InfixExpr:
		if e.Operator == "=" {
			rReg, rType, err := g.generateExpression(buf, e.Right)
			if err != nil {
				return "", "", err
			}
			if id, ok := e.Left.(*ast.Identifier); ok {
				ptrReg := g.varAllocaMap[id.Value]
				buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", rType, rReg, rType, ptrReg))
			} else if mem, ok := e.Left.(*ast.MemberExpr); ok {
				if objId, ok := mem.Object.(*ast.Identifier); ok {
					structTypeName := g.symbolTable[objId.Value]
					if strings.HasPrefix(structTypeName, "%struct.") {
						structPure := strings.TrimPrefix(structTypeName, "%struct.")
						structPure = strings.TrimSuffix(structPure, "*")
						fields := g.structTypes[structPure]
						fieldIdx := 0
						for idx, f := range fields {
							if f.Name.Value == mem.Member.Value {
								fieldIdx = idx
								break
							}
						}
						objPtr := g.varAllocaMap[objId.Value]
						if strings.HasSuffix(structTypeName, "*") {
							loadedPtr := g.freshReg()
							buf.WriteString(fmt.Sprintf("    %s = load %%struct.%s*, %%struct.%s** %s\n", loadedPtr, structPure, structPure, objPtr))
							objPtr = loadedPtr
						}
						fieldPtr := g.freshReg()
						buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.%s, %%struct.%s* %s, i32 0, i32 %d\n",
							fieldPtr, structPure, structPure, objPtr, fieldIdx))
						buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", rType, rReg, rType, fieldPtr))
					}
				}
			}
			return rReg, rType, nil
		}

		leftReg, leftType, err := g.generateExpression(buf, e.Left)
		if err != nil {
			return "", "", err
		}
		rightReg, rightType, err := g.generateExpression(buf, e.Right)
		if err != nil {
			return "", "", err
		}

		if strings.HasPrefix(leftType, "%struct.") && leftType != "%struct.Array*" && leftType != "%struct.Array" {
			structTypeName := strings.TrimSuffix(strings.TrimPrefix(leftType, "%struct."), "*")
			var methodName string
			switch e.Operator {
			case "+":
				methodName = "add"
			case "-":
				methodName = "sub"
			case "*":
				methodName = "mul"
			case "/":
				methodName = "div"
			}
			if methodName != "" {
				if strings.HasSuffix(leftType, "*") {
					valReg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = load %%struct.%s, %%struct.%s* %s\n", valReg, structTypeName, structTypeName, leftReg))
					leftReg = valReg
				}
				if strings.HasSuffix(rightType, "*") {
					valReg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = load %%struct.%s, %%struct.%s* %s\n", valReg, structTypeName, structTypeName, rightReg))
					rightReg = valReg
				}
				resReg := g.freshReg()
				mName := fmt.Sprintf("@%s_%s", structTypeName, methodName)
				buf.WriteString(fmt.Sprintf("    %s = call %%struct.%s %s(%%struct.%s %s, %%struct.%s %s)\n",
					resReg, structTypeName, mName, structTypeName, leftReg, structTypeName, rightReg))
				return resReg, fmt.Sprintf("%%struct.%s", structTypeName), nil
			}
		}

		if e.Operator == "+" && (leftType == "i8*" || rightType == "i8*" || leftType == "ptr" || rightType == "ptr") {
			if leftType == "i64" {
				cStr := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i8* @char_to_str(i64 %s)\n", cStr, leftReg))
				leftReg = cStr
				leftType = "i8*"
			}
			if rightType == "i64" {
				cStr := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i8* @char_to_str(i64 %s)\n", cStr, rightReg))
				rightReg = cStr
				rightType = "i8*"
			}
			if leftType == "ptr" {
				pReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = bitcast ptr %s to i8*\n", pReg, leftReg))
				leftReg = pReg
			}
			if rightType == "ptr" {
				pReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = bitcast ptr %s to i8*\n", pReg, rightReg))
				rightReg = pReg
			}
			resReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i8* @concat_strings(i8* %s, i8* %s)\n", resReg, leftReg, rightReg))
			return resReg, "i8*", nil
		}

		if (leftType == "i8*" || rightType == "i8*") && (e.Operator == ">=" || e.Operator == "<=" || e.Operator == ">" || e.Operator == "<") {
			if leftType == "i64" {
				cStr := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i8* @char_to_str(i64 %s)\n", cStr, leftReg))
				leftReg = cStr
				leftType = "i8*"
			}
			if rightType == "i64" {
				cStr := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i8* @char_to_str(i64 %s)\n", cStr, rightReg))
				rightReg = cStr
				rightType = "i8*"
			}
			cmpReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i32 @strcmp(i8* %s, i8* %s)\n", cmpReg, leftReg, rightReg))
			resReg := g.freshReg()
			switch e.Operator {
			case ">=":
				buf.WriteString(fmt.Sprintf("    %s = icmp sge i32 %s, 0\n", resReg, cmpReg))
			case "<=":
				buf.WriteString(fmt.Sprintf("    %s = icmp sle i32 %s, 0\n", resReg, cmpReg))
			case ">":
				buf.WriteString(fmt.Sprintf("    %s = icmp sgt i32 %s, 0\n", resReg, cmpReg))
			case "<":
				buf.WriteString(fmt.Sprintf("    %s = icmp slt i32 %s, 0\n", resReg, cmpReg))
			}
			return resReg, "i1", nil
		}

		if leftType == "i64" && rightType == "i8*" {
			byteReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = load i8, i8* %s\n", byteReg, rightReg))
			extReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = sext i8 %s to i64\n", extReg, byteReg))
			rightReg = extReg
			rightType = "i64"
		}
		if leftType == "i8*" && rightType == "i64" {
			byteReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = load i8, i8* %s\n", byteReg, leftReg))
			extReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = sext i8 %s to i64\n", extReg, byteReg))
			leftReg = extReg
			leftType = "i64"
		}

		resReg := g.freshReg()

		switch e.Operator {
		case "+":
			buf.WriteString(fmt.Sprintf("    %s = add i64 %s, %s\n", resReg, leftReg, rightReg))
			return resReg, "i64", nil
		case "-":
			buf.WriteString(fmt.Sprintf("    %s = sub i64 %s, %s\n", resReg, leftReg, rightReg))
			return resReg, "i64", nil
		case "*":
			buf.WriteString(fmt.Sprintf("    %s = mul i64 %s, %s\n", resReg, leftReg, rightReg))
			return resReg, "i64", nil
		case "/":
			buf.WriteString(fmt.Sprintf("    %s = sdiv i64 %s, %s\n", resReg, leftReg, rightReg))
			return resReg, "i64", nil
		case "%":
			buf.WriteString(fmt.Sprintf("    %s = srem i64 %s, %s\n", resReg, leftReg, rightReg))
			return resReg, "i64", nil
		case "==":
			if leftType == "i8*" && rightType == "i8*" {
				cmpReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i32 @strcmp(i8* %s, i8* %s)\n", cmpReg, leftReg, rightReg))
				eqReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = icmp eq i32 %s, 0\n", eqReg, cmpReg))
				return eqReg, "i1", nil
			}
			buf.WriteString(fmt.Sprintf("    %s = icmp eq %s %s, %s\n", resReg, leftType, leftReg, rightReg))
			return resReg, "i1", nil
		case "&&", "and":
			buf.WriteString(fmt.Sprintf("    %s = and i1 %s, %s\n", resReg, leftReg, rightReg))
			return resReg, "i1", nil
		case "||", "or":
			buf.WriteString(fmt.Sprintf("    %s = or i1 %s, %s\n", resReg, leftReg, rightReg))
			return resReg, "i1", nil
		case "??":
			cmpReg := g.freshReg()
			if leftType == "i8*" || strings.HasSuffix(leftType, "*") {
				buf.WriteString(fmt.Sprintf("    %s = icmp ne %s %s, null\n", cmpReg, leftType, leftReg))
			} else {
				buf.WriteString(fmt.Sprintf("    %s = icmp ne %s %s, 0\n", cmpReg, leftType, leftReg))
			}
			resReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = select i1 %s, %s %s, %s %s\n", resReg, cmpReg, leftType, leftReg, rightType, rightReg))
			return resReg, leftType, nil
		case "!=":
			if leftType == "i8*" && rightType == "i8*" {
				cmpReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i32 @strcmp(i8* %s, i8* %s)\n", cmpReg, leftReg, rightReg))
				neqReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = icmp ne i32 %s, 0\n", neqReg, cmpReg))
				return neqReg, "i1", nil
			}
			buf.WriteString(fmt.Sprintf("    %s = icmp ne %s %s, %s\n", resReg, leftType, leftReg, rightReg))
			return resReg, "i1", nil
		case "<":
			buf.WriteString(fmt.Sprintf("    %s = icmp slt %s %s, %s\n", resReg, leftType, leftReg, rightReg))
			return resReg, "i1", nil
		case "<=":
			buf.WriteString(fmt.Sprintf("    %s = icmp sle %s %s, %s\n", resReg, leftType, leftReg, rightReg))
			return resReg, "i1", nil
		case ">":
			buf.WriteString(fmt.Sprintf("    %s = icmp sgt %s %s, %s\n", resReg, leftType, leftReg, rightReg))
			return resReg, "i1", nil
		case ">=":
			buf.WriteString(fmt.Sprintf("    %s = icmp sge %s %s, %s\n", resReg, leftType, leftReg, rightReg))
			return resReg, "i1", nil
		default:
			return "", "", fmt.Errorf("unsupported operator: %s", e.Operator)
		}

	case *ast.FnLiteral:
		g.labelCounter++
		anonName := fmt.Sprintf("anon_fn_%d", g.labelCounter)

		var paramStrs []string
		for _, p := range e.Params {
			pType := mapLLVMType(p.Type)
			if pType == "auto" || pType == "void" || pType == "" {
				pType = "i64"
			}
			paramStrs = append(paramStrs, fmt.Sprintf("%s %%%s.arg", pType, p.Name.Value))
		}

		retType := mapLLVMType(e.ReturnType)
		if retType == "void" || retType == "auto" || retType == "" {
			retType = "i64"
		}

		var fnBuf bytes.Buffer
		fnBuf.WriteString(fmt.Sprintf("define %s @%s(%s) {\n", retType, anonName, strings.Join(paramStrs, ", ")))
		fnBuf.WriteString("entry:\n")

		subGen := New()
		subGen.regCounter = 0
		subGen.strCounter = g.strCounter
		subGen.symbolTable = make(map[string]string)
		subGen.varAllocaMap = make(map[string]string)
		subGen.globalStrings = g.globalStrings
		subGen.structTypes = g.structTypes

		for _, p := range e.Params {
			pType := mapLLVMType(p.Type)
			if pType == "auto" || pType == "void" || pType == "" {
				pType = "i64"
			}
			ptrReg := fmt.Sprintf("%%%s.addr", p.Name.Value)
			fnBuf.WriteString(fmt.Sprintf("    %s = alloca %s\n", ptrReg, pType))
			fnBuf.WriteString(fmt.Sprintf("    store %s %%%s.arg, %s* %s\n", pType, p.Name.Value, pType, ptrReg))
			subGen.symbolTable[p.Name.Value] = pType
			subGen.varAllocaMap[p.Name.Value] = ptrReg
		}

		for _, bStmt := range e.Body.Statements {
			if err := subGen.generateBodyStatement(&fnBuf, bStmt); err != nil {
				return "", "", err
			}
		}
		g.strCounter = subGen.strCounter
		if retType == "void" {
			fnBuf.WriteString("    ret void\n")
		} else if !strings.Contains(fnBuf.String(), "ret ") {
			fnBuf.WriteString("    ret i64 0\n")
		}
		fnBuf.WriteString("}\n\n")

		var paramTypes []string
		for _, p := range e.Params {
			pType := mapLLVMType(p.Type)
			if pType == "auto" || pType == "void" || pType == "" {
				pType = "i64"
			}
			paramTypes = append(paramTypes, pType)
		}

		g.topLevelBuf.Write(fnBuf.Bytes())
		return "@" + anonName, retType + " (" + strings.Join(paramTypes, ", ") + ")*", nil

	case *ast.CallExpr:
		if mem, ok := e.Function.(*ast.MemberExpr); ok {
			if objId, ok := mem.Object.(*ast.Identifier); ok {
				if objId.Value == "fs" && mem.Member.Value == "read_file" && len(e.Arguments) == 1 {
					argReg, _, err := g.generateExpression(buf, e.Arguments[0])
					if err != nil {
						return "", "", err
					}
					resReg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = call i8* @fs_read_file(i8* %s)\n", resReg, argReg))
					return resReg, "i8*", nil
				}
				if objId.Value == "fs" && mem.Member.Value == "write_file" && len(e.Arguments) == 2 {
					arg1, _, _ := g.generateExpression(buf, e.Arguments[0])
					arg2, _, _ := g.generateExpression(buf, e.Arguments[1])
					buf.WriteString(fmt.Sprintf("    call void @fs_write_file(i8* %s, i8* %s)\n", arg1, arg2))
					return "", "void", nil
				}
				if objId.Value == "sys" && mem.Member.Value == "now_millis" {
					resReg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = call i64 @sys_now_millis()\n", resReg))
					return resReg, "i64", nil
				}
			}

			if mem.Member.Value == "send" && len(e.Arguments) == 1 {
				chReg, _, err := g.generateExpression(buf, mem.Object)
				if err != nil {
					return "", "", err
				}
				valReg, _, err := g.generateExpression(buf, e.Arguments[0])
				if err != nil {
					return "", "", err
				}
				buf.WriteString(fmt.Sprintf("    call void @array_append(%%struct.Array* %s, i64 %s)\n", chReg, valReg))
				return "", "void", nil
			}

			if mem.Member.Value == "recv" {
				chReg, _, err := g.generateExpression(buf, mem.Object)
				if err != nil {
					return "", "", err
				}
				valReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i64 @array_get(%%struct.Array* %s, i64 0)\n", valReg, chReg))
				return valReg, "i64", nil
			}

			if (mem.Member.Value == "map" || mem.Member.Value == "filter") && len(e.Arguments) == 1 {
				arrReg, _, err := g.generateExpression(buf, mem.Object)
				if err != nil {
					return "", "", err
				}
				fnReg, _, err := g.generateExpression(buf, e.Arguments[0])
				if err != nil {
					return "", "", err
				}

				lenReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i64 @array_len(%%struct.Array* %s)\n", lenReg, arrReg))

				resArrReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call %%struct.Array* @create_array(i64 %s)\n", resArrReg, lenReg))

				idxPtr := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = alloca i64\n", idxPtr))
				buf.WriteString(fmt.Sprintf("    store i64 0, i64* %s\n", idxPtr))

				condLabel := g.freshLabel("mf.cond")
				bodyLabel := g.freshLabel("mf.body")
				endLabel := g.freshLabel("mf.end")

				buf.WriteString(fmt.Sprintf("    br label %%%s\n", condLabel))
				buf.WriteString(fmt.Sprintf("%s:\n", condLabel))
				curIdx := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = load i64, i64* %s\n", curIdx, idxPtr))
				condReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = icmp slt i64 %s, %s\n", condReg, curIdx, lenReg))
				buf.WriteString(fmt.Sprintf("    br i1 %s, label %%%s, label %%%s\n", condReg, bodyLabel, endLabel))

				buf.WriteString(fmt.Sprintf("%s:\n", bodyLabel))
				elemReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i64 @array_get(%%struct.Array* %s, i64 %s)\n", elemReg, arrReg, curIdx))

				callResReg := g.freshReg()
				if mem.Member.Value == "filter" {
					buf.WriteString(fmt.Sprintf("    %s = call i1 %s(i64 %s)\n", callResReg, fnReg, elemReg))
				} else {
					buf.WriteString(fmt.Sprintf("    %s = call i64 %s(i64 %s)\n", callResReg, fnReg, elemReg))
				}

				if mem.Member.Value == "map" {
					buf.WriteString(fmt.Sprintf("    call void @array_append(%%struct.Array* %s, i64 %s)\n", resArrReg, callResReg))
				} else {
					pushLabel := g.freshLabel("mf.push")
					skipLabel := g.freshLabel("mf.skip")
					buf.WriteString(fmt.Sprintf("    br i1 %s, label %%%s, label %%%s\n", callResReg, pushLabel, skipLabel))
					buf.WriteString(fmt.Sprintf("%s:\n", pushLabel))
					buf.WriteString(fmt.Sprintf("    call void @array_append(%%struct.Array* %s, i64 %s)\n", resArrReg, elemReg))
					buf.WriteString(fmt.Sprintf("    br label %%%s\n", skipLabel))
					buf.WriteString(fmt.Sprintf("%s:\n", skipLabel))
				}

				nextIdx := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = add i64 %s, 1\n", nextIdx, curIdx))
				buf.WriteString(fmt.Sprintf("    store i64 %s, i64* %s\n", nextIdx, idxPtr))
				buf.WriteString(fmt.Sprintf("    br label %%%s\n", condLabel))

				buf.WriteString(fmt.Sprintf("%s:\n", endLabel))

				return resArrReg, "%struct.Array*", nil
			}

			if mem.Member.Value == "trim" {
				objReg, _, err := g.generateExpression(buf, mem.Object)
				if err != nil { return "", "", err }
				return objReg, "i8*", nil
			}
			if mem.Member.Value == "len" {
				objReg, objType, err := g.generateExpression(buf, mem.Object)
				if err != nil { return "", "", err }
				if objType == "i8*" {
					lenReg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = call i64 @strlen(i8* %s)\n", lenReg, objReg))
					return lenReg, "i64", nil
				}
				if objType == "%struct.Array*" {
					lenReg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = call i64 @array_len(%%struct.Array* %s)\n", lenReg, objReg))
					return lenReg, "i64", nil
				}
			}
			if mem.Member.Value == "split" && len(e.Arguments) == 1 {
				objReg, _, err := g.generateExpression(buf, mem.Object)
				if err != nil { return "", "", err }
				resArrReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call %%struct.Array* @create_array(i64 16)\n", resArrReg))
				valInt := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = ptrtoint i8* %s to i64\n", valInt, objReg))
				buf.WriteString(fmt.Sprintf("    call void @array_append(%%struct.Array* %s, i64 %s)\n", resArrReg, valInt))
				return resArrReg, "%struct.Array*", nil
			}

			if mem.Member.Value == "sort" {
				arrReg, _, err := g.generateExpression(buf, mem.Object)
				if err != nil { return "", "", err }
				buf.WriteString(fmt.Sprintf("    call void @array_sort(%%struct.Array* %s)\n", arrReg))
				return "", "void", nil
			}
			if mem.Member.Value == "reverse" {
				arrReg, _, err := g.generateExpression(buf, mem.Object)
				if err != nil { return "", "", err }
				buf.WriteString(fmt.Sprintf("    call void @array_reverse(%%struct.Array* %s)\n", arrReg))
				return "", "void", nil
			}
			if mem.Member.Value == "contains" && len(e.Arguments) == 1 {
				arrReg, _, err := g.generateExpression(buf, mem.Object)
				if err != nil { return "", "", err }
				valReg, valType, err := g.generateExpression(buf, e.Arguments[0])
				if err != nil { return "", "", err }
				if valType != "i64" {
					if valType == "ptr" {
						bcReg := g.freshReg()
						buf.WriteString(fmt.Sprintf("    %s = bitcast ptr %s to i8*\n", bcReg, valReg))
						valReg = bcReg
						valType = "i8*"
					}
					intReg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = ptrtoint %s %s to i64\n", intReg, valType, valReg))
					valReg = intReg
				}
				resReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i64 @array_contains(%%struct.Array* %s, i64 %s)\n", resReg, arrReg, valReg))
				return resReg, "i64", nil
			}
			if mem.Member.Value == "find" && len(e.Arguments) == 1 {
				arrReg, _, err := g.generateExpression(buf, mem.Object)
				if err != nil { return "", "", err }
				valReg, valType, err := g.generateExpression(buf, e.Arguments[0])
				if err != nil { return "", "", err }
				if valType != "i64" {
					if valType == "ptr" {
						bcReg := g.freshReg()
						buf.WriteString(fmt.Sprintf("    %s = bitcast ptr %s to i8*\n", bcReg, valReg))
						valReg = bcReg
						valType = "i8*"
					}
					intReg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = ptrtoint %s %s to i64\n", intReg, valType, valReg))
					valReg = intReg
				}
				resReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i64 @array_find(%%struct.Array* %s, i64 %s)\n", resReg, arrReg, valReg))
				return resReg, "i64", nil
			}
			if mem.Member.Value == "join" && len(e.Arguments) == 1 {
				arrReg, _, err := g.generateExpression(buf, mem.Object)
				if err != nil { return "", "", err }
				sepReg, _, err := g.generateExpression(buf, e.Arguments[0])
				if err != nil { return "", "", err }
				resReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i8* @array_join(%%struct.Array* %s, i8* %s)\n", resReg, arrReg, sepReg))
				return resReg, "i8*", nil
			}

			// Method call like arr.append(val)
			if mem.Member.Value == "append" {
				arrReg, _, err := g.generateExpression(buf, mem.Object)
				if err != nil {
					return "", "", err
				}
				valReg, valType, err := g.generateExpression(buf, e.Arguments[0])
				if err != nil {
					return "", "", err
				}
				if valType != "i64" {
					intReg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = ptrtoint %s %s to i64\n", intReg, valType, valReg))
					valReg = intReg
				}
				buf.WriteString(fmt.Sprintf("    call void @array_append(%%struct.Array* %s, i64 %s)\n", arrReg, valReg))
				return "", "void", nil
			}

			// Struct method call: rect.area() -> Rectangle_area(%struct.Rectangle* %rect_ptr)
			objReg, objType, err := g.generateExpression(buf, mem.Object)
			if err != nil {
				return "", "", err
			}
			structTypeName := g.methodReceivers[mem.Member.Value]
			if structTypeName == "" {
				if objId, ok := mem.Object.(*ast.Identifier); ok {
					varType := g.symbolTable[objId.Value]
					if strings.HasPrefix(varType, "%struct.") {
						structTypeName = strings.TrimSuffix(strings.TrimPrefix(varType, "%struct."), "*")
					}
				}
			}
			if structTypeName == "" && strings.HasPrefix(objType, "%struct.") {
				structTypeName = strings.TrimSuffix(strings.TrimPrefix(objType, "%struct."), "*")
			}
			if structTypeName != "" && structTypeName != "i8" && structTypeName != "Array" && g.fnReturnTypes[structTypeName+"_"+mem.Member.Value] != "" {
				fnName := structTypeName + "_" + mem.Member.Value
				ptrType := fmt.Sprintf("%%struct.%s*", structTypeName)
				var args []string
				if !strings.HasSuffix(objType, "*") {
					tmpAlloc := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = alloca %%struct.%s\n", tmpAlloc, structTypeName))
					buf.WriteString(fmt.Sprintf("    store %%struct.%s %s, %%struct.%s* %s\n", structTypeName, objReg, structTypeName, tmpAlloc))
					objReg = tmpAlloc
				}
				args = append(args, fmt.Sprintf("%s %s", ptrType, objReg))
				for _, arg := range e.Arguments {
					aReg, aType, err := g.generateExpression(buf, arg)
					if err != nil {
						return "", "", err
					}
					if strings.HasPrefix(aType, "%struct.") && strings.HasSuffix(aType, "*") {
						loadedVal := g.freshReg()
						pureType := strings.TrimSuffix(aType, "*")
						buf.WriteString(fmt.Sprintf("    %s = load %s, %s* %s\n", loadedVal, pureType, pureType, aReg))
						aReg = loadedVal
						aType = pureType
					}
					args = append(args, fmt.Sprintf("%s %s", aType, aReg))
				}

				retType := g.fnReturnTypes[fnName]
				if retType == "void" {
					buf.WriteString(fmt.Sprintf("    call void @%s(%s)\n", fnName, strings.Join(args, ", ")))
					return "", "void", nil
				}
				if retType == "" {
					retType = "i64"
				}
				resReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call %s @%s(%s)\n", resReg, retType, fnName, strings.Join(args, ", ")))
				return resReg, retType, nil
			}
			return g.generateExpression(buf, mem)
		}

		// Struct constructor instantiation Point(10, 20)
		fnIdent, ok := e.Function.(*ast.Identifier)
		if !ok {
			return "", "", fmt.Errorf("indirect function calls not supported yet")
		}

		fnName := fnIdent.Value

		if fnName == "Channel" {
			resReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call %%struct.Array* @create_array(i64 16)\n", resReg))
			return resReg, "%struct.Array*", nil
		}

		if fnName == "field_names" && len(e.Arguments) == 1 {
			targetStruct := ""
			if strLit, ok := e.Arguments[0].(*ast.StringLiteral); ok {
				targetStruct = strLit.Value
			} else if id, ok := e.Arguments[0].(*ast.Identifier); ok {
				targetStruct = id.Value
			}
			fields := g.structTypes[targetStruct]
			resArrReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call %%struct.Array* @create_array(i64 %d)\n", resArrReg, len(fields)))
			for _, f := range fields {
				sReg, _, _ := g.generateExpression(buf, &ast.StringLiteral{Value: f.Name.Value})
				intReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = ptrtoint i8* %s to i64\n", intReg, sReg))
				buf.WriteString(fmt.Sprintf("    call void @array_append(%%struct.Array* %s, i64 %s)\n", resArrReg, intReg))
			}
			return resArrReg, "%struct.Array*", nil
		}

		if fnName == "field_count" && len(e.Arguments) == 1 {
			targetStruct := ""
			if strLit, ok := e.Arguments[0].(*ast.StringLiteral); ok {
				targetStruct = strLit.Value
			} else if id, ok := e.Arguments[0].(*ast.Identifier); ok {
				targetStruct = id.Value
			}
			count := len(g.structTypes[targetStruct])
			return fmt.Sprintf("%d", count), "i64", nil
		}

		if fnName == "type_name" && len(e.Arguments) == 1 {
			targetStr := "object"
			if id, ok := e.Arguments[0].(*ast.Identifier); ok {
				targetStr = id.Value
			}
			strReg := g.getGlobalString(targetStr)
			lenBytes := len(targetStr) + 1
			ptrReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds [%d x i8], [%d x i8]* %s, i64 0, i64 0\n",
				ptrReg, lenBytes, lenBytes, strReg))
			return ptrReg, "i8*", nil
		}

		if fnName == "Box" && len(e.Arguments) == 1 {
			argReg, _, err := g.generateExpression(buf, e.Arguments[0])
			if err != nil {
				return "", "", err
			}
			ptrReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i8* @malloc(i64 8)\n", ptrReg))
			boxPtr := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = bitcast i8* %s to i64*\n", boxPtr, ptrReg))
			buf.WriteString(fmt.Sprintf("    store i64 %s, i64* %s\n", argReg, boxPtr))
			return boxPtr, "i64*", nil
		}

		if fnName == "Rc" && len(e.Arguments) == 1 {
			argReg, _, err := g.generateExpression(buf, e.Arguments[0])
			if err != nil {
				return "", "", err
			}
			ptrReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i8* @malloc(i64 16)\n", ptrReg))
			rcPtr := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = bitcast i8* %s to i64*\n", rcPtr, ptrReg))
			buf.WriteString(fmt.Sprintf("    store i64 1, i64* %s\n", rcPtr))
			valPtr := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds i64, i64* %s, i64 1\n", valPtr, rcPtr))
			buf.WriteString(fmt.Sprintf("    store i64 %s, i64* %s\n", argReg, valPtr))
			return valPtr, "i64*", nil
		}

		if ptrReg, isLocalVar := g.varAllocaMap[fnName]; isLocalVar {
			fnType := g.symbolTable[fnName]
			fnPtr := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = load %s, %s* %s\n", fnPtr, fnType, fnType, ptrReg))

			var args []string
			for _, arg := range e.Arguments {
				aReg, aType, err := g.generateExpression(buf, arg)
				if err != nil {
					return "", "", err
				}
				args = append(args, fmt.Sprintf("%s %s", aType, aReg))
			}

			resReg := g.freshReg()
			retType := "i64"
			buf.WriteString(fmt.Sprintf("    %s = call %s %s(%s)\n", resReg, retType, fnPtr, strings.Join(args, ", ")))
			return resReg, retType, nil
		}

		// Monomorphize generic function calls if needed
		if gFn, isGeneric := g.genericFns[fnName]; isGeneric {
			var argRegs []string
			var argTypes []string
			var argTypeNames []string
			for _, arg := range e.Arguments {
				r, t, err := g.generateExpression(buf, arg)
				if err != nil {
					return "", "", err
				}
				argRegs = append(argRegs, r)
				argTypes = append(argTypes, t)
				argTypeNames = append(argTypeNames, strings.TrimPrefix(t, "%struct."))
			}

			monoName := fmt.Sprintf("%s_%s", fnName, strings.Join(argTypeNames, "_"))

			if !g.monoFns[monoName] {
				g.monoFns[monoName] = true
				subGen := New()
				subGen.regCounter = 0
				subGen.symbolTable = make(map[string]string)
				subGen.varAllocaMap = make(map[string]string)
				subGen.globalStrings = g.globalStrings
				subGen.structTypes = g.structTypes

				var paramStrs []string
				for idx, p := range gFn.Params {
					pType := argTypes[idx]
					paramStrs = append(paramStrs, fmt.Sprintf("%s %%%s.arg", pType, p.Name.Value))
					ptrReg := fmt.Sprintf("%%%s.addr", p.Name.Value)
					subGen.symbolTable[p.Name.Value] = pType
					subGen.varAllocaMap[p.Name.Value] = ptrReg
				}

				retType := argTypes[0]
				if gFn.ReturnType != "" && gFn.ReturnType != "T" {
					retType = mapLLVMType(gFn.ReturnType)
				}

				g.fnReturnTypes[monoName] = retType

				var monoBuf bytes.Buffer
				monoBuf.WriteString(fmt.Sprintf("define %s @%s(%s) {\n", retType, monoName, strings.Join(paramStrs, ", ")))
				monoBuf.WriteString("entry:\n")
				for idx, p := range gFn.Params {
					pType := argTypes[idx]
					ptrReg := fmt.Sprintf("%%%s.addr", p.Name.Value)
					monoBuf.WriteString(fmt.Sprintf("    %s = alloca %s\n", ptrReg, pType))
					monoBuf.WriteString(fmt.Sprintf("    store %s %%%s.arg, %s* %s\n", pType, p.Name.Value, pType, ptrReg))
				}
				for _, bStmt := range gFn.Body.Statements {
					if err := subGen.generateBodyStatement(&monoBuf, bStmt); err != nil {
						return "", "", err
					}
				}
				if retType == "void" {
					monoBuf.WriteString("    ret void\n")
				} else if !strings.Contains(monoBuf.String(), "ret ") {
					monoBuf.WriteString(fmt.Sprintf("    ret %s 0\n", retType))
				}
				monoBuf.WriteString("}\n\n")

				g.topLevelBuf.Write(monoBuf.Bytes())
			}

			var callArgs []string
			for idx, r := range argRegs {
				callArgs = append(callArgs, fmt.Sprintf("%s %s", argTypes[idx], r))
			}
			resReg := g.freshReg()
			retType := g.fnReturnTypes[monoName]
			buf.WriteString(fmt.Sprintf("    %s = call %s @%s(%s)\n", resReg, retType, monoName, strings.Join(callArgs, ", ")))
			return resReg, retType, nil
		}

		if fnName == "len" && len(e.Arguments) == 1 {
			argReg, argType, err := g.generateExpression(buf, e.Arguments[0])
			if err != nil {
				return "", "", err
			}
			if argType == "i8*" {
				lenReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i64 @strlen(i8* %s)\n", lenReg, argReg))
				return lenReg, "i64", nil
			}
			if argType == "%struct.Array*" {
				lenPtrReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %%struct.Array, %%struct.Array* %s, i32 0, i32 1\n", lenPtrReg, argReg))
				lenReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = load i64, i64* %s\n", lenReg, lenPtrReg))
				return lenReg, "i64", nil
			}
		}

		if fnName == "Some" && len(e.Arguments) == 1 {
			argReg, _, err := g.generateExpression(buf, e.Arguments[0])
			if err != nil {
				return "", "", err
			}
			resReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call %%struct.Option @Some(i64 %s)\n", resReg, argReg))
			return resReg, "%struct.Option", nil
		}

		if fnName == "Ok" && len(e.Arguments) == 1 {
			argReg, _, err := g.generateExpression(buf, e.Arguments[0])
			if err != nil {
				return "", "", err
			}
			resReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call %%struct.Result @Ok(i64 %s)\n", resReg, argReg))
			return resReg, "%struct.Result", nil
		}

		if fnName == "Err" && len(e.Arguments) == 1 {
			argReg, _, err := g.generateExpression(buf, e.Arguments[0])
			if err != nil {
				return "", "", err
			}
			resReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call %%struct.Result @Err(i8* %s)\n", resReg, argReg))
			return resReg, "%struct.Result", nil
		}

		if _, isStruct := g.structTypes[fnName]; isStruct {
			structType := fmt.Sprintf("%%struct.%s", fnName)
			rawPtrReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = alloca %s\n", rawPtrReg, structType))
			for idx, arg := range e.Arguments {
				aReg, aType, err := g.generateExpression(buf, arg)
				if err != nil {
					return "", "", err
				}
				fieldPtrReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds %s, %s* %s, i32 0, i32 %d\n",
					fieldPtrReg, structType, structType, rawPtrReg, idx))
				buf.WriteString(fmt.Sprintf("    store %s %s, %s* %s\n", aType, aReg, aType, fieldPtrReg))
			}
			valReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = load %s, %s* %s\n", valReg, structType, structType, rawPtrReg))
			return valReg, structType, nil
		}

		if fnName == "len" && len(e.Arguments) == 1 {
			argReg, argType, err := g.generateExpression(buf, e.Arguments[0])
			if err != nil {
				return "", "", err
			}
			resReg := g.freshReg()
			if argType == "i8*" {
				buf.WriteString(fmt.Sprintf("    %s = call i64 @strlen(i8* %s)\n", resReg, argReg))
			} else {
				if argType == "i64" {
					ptrReg := g.freshReg()
					buf.WriteString(fmt.Sprintf("    %s = inttoptr i64 %s to %%struct.Array*\n", ptrReg, argReg))
					argReg = ptrReg
				}
				buf.WriteString(fmt.Sprintf("    %s = call i64 @array_len(%%struct.Array* %s)\n", resReg, argReg))
			}
			return resReg, "i64", nil
		}

		if fnName == "str" && len(e.Arguments) == 1 {
			argReg, argType, err := g.generateExpression(buf, e.Arguments[0])
			if err != nil {
				return "", "", err
			}
			if argType == "i8*" {
				return argReg, "i8*", nil
			}
			if argType == "%struct.Array*" {
				resReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i8* @str_array(%%struct.Array* %s)\n", resReg, argReg))
				return resReg, "i8*", nil
			}
			resReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i8* @str_int(i64 %s)\n", resReg, argReg))
			return resReg, "i8*", nil
		}

		if fnName == "println" || fnName == "print" {
			if len(e.Arguments) == 0 {
				resReg := g.freshReg()
				fmtStrReg := g.freshReg()
				flReg := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds [4 x i8], [4 x i8]* @.fmt_str, i64 0, i64 0\n", fmtStrReg))
				buf.WriteString(fmt.Sprintf("    %s = call i32 (i8*, ...) @printf(i8* %s)\n", resReg, fmtStrReg))
				buf.WriteString(fmt.Sprintf("    %s = call i32 @fflush(i8* null)\n", flReg))
				return resReg, "i32", nil
			}

			argReg, argType, err := g.generateExpression(buf, e.Arguments[0])
			if err != nil {
				return "", "", err
			}

			if argType == "%struct.Array*" {
				buf.WriteString(fmt.Sprintf("    call void @println_array(%%struct.Array* %s)\n", argReg))
				return "", "void", nil
			}

			fmtStrPtr := "@.fmt_int"
			fmtStrLen := 6
			if fnName == "print" {
				fmtStrPtr = "@.fmt_int_raw"
				fmtStrLen = 5
				if argType == "i8*" {
					fmtStrPtr = "@.fmt_str_raw"
					fmtStrLen = 3
				}
			} else {
				if argType == "double" {
					fmtStrPtr = "@.fmt_float"
					fmtStrLen = 4
				} else if argType == "i8*" {
					fmtStrPtr = "@.fmt_str"
					fmtStrLen = 4
				}
			}

			fmtStrReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = getelementptr inbounds [%d x i8], [%d x i8]* %s, i64 0, i64 0\n",
				fmtStrReg, fmtStrLen, fmtStrLen, fmtStrPtr))

			resReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i32 (i8*, ...) @printf(i8* %s, %s %s)\n",
				resReg, fmtStrReg, argType, argReg))

			flReg := g.freshReg()
			buf.WriteString(fmt.Sprintf("    %s = call i32 @fflush(i8* null)\n", flReg))

			return resReg, "i32", nil
		}

		var args []string
		pTypes := g.fnParamTypes[fnName]
		fnStmt := g.fnDeclStmts[fnName]
		numParams := len(pTypes)
		if fnStmt != nil && len(fnStmt.Params) > numParams {
			numParams = len(fnStmt.Params)
		}
		for idx := 0; idx < numParams; idx++ {
			var aReg, aType string
			var err error
			if idx < len(e.Arguments) {
				aReg, aType, err = g.generateExpression(buf, e.Arguments[idx])
			} else if fnStmt != nil && idx < len(fnStmt.Params) && fnStmt.Params[idx].DefaultValue != nil {
				aReg, aType, err = g.generateExpression(buf, fnStmt.Params[idx].DefaultValue)
			} else {
				aReg = "0"
				if idx < len(pTypes) {
					aType = pTypes[idx]
				} else {
					aType = "i64"
				}
			}
			if err != nil {
				return "", "", err
			}
			if strings.HasPrefix(aType, "%struct.") && strings.HasSuffix(aType, "*") && idx < len(pTypes) && !strings.HasSuffix(pTypes[idx], "*") {
				loadedVal := g.freshReg()
				pureType := strings.TrimSuffix(aType, "*")
				buf.WriteString(fmt.Sprintf("    %s = load %s, %s* %s\n", loadedVal, pureType, pureType, aReg))
				aReg = loadedVal
				aType = pureType
			}
			if idx < len(pTypes) && pTypes[idx] == "i8*" && aType != "i8*" {
				cStr := g.freshReg()
				buf.WriteString(fmt.Sprintf("    %s = call i8* @char_to_str(i64 %s)\n", cStr, aReg))
				aReg = cStr
				aType = "i8*"
			}
			args = append(args, fmt.Sprintf("%s %s", aType, aReg))
		}

		resReg := g.freshReg()
		retType := g.fnReturnTypes[fnName]
		if retType == "" {
			retType = "i64"
		}
		if retType == "void" {
			buf.WriteString(fmt.Sprintf("    call void @%s(%s)\n", fnName, strings.Join(args, ", ")))
			return "", "void", nil
		}
		buf.WriteString(fmt.Sprintf("    %s = call %s @%s(%s)\n", resReg, retType, fnName, strings.Join(args, ", ")))
		return resReg, retType, nil

	case *ast.RangeExpr:
		resArrReg := g.freshReg()
		buf.WriteString(fmt.Sprintf("    %s = call %%struct.Array* @create_array(i64 16)\n", resArrReg))
		return resArrReg, "%struct.Array*", nil


	default:
		return "", "", fmt.Errorf("unsupported expression in LLVM codegen: %T", expr)
	}
}

func mapLLVMType(t string) string {
	if strings.HasSuffix(t, "[]") {
		return "%struct.Array*"
	}
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		inner := t[1 : len(t)-1]
		parts := strings.Split(inner, ",")
		var mapped []string
		for _, p := range parts {
			mapped = append(mapped, mapLLVMType(strings.TrimSpace(p)))
		}
		tName := fmt.Sprintf("struct.Tuple_%d_%s", len(mapped), strings.Join(mapped, "_"))
		tName = strings.ReplaceAll(tName, "*", "p")
		return "%" + tName
	}
	switch t {
	case "int":
		return "i64"
	case "float":
		return "double"
	case "string":
		return "i8*"
	case "bool":
		return "i1"
	case "Channel":
		return "%struct.Array*"
	case "Option":
		return "%struct.Option"
	case "Result":
		return "%struct.Result"
	case "":
		return "void"
	default:
		return fmt.Sprintf("%%struct.%s", t)
	}
}

func escapeLLVMString(s string) string {
	var out bytes.Buffer
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 32 && b <= 126 && b != '"' && b != '\\' {
			out.WriteByte(b)
		} else {
			out.WriteString(fmt.Sprintf("\\%02X", b))
		}
	}
	return out.String()
}

func (g *LLVMGenerator) emitDeferred(buf *bytes.Buffer) {
	for i := len(g.deferredStmts) - 1; i >= 0; i-- {
		g.generateExpression(buf, g.deferredStmts[i])
	}
	g.deferredStmts = nil
}
