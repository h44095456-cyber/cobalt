package codegen

import (
	"bytes"
	"cobalt/pkg/ast"
	"fmt"
	"strings"
)

type CodeGenerator struct {
	buf             bytes.Buffer
	indentLevel     int
	structTypes     map[string]bool
	methodReceivers map[string]string // methodName -> structType ("area" -> "Rectangle")
	externFns       map[string]*ast.ExternFnStmt
	structFields    map[string][]ast.StructField
	fnParams        map[string][]ast.Param
	currentFn       string
	deferredStmts   []ast.Expression
}

func New() *CodeGenerator {
	return &CodeGenerator{
		structTypes:     make(map[string]bool),
		methodReceivers: make(map[string]string),
		externFns:       make(map[string]*ast.ExternFnStmt),
		structFields:    make(map[string][]ast.StructField),
		fnParams:        make(map[string][]ast.Param),
	}
}

func (cg *CodeGenerator) indent() string {
	return strings.Repeat("    ", cg.indentLevel)
}

func (cg *CodeGenerator) Generate(program *ast.Program) (string, error) {
	cg.buf.Reset()
	cg.indentLevel = 0
	cg.structTypes = make(map[string]bool)
	cg.methodReceivers = make(map[string]string)
	cg.externFns = make(map[string]*ast.ExternFnStmt)
	cg.structFields = make(map[string][]ast.StructField)

	// Standard includes
	cg.buf.WriteString("#include <iostream>\n")
	cg.buf.WriteString("#include <string>\n")
	cg.buf.WriteString("#include <vector>\n")
	cg.buf.WriteString("#include <algorithm>\n")
	cg.buf.WriteString("#include <tuple>\n")
	cg.buf.WriteString("#include <unordered_map>\n")
	cg.buf.WriteString("#include <future>\n")
	cg.buf.WriteString("#include <thread>\n")
	cg.buf.WriteString("#include <cstdint>\n")
	cg.buf.WriteString("#include <cmath>\n")
	cg.buf.WriteString("#include <fstream>\n")
	cg.buf.WriteString("#include <sstream>\n")
	cg.buf.WriteString("#include <chrono>\n")
	cg.buf.WriteString("#include <thread>\n")
	cg.buf.WriteString("#include <mutex>\n")
	cg.buf.WriteString("#include <condition_variable>\n")
	cg.buf.WriteString("#include <future>\n")
	cg.buf.WriteString("#include <queue>\n\n")

	cg.buf.WriteString("inline std::vector<std::string> g_cobalt_args;\n\n")

	cg.buf.WriteString("template <typename T = long long>\n")
	cg.buf.WriteString("struct CobaltChannel {\n")
	cg.buf.WriteString("    std::queue<T> q;\n")
	cg.buf.WriteString("    std::mutex mtx;\n")
	cg.buf.WriteString("    std::condition_variable cv;\n\n")
	cg.buf.WriteString("    void send(T val) {\n")
	cg.buf.WriteString("        std::unique_lock<std::mutex> lock(mtx);\n")
	cg.buf.WriteString("        q.push(val);\n")
	cg.buf.WriteString("        cv.notify_one();\n")
	cg.buf.WriteString("    }\n\n")
	cg.buf.WriteString("    T recv() {\n")
	cg.buf.WriteString("        std::unique_lock<std::mutex> lock(mtx);\n")
	cg.buf.WriteString("        cv.wait(lock, [this]() { return !q.empty(); });\n")
	cg.buf.WriteString("        T val = q.front();\n")
	cg.buf.WriteString("        q.pop();\n")
	cg.buf.WriteString("        return val;\n")
	cg.buf.WriteString("    }\n")
	cg.buf.WriteString("};\n\n")
	cg.buf.WriteString("using Channel = std::shared_ptr<CobaltChannel<long long>>;\n\n")
	cg.buf.WriteString("inline Channel create_channel() {\n")
	cg.buf.WriteString("    return std::make_shared<CobaltChannel<long long>>();\n")
	cg.buf.WriteString("}\n\n")

	cg.buf.WriteString("inline std::string fs_read_file(const std::string& path) {\n")
	cg.buf.WriteString("    std::ifstream file(path);\n")
	cg.buf.WriteString("    if (!file.is_open()) return \"\";\n")
	cg.buf.WriteString("    std::stringstream buffer;\n")
	cg.buf.WriteString("    buffer << file.rdbuf();\n")
	cg.buf.WriteString("    return buffer.str();\n")
	cg.buf.WriteString("}\n\n")

	cg.buf.WriteString("inline void fs_write_file(const std::string& path, const std::string& data) {\n")
	cg.buf.WriteString("    std::ofstream file(path);\n")
	cg.buf.WriteString("    if (file.is_open()) {\n")
	cg.buf.WriteString("        file << data;\n")
	cg.buf.WriteString("    }\n")
	cg.buf.WriteString("}\n\n")

	cg.buf.WriteString("inline long long sys_now_millis() {\n")
	cg.buf.WriteString("    return std::chrono::duration_cast<std::chrono::milliseconds>(\n")
	cg.buf.WriteString("        std::chrono::system_clock::now().time_since_epoch()\n")
	cg.buf.WriteString("    ).count();\n")
	cg.buf.WriteString("}\n\n")

	cg.buf.WriteString("inline std::vector<long long> env_args() {\n")
	cg.buf.WriteString("    std::vector<long long> res;\n")
	cg.buf.WriteString("    return res;\n")
	cg.buf.WriteString("}\n\n")

	// Custom helper runtime functions
	cg.buf.WriteString("// Helper runtime functions for Cobalt\n")
	cg.buf.WriteString("template<typename T>\n")
	cg.buf.WriteString("void println(T val) {\n")
	cg.buf.WriteString("    std::cout << val << std::endl;\n")
	cg.buf.WriteString("}\n\n")
	cg.buf.WriteString("inline void println() {\n")
	cg.buf.WriteString("    std::cout << std::endl;\n")
	cg.buf.WriteString("}\n\n")
	cg.buf.WriteString("template<typename T>\n")
	cg.buf.WriteString("std::string str(T val) {\n")
	cg.buf.WriteString("    return std::to_string(val);\n")
	cg.buf.WriteString("}\n\n")
	cg.buf.WriteString("inline std::string str(const std::string& val) {\n")
	cg.buf.WriteString("    return val;\n")
	cg.buf.WriteString("}\n\n")
	cg.buf.WriteString("inline std::string str(const char* val) {\n")
	cg.buf.WriteString("    return std::string(val);\n")
	cg.buf.WriteString("}\n\n")
	cg.buf.WriteString("inline std::string str(bool val) {\n")
	cg.buf.WriteString("    return val ? \"true\" : \"false\";\n")
	cg.buf.WriteString("}\n\n")

	cg.buf.WriteString("template <typename T>\n")
	cg.buf.WriteString("inline std::string str(const std::vector<T>& v) {\n")
	cg.buf.WriteString("    std::string res = \"[\";\n")
	cg.buf.WriteString("    for (size_t i = 0; i < v.size(); ++i) {\n")
	cg.buf.WriteString("        res += str(v[i]);\n")
	cg.buf.WriteString("        if (i + 1 < v.size()) res += \", \";\n")
	cg.buf.WriteString("    }\n")
	cg.buf.WriteString("    res += \"]\";\n")
	cg.buf.WriteString("    return res;\n")
	cg.buf.WriteString("}\n\n")

	cg.buf.WriteString("inline std::string chr(long long code) {\n")
	cg.buf.WriteString("    return std::string(1, (char)code);\n")
	cg.buf.WriteString("}\n\n")
	cg.buf.WriteString("template <typename T>\n")
	cg.buf.WriteString("inline void print(const T& val) {\n")
	cg.buf.WriteString("    std::cout << val << std::flush;\n")
	cg.buf.WriteString("}\n\n")
	cg.buf.WriteString("inline bool operator==(char c, const std::string& s) { return s.length() == 1 && s[0] == c; }\n")
	cg.buf.WriteString("inline bool operator==(const std::string& s, char c) { return s.length() == 1 && s[0] == c; }\n")
	cg.buf.WriteString("inline bool operator!=(char c, const std::string& s) { return !(c == s); }\n")
	cg.buf.WriteString("inline bool operator!=(const std::string& s, char c) { return !(s == c); }\n\n")

	cg.buf.WriteString("inline std::string cobalt_trim(const std::string& s) {\n")
	cg.buf.WriteString("    size_t first = s.find_first_not_of(\" \\t\\n\\r\");\n")
	cg.buf.WriteString("    if (first == std::string::npos) return \"\";\n")
	cg.buf.WriteString("    size_t last = s.find_last_not_of(\" \\t\\n\\r\");\n")
	cg.buf.WriteString("    return s.substr(first, (last - first + 1));\n")
	cg.buf.WriteString("}\n\n")

	cg.buf.WriteString("inline std::vector<std::string> cobalt_split(const std::string& s, const std::string& delim) {\n")
	cg.buf.WriteString("    std::vector<std::string> res;\n")
	cg.buf.WriteString("    size_t start = 0;\n")
	cg.buf.WriteString("    size_t end = s.find(delim);\n")
	cg.buf.WriteString("    while (end != std::string::npos) {\n")
	cg.buf.WriteString("        res.push_back(s.substr(start, end - start));\n")
	cg.buf.WriteString("        start = end + delim.length();\n")
	cg.buf.WriteString("        end = s.find(delim, start);\n")
	cg.buf.WriteString("    }\n")
	cg.buf.WriteString("    res.push_back(s.substr(start));\n")
	cg.buf.WriteString("    return res;\n")
	cg.buf.WriteString("}\n\n")

	cg.buf.WriteString("template <typename T>\n")
	cg.buf.WriteString("auto cobalt_slice(const T& src, long long start, long long end) {\n")
	cg.buf.WriteString("    if constexpr (std::is_same_v<T, std::string>) {\n")
	cg.buf.WriteString("        return src.substr(start, end - start);\n")
	cg.buf.WriteString("    } else {\n")
	cg.buf.WriteString("        return T(src.begin() + start, src.begin() + end);\n")
	cg.buf.WriteString("    }\n")
	cg.buf.WriteString("}\n\n")

	cg.buf.WriteString("template <typename T, typename I>\n")
	cg.buf.WriteString("decltype(auto) cobalt_index(T&& container, I index) {\n")
	cg.buf.WriteString("    if constexpr (std::is_same_v<std::decay_t<T>, std::string>) {\n")
	cg.buf.WriteString("        return std::string(1, container[index]);\n")
	cg.buf.WriteString("    } else {\n")
	cg.buf.WriteString("        return container[index];\n")
	cg.buf.WriteString("    }\n")
	cg.buf.WriteString("}\n\n")

	cg.buf.WriteString("template <typename T>\n")
	cg.buf.WriteString("inline void cobalt_sort(std::vector<T>& v) {\n")
	cg.buf.WriteString("    std::sort(v.begin(), v.end());\n")
	cg.buf.WriteString("}\n\n")
	cg.buf.WriteString("template <typename T>\n")
	cg.buf.WriteString("inline void cobalt_reverse(std::vector<T>& v) {\n")
	cg.buf.WriteString("    std::reverse(v.begin(), v.end());\n")
	cg.buf.WriteString("}\n\n")
	cg.buf.WriteString("template <typename T, typename V>\n")
	cg.buf.WriteString("inline bool cobalt_contains(const T& container, const V& val) {\n")
	cg.buf.WriteString("    if constexpr (std::is_same_v<std::decay_t<T>, std::string>) {\n")
	cg.buf.WriteString("        return container.find(str(val)) != std::string::npos;\n")
	cg.buf.WriteString("    } else {\n")
	cg.buf.WriteString("        return std::find(container.begin(), container.end(), val) != container.end();\n")
	cg.buf.WriteString("    }\n")
	cg.buf.WriteString("}\n\n")
	cg.buf.WriteString("inline std::string cobalt_join(const std::vector<std::string>& v, const std::string& sep) {\n")
	cg.buf.WriteString("    std::string res;\n")
	cg.buf.WriteString("    for (size_t i = 0; i < v.size(); ++i) {\n")
	cg.buf.WriteString("        res += v[i];\n")
	cg.buf.WriteString("        if (i + 1 < v.size()) res += sep;\n")
	cg.buf.WriteString("    }\n")
	cg.buf.WriteString("    return res;\n")
	cg.buf.WriteString("}\n\n")
	cg.buf.WriteString("template <typename T, typename V>\n")
	cg.buf.WriteString("inline long long cobalt_find(const T& container, const V& val) {\n")
	cg.buf.WriteString("    if constexpr (std::is_same_v<std::decay_t<T>, std::string>) {\n")
	cg.buf.WriteString("        auto pos = container.find(str(val));\n")
	cg.buf.WriteString("        return pos == std::string::npos ? -1 : (long long)pos;\n")
	cg.buf.WriteString("    } else {\n")
	cg.buf.WriteString("        auto it = std::find(container.begin(), container.end(), val);\n")
	cg.buf.WriteString("        return it == container.end() ? -1 : (long long)std::distance(container.begin(), it);\n")
	cg.buf.WriteString("    }\n")
	cg.buf.WriteString("}\n\n")

	cg.buf.WriteString("struct CobaltEmptyVec {\n")
	cg.buf.WriteString("    template <typename T>\n")
	cg.buf.WriteString("    operator std::vector<T>() const { return std::vector<T>{}; }\n")
	cg.buf.WriteString("};\n\n")

	// Option Runtime Type
	cg.buf.WriteString("struct Option {\n")
	cg.buf.WriteString("    bool is_some_flag;\n")
	cg.buf.WriteString("    long long val;\n")
	cg.buf.WriteString("    bool is_some() const { return is_some_flag; }\n")
	cg.buf.WriteString("    bool is_none() const { return !is_some_flag; }\n")
	cg.buf.WriteString("    long long unwrap() const { return val; }\n")
	cg.buf.WriteString("};\n\n")
	cg.buf.WriteString("inline bool s1_is_some(const Option& o) { return o.is_some(); }\n")
	cg.buf.WriteString("inline bool s2_is_none(const Option& o) { return o.is_none(); }\n")
	cg.buf.WriteString("inline long long s1_unwrap(const Option& o) { return o.unwrap(); }\n")
	cg.buf.WriteString("inline Option Some(long long v) { return Option{ true, v }; }\n")
	cg.buf.WriteString("inline const Option None = Option{ false, 0 };\n\n")

	// Result Runtime Type
	cg.buf.WriteString("struct Result {\n")
	cg.buf.WriteString("    bool is_ok;\n")
	cg.buf.WriteString("    long long val;\n")
	cg.buf.WriteString("    std::string err;\n")
	cg.buf.WriteString("};\n\n")
	cg.buf.WriteString("inline Result Ok(long long v) { return Result{ true, v, \"\" }; }\n")
	cg.buf.WriteString("inline Result Err(std::string msg) { return Result{ false, 0, msg }; }\n\n")

	// Forward declarations pass
	for _, stmt := range program.Statements {
		if st, ok := stmt.(*ast.StructDeclStmt); ok {
			cg.structTypes[st.Name.Value] = true
			if len(st.TypeParams) > 0 {
				var tParams []string
				for _, tp := range st.TypeParams {
					tParams = append(tParams, "typename "+tp)
				}
				cg.buf.WriteString(fmt.Sprintf("template <%s>\nstruct %s;\n", strings.Join(tParams, ", "), st.Name.Value))
			} else {
				cg.buf.WriteString(fmt.Sprintf("struct %s;\n", st.Name.Value))
			}
		}
		if fn, ok := stmt.(*ast.FnDeclStmt); ok && fn.Receiver != nil {
			cg.methodReceivers[fn.Name.Value] = fn.Receiver.Type
		}
		if imp, ok := stmt.(*ast.ImplDeclStmt); ok {
			for _, m := range imp.Methods {
				cg.methodReceivers[m.Name.Value] = imp.TargetType
			}
		}
		if ext, ok := stmt.(*ast.ExternFnStmt); ok {
			cg.externFns[ext.Name.Value] = ext
		}
	}
	if len(cg.structTypes) > 0 {
		cg.buf.WriteString("\n")
	}

	for _, stmt := range program.Statements {
		if err := cg.generateStatement(stmt); err != nil {
			return "", err
		}
	}

	return cg.buf.String(), nil
}

func (cg *CodeGenerator) generateStatement(stmt ast.Statement) error {
	switch s := stmt.(type) {
	case *ast.ImportStmt:
		return nil

	case *ast.VarDeclStmt:
		valStr, err := cg.generateExpression(s.Value)
		if err != nil {
			return err
		}
		varType := mapType(s.Type)

		cg.buf.WriteString(fmt.Sprintf("%sauto %s = %s;\n", cg.indent(), s.Name.Value, valStr))
		_ = varType

	case *ast.TupleVarDeclStmt:
		valStr, err := cg.generateExpression(s.Value)
		if err != nil {
			return err
		}
		var names []string
		for _, n := range s.Names {
			names = append(names, n.Value)
		}
		cg.buf.WriteString(fmt.Sprintf("%sauto [%s] = %s;\n", cg.indent(), strings.Join(names, ", "), valStr))

	case *ast.FnDeclStmt:
		cg.currentFn = s.Name.Value
		retType := mapType(s.ReturnType)
		funcName := s.Name.Value
		if s.Name.Value == "main" {
			retType = "int"
		}

		if len(s.TypeParams) > 0 {
			var tParams []string
			for _, tp := range s.TypeParams {
				tParams = append(tParams, "typename "+tp)
			}
			cg.buf.WriteString(fmt.Sprintf("%stemplate <%s>\n", cg.indent(), strings.Join(tParams, ", ")))
		}

		if s.IsAsync {
			retType = fmt.Sprintf("std::future<%s>", retType)
		}

		var params []string
		if s.Receiver != nil {
			funcName = s.Receiver.Type + "_" + s.Name.Value
			recvType := mapType(s.Receiver.Type)
			params = append(params, fmt.Sprintf("%s& %s", recvType, s.Receiver.Name.Value))
		} else if len(s.Params) > 0 && s.Params[0].Name.Value == "self" {
			funcName = s.Params[0].Type + "_" + s.Name.Value
		}

		cg.currentFn = funcName
		cg.deferredStmts = []ast.Expression{}
		cg.fnParams[funcName] = s.Params

		for _, p := range s.Params {
			pType := mapType(p.Type)
			if p.DefaultValue != nil {
				defStr, _ := cg.generateExpression(p.DefaultValue)
				params = append(params, fmt.Sprintf("%s %s = %s", pType, p.Name.Value, defStr))
			} else {
				params = append(params, fmt.Sprintf("%s %s", pType, p.Name.Value))
			}
		}

		inlinePrefix := ""
		for _, dec := range s.Decorators {
			if dec == "inline" {
				inlinePrefix = "inline "
			}
		}

		cg.buf.WriteString(fmt.Sprintf("%s%s%s %s(%s) {\n", cg.indent(), inlinePrefix, retType, funcName, strings.Join(params, ", ")))
		cg.indentLevel++

		if s.IsAsync {
			cg.buf.WriteString(fmt.Sprintf("%sreturn std::async(std::launch::async, [=]() mutable {\n", cg.indent()))
			cg.indentLevel++
			for _, stmt := range s.Body.Statements {
				if err := cg.generateStatement(stmt); err != nil {
					return err
				}
			}
			cg.emitDeferred()
			cg.indentLevel--
			cg.buf.WriteString(fmt.Sprintf("%s});\n", cg.indent()))
		} else {
			for _, stmt := range s.Body.Statements {
				if err := cg.generateStatement(stmt); err != nil {
					return err
				}
			}
			cg.emitDeferred()
		}

		if funcName == "main" {
			cg.buf.WriteString(fmt.Sprintf("%sreturn 0;\n", cg.indent()))
		}
		cg.indentLevel--
		cg.buf.WriteString(fmt.Sprintf("%s}\n\n", cg.indent()))

		if s.IsRPC {
			var argNames []string
			for _, p := range s.Params {
				argNames = append(argNames, p.Name.Value)
			}
			cg.buf.WriteString(fmt.Sprintf("inline %s %s_rpc_call(const std::string& node_endpoint, %s) {\n", retType, funcName, strings.Join(params, ", ")))
			cg.buf.WriteString(fmt.Sprintf("    std::cout << \"[RPC Zero-Copy Engine] Dispatching RPC payload to '\" << node_endpoint << \"' -> Function: %s\" << std::endl;\n", funcName))
			cg.buf.WriteString(fmt.Sprintf("    return %s(%s);\n", funcName, strings.Join(argNames, ", ")))
			cg.buf.WriteString("}\n\n")
		}

	case *ast.TraitDeclStmt:
		cg.buf.WriteString(fmt.Sprintf("%sstruct %s {\n", cg.indent(), s.Name.Value))
		cg.indentLevel++
		for _, m := range s.Methods {
			retType := mapType(m.ReturnType)
			var params []string
			for _, p := range m.Params {
				if p.Name.Value != "self" {
					params = append(params, fmt.Sprintf("%s %s", mapType(p.Type), p.Name.Value))
				}
			}
			cg.buf.WriteString(fmt.Sprintf("%svirtual %s %s(%s) = 0;\n", cg.indent(), retType, m.Name.Value, strings.Join(params, ", ")))
		}
		cg.buf.WriteString(fmt.Sprintf("%svirtual ~%s() = default;\n", cg.indent(), s.Name.Value))
		cg.indentLevel--
		cg.buf.WriteString(fmt.Sprintf("%s};\n\n", cg.indent()))

	case *ast.ImplDeclStmt:
		for _, m := range s.Methods {
			if err := cg.generateStatement(m); err != nil {
				return err
			}
			if m.Name.Value == "add" {
				cg.buf.WriteString(fmt.Sprintf("inline %s operator+(const %s& a, const %s& b) { return %s_add(a, b); }\n", s.TargetType, s.TargetType, s.TargetType, s.TargetType))
			} else if m.Name.Value == "sub" {
				cg.buf.WriteString(fmt.Sprintf("inline %s operator-(const %s& a, const %s& b) { return %s_sub(a, b); }\n", s.TargetType, s.TargetType, s.TargetType, s.TargetType))
			} else if m.Name.Value == "mul" {
				cg.buf.WriteString(fmt.Sprintf("inline %s operator*(const %s& a, const %s& b) { return %s_mul(a, b); }\n", s.TargetType, s.TargetType, s.TargetType, s.TargetType))
			} else if m.Name.Value == "div" {
				cg.buf.WriteString(fmt.Sprintf("inline %s operator/(const %s& a, const %s& b) { return %s_div(a, b); }\n", s.TargetType, s.TargetType, s.TargetType, s.TargetType))
			}
		}

	case *ast.SpawnStmt:
		callStr, err := cg.generateExpression(s.Call)
		if err != nil {
			return err
		}
		if _, isFnLit := s.Call.(*ast.FnLiteral); isFnLit {
			cg.buf.WriteString(fmt.Sprintf("%sstd::thread([=]() { (%s)(); }).detach();\n", cg.indent(), callStr))
		} else {
			cg.buf.WriteString(fmt.Sprintf("%sstd::thread([=]() { %s; }).detach();\n", cg.indent(), callStr))
		}

	case *ast.ExternFnStmt:
		retType := mapExternType(s.ReturnType)
		var params []string
		for _, p := range s.Params {
			params = append(params, fmt.Sprintf("%s %s", mapExternType(p.Type), p.Name.Value))
		}
		cg.buf.WriteString(fmt.Sprintf("%sextern %q %s %s(%s);\n", cg.indent(), s.Abi, retType, s.Name.Value, strings.Join(params, ", ")))

	case *ast.StructDeclStmt:
		cg.structFields[s.Name.Value] = s.Fields
		if len(s.TypeParams) > 0 {
			var tParams []string
			for _, tp := range s.TypeParams {
				tParams = append(tParams, "typename "+tp)
			}
			cg.buf.WriteString(fmt.Sprintf("%stemplate <%s>\n", cg.indent(), strings.Join(tParams, ", ")))
		}
		cg.buf.WriteString(fmt.Sprintf("%sstruct %s {\n", cg.indent(), s.Name.Value))
		cg.indentLevel++
		for _, f := range s.Fields {
			fType := mapType(f.Type)
			cg.buf.WriteString(fmt.Sprintf("%s%s %s;\n", cg.indent(), fType, f.Name.Value))
		}
		cg.indentLevel--
		cg.buf.WriteString(fmt.Sprintf("%s};\n\n", cg.indent()))

		// Process @derive(...) macros
		for _, dec := range s.Decorators {
			if strings.HasPrefix(dec, "derive(") {
				inner := strings.TrimSuffix(strings.TrimPrefix(dec, "derive("), ")")
				traits := strings.Split(inner, ",")
				for _, t := range traits {
					trait := strings.TrimSpace(t)
					if trait == "Debug" {
						var strParts []string
						for _, f := range s.Fields {
							strParts = append(strParts, fmt.Sprintf("\"%s=\" + str(self.%s)", f.Name.Value, f.Name.Value))
						}
						fmtStr := fmt.Sprintf("%q + std::string(\"(\") + %s + std::string(\")\")", s.Name.Value, strings.Join(strParts, " + \", \" + "))
						if len(s.Fields) == 0 {
							fmtStr = fmt.Sprintf("%q + std::string(\"()\")", s.Name.Value)
						}
						cg.methodReceivers["to_string"] = s.Name.Value
						cg.buf.WriteString(fmt.Sprintf("inline std::string %s_to_string(const %s& self) {\n    return %s;\n}\n\n", s.Name.Value, s.Name.Value, fmtStr))
					} else if trait == "Clone" {
						cg.methodReceivers["clone"] = s.Name.Value
						var fCopies []string
						for _, f := range s.Fields {
							fCopies = append(fCopies, fmt.Sprintf("self.%s", f.Name.Value))
						}
						cg.buf.WriteString(fmt.Sprintf("inline %s %s_clone(const %s& self) {\n    return %s{ %s };\n}\n\n", s.Name.Value, s.Name.Value, s.Name.Value, s.Name.Value, strings.Join(fCopies, ", ")))
					} else if trait == "Eq" {
						cg.methodReceivers["equals"] = s.Name.Value
						var eqParts []string
						for _, f := range s.Fields {
							eqParts = append(eqParts, fmt.Sprintf("self.%s == other.%s", f.Name.Value, f.Name.Value))
						}
						eqCond := strings.Join(eqParts, " && ")
						if len(s.Fields) == 0 {
							eqCond = "true"
						}
						cg.buf.WriteString(fmt.Sprintf("inline bool %s_equals(const %s& self, const %s& other) {\n    return %s;\n}\n", s.Name.Value, s.Name.Value, s.Name.Value, eqCond))
						cg.buf.WriteString(fmt.Sprintf("inline bool operator==(const %s& a, const %s& b) { return %s_equals(a, b); }\n\n", s.Name.Value, s.Name.Value, s.Name.Value))
					}
				}
			}
		}

	case *ast.DeferStmt:
		cg.deferredStmts = append(cg.deferredStmts, s.Expr)

	case *ast.ReturnStmt:
		cg.emitDeferred()
		if s.Value == nil {
			if cg.currentFn == "main" {
				cg.buf.WriteString(fmt.Sprintf("%sreturn 0;\n", cg.indent()))
			} else {
				cg.buf.WriteString(fmt.Sprintf("%sreturn;\n", cg.indent()))
			}
		} else {
			valStr, err := cg.generateExpression(s.Value)
			if err != nil {
				return err
			}
			cg.buf.WriteString(fmt.Sprintf("%sreturn %s;\n", cg.indent(), valStr))
		}

	case *ast.IfStmt:
		condStr, err := cg.generateExpression(s.Condition)
		if err != nil {
			return err
		}
		cg.buf.WriteString(fmt.Sprintf("%sif (%s) {\n", cg.indent(), condStr))
		cg.indentLevel++
		for _, bStmt := range s.Consequence.Statements {
			if err := cg.generateStatement(bStmt); err != nil {
				return err
			}
		}
		cg.indentLevel--
		cg.buf.WriteString(fmt.Sprintf("%s}", cg.indent()))

		for _, elif := range s.Elifs {
			elifCond, err := cg.generateExpression(elif.Condition)
			if err != nil {
				return err
			}
			cg.buf.WriteString(fmt.Sprintf(" else if (%s) {\n", elifCond))
			cg.indentLevel++
			for _, bStmt := range elif.Consequence.Statements {
				if err := cg.generateStatement(bStmt); err != nil {
					return err
				}
			}
			cg.indentLevel--
			cg.buf.WriteString(fmt.Sprintf("%s}", cg.indent()))
		}

		if s.Alternative != nil {
			cg.buf.WriteString(" else {\n")
			cg.indentLevel++
			for _, bStmt := range s.Alternative.Statements {
				if err := cg.generateStatement(bStmt); err != nil {
					return err
				}
			}
			cg.indentLevel--
			cg.buf.WriteString(fmt.Sprintf("%s}\n", cg.indent()))
		} else {
			cg.buf.WriteString("\n")
		}

	case *ast.WhileStmt:
		condStr, err := cg.generateExpression(s.Condition)
		if err != nil {
			return err
		}
		cg.buf.WriteString(fmt.Sprintf("%swhile (%s) {\n", cg.indent(), condStr))
		cg.indentLevel++
		for _, bStmt := range s.Body.Statements {
			if err := cg.generateStatement(bStmt); err != nil {
				return err
			}
		}
		cg.indentLevel--
		cg.buf.WriteString(fmt.Sprintf("%s}\n", cg.indent()))

	case *ast.ForInStmt:
		if rng, ok := s.Iterable.(*ast.RangeExpr); ok {
			startStr, _ := cg.generateExpression(rng.Start)
			endStr, _ := cg.generateExpression(rng.End)
			cg.buf.WriteString(fmt.Sprintf("%sfor (auto %s = %s; %s < %s; ++%s) {\n", cg.indent(), s.VarName.Value, startStr, s.VarName.Value, endStr, s.VarName.Value))
			cg.indentLevel++
			for _, bStmt := range s.Body.Statements {
				if err := cg.generateStatement(bStmt); err != nil {
					return err
				}
			}
			cg.indentLevel--
			cg.buf.WriteString(fmt.Sprintf("%s}\n", cg.indent()))
			return nil
		}

		iterStr, err := cg.generateExpression(s.Iterable)
		if err != nil {
			return err
		}
		cg.buf.WriteString(fmt.Sprintf("%sfor (auto&& %s : %s) {\n", cg.indent(), s.VarName.Value, iterStr))
		cg.indentLevel++
		for _, bStmt := range s.Body.Statements {
			if err := cg.generateStatement(bStmt); err != nil {
				return err
			}
		}
		cg.indentLevel--
		cg.buf.WriteString(fmt.Sprintf("%s}\n", cg.indent()))

	case *ast.MatchStmt:
		matchVar := fmt.Sprintf("_match_val_%d", s.Token.Line)
		exprStr, err := cg.generateExpression(s.Expr)
		if err != nil {
			return err
		}
		cg.buf.WriteString(fmt.Sprintf("%sauto %s = %s;\n", cg.indent(), matchVar, exprStr))

		for idx, c := range s.Cases {
			var condStr string
			var bindingVar string
			var bindingExpr string

			if call, ok := c.Pattern.(*ast.CallExpr); ok {
				if fnId, ok := call.Function.(*ast.Identifier); ok {
					if fnId.Value == "Some" && len(call.Arguments) == 1 {
						condStr = fmt.Sprintf("%s.is_some", matchVar)
						if argId, ok := call.Arguments[0].(*ast.Identifier); ok {
							bindingVar = argId.Value
							bindingExpr = fmt.Sprintf("%s.val", matchVar)
						}
					} else if fnId.Value == "Ok" && len(call.Arguments) == 1 {
						condStr = fmt.Sprintf("%s.is_ok", matchVar)
						if argId, ok := call.Arguments[0].(*ast.Identifier); ok {
							bindingVar = argId.Value
							bindingExpr = fmt.Sprintf("%s.val", matchVar)
						}
					} else if fnId.Value == "Err" && len(call.Arguments) == 1 {
						condStr = fmt.Sprintf("!%s.is_ok", matchVar)
						if argId, ok := call.Arguments[0].(*ast.Identifier); ok {
							bindingVar = argId.Value
							bindingExpr = fmt.Sprintf("%s.err", matchVar)
						}
					}
				}
			} else if patRng, ok := c.Pattern.(*ast.RangeExpr); ok {
				startStr, _ := cg.generateExpression(patRng.Start)
				endStr, _ := cg.generateExpression(patRng.End)
				condStr = fmt.Sprintf("(%s >= %s && %s <= %s)", matchVar, startStr, matchVar, endStr)
			} else if patInfix, ok := c.Pattern.(*ast.InfixExpr); ok && patInfix.Operator == ".." {
				startStr, _ := cg.generateExpression(patInfix.Left)
				endStr, _ := cg.generateExpression(patInfix.Right)
				condStr = fmt.Sprintf("(%s >= %s && %s <= %s)", matchVar, startStr, matchVar, endStr)
			} else if id, ok := c.Pattern.(*ast.Identifier); ok {
				if id.Value == "None" {
					condStr = fmt.Sprintf("!%s.is_some", matchVar)
				} else if id.Value == "_" {
					condStr = "true"
				} else {
					bindingVar = id.Value
					bindingExpr = matchVar
					condStr = "true"
				}
			}

			if condStr == "" {
				patStr, err := cg.generateExpression(c.Pattern)
				if err != nil {
					return err
				}
				condStr = fmt.Sprintf("%s == %s", matchVar, patStr)
			}

			initClause := ""
			if bindingVar != "" {
				initClause = fmt.Sprintf("auto %s = %s; ", bindingVar, bindingExpr)
			}

			if c.Guard != nil {
				guardStr, err := cg.generateExpression(c.Guard)
				if err != nil {
					return err
				}
				if condStr != "true" {
					condStr = fmt.Sprintf("(%s) && (%s)", condStr, guardStr)
				} else {
					condStr = guardStr
				}
			}

			keyword := "if"
			if idx > 0 {
				keyword = "else if"
			}

			if condStr == "true" && idx == len(s.Cases)-1 {
				keyword = "else"
				cg.buf.WriteString(fmt.Sprintf("%s%s {\n", cg.indent(), keyword))
			} else {
				cg.buf.WriteString(fmt.Sprintf("%s%s (%s%s) {\n", cg.indent(), keyword, initClause, condStr))
			}

			cg.indentLevel++
			if err := cg.generateStatement(c.Body); err != nil {
				return err
			}
			cg.indentLevel--
			cg.buf.WriteString(fmt.Sprintf("%s}\n", cg.indent()))
		}

	case *ast.ExprStmt:
		if s.Expr != nil {
			exprStr, err := cg.generateExpression(s.Expr)
			if err != nil {
				return err
			}
			cg.buf.WriteString(fmt.Sprintf("%s%s;\n", cg.indent(), exprStr))
		}

	default:
		return fmt.Errorf("unsupported statement: %T", stmt)
	}

	return nil
}

func (cg *CodeGenerator) hoistTryExprs(expr ast.Expression) (string, error) {
	if try, ok := expr.(*ast.TryExpr); ok {
		innerStr, err := cg.generateExpression(try.Expr)
		if err != nil {
			return "", err
		}
		varName := fmt.Sprintf("_try_res_%d", try.Token.Line)
		cg.buf.WriteString(fmt.Sprintf("%sauto %s = %s;\n", cg.indent(), varName, innerStr))
		cg.buf.WriteString(fmt.Sprintf("%sif (!%s.is_ok) return %s;\n", cg.indent(), varName, varName))
		return fmt.Sprintf("%s.val", varName), nil
	}
	return "", fmt.Errorf("not a try expr")
}

func (cg *CodeGenerator) generateExpression(expr ast.Expression) (string, error) {
	if expr == nil {
		return "", nil
	}
	switch e := expr.(type) {
	case *ast.TryExpr:
		innerStr, err := cg.generateExpression(e.Expr)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("([&]() { auto _res = %s; return _res.val; })()", innerStr), nil

	case *ast.OptChainExpr:
		objStr, err := cg.generateExpression(e.Object)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("([&]() { auto _opt_obj = %s; return _opt_obj ? _opt_obj->%s : decltype(_opt_obj->%s){}; })()", objStr, e.Member.Value, e.Member.Value), nil

	case *ast.IntegerLiteral:
		return fmt.Sprintf("%dLL", e.Value), nil

	case *ast.FloatLiteral:
		return fmt.Sprintf("%f", e.Value), nil

	case *ast.StringLiteral:
		return fmt.Sprintf("std::string(%q)", e.Value), nil

	case *ast.FStringLiteral:
		var parts []string
		for _, p := range e.Parts {
			pStr, err := cg.generateExpression(p)
			if err != nil {
				return "", err
			}
			parts = append(parts, fmt.Sprintf("str(%s)", pStr))
		}
		if len(parts) == 0 {
			return `""`, nil
		}
		return strings.Join(parts, " + "), nil

	case *ast.BoolLiteral:
		if e.Value {
			return "true", nil
		}
		return "false", nil

	case *ast.ArrayLiteral:
		if len(e.Elements) == 0 {
			return "CobaltEmptyVec{}", nil
		}
		var elems []string
		for _, el := range e.Elements {
			elStr, err := cg.generateExpression(el)
			if err != nil {
				return "", err
			}
			elems = append(elems, elStr)
		}
		return fmt.Sprintf("std::vector{ %s }", strings.Join(elems, ", ")), nil

	case *ast.MapLiteral:
		var pairs []string
		for i := 0; i < len(e.Keys); i++ {
			kStr, err := cg.generateExpression(e.Keys[i])
			if err != nil {
				return "", err
			}
			vStr, err := cg.generateExpression(e.Values[i])
			if err != nil {
				return "", err
			}
			pairs = append(pairs, fmt.Sprintf("{ %s, %s }", kStr, vStr))
		}
		return fmt.Sprintf("std::unordered_map<std::string, long long>{ %s }", strings.Join(pairs, ", ")), nil

	case *ast.TupleLiteral:
		var elems []string
		for _, el := range e.Elements {
			elStr, err := cg.generateExpression(el)
			if err != nil {
				return "", err
			}
			elems = append(elems, elStr)
		}
		return fmt.Sprintf("std::make_tuple(%s)", strings.Join(elems, ", ")), nil

	case *ast.TupleIndexExpr:
		tupStr, err := cg.generateExpression(e.Left)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("std::get<%d>(%s)", e.Index, tupStr), nil

	case *ast.Identifier:
		return e.Value, nil

	case *ast.IndexExpr:
		leftStr, err := cg.generateExpression(e.Left)
		if err != nil {
			return "", err
		}
		if rng, ok := e.Index.(*ast.RangeExpr); ok {
			startStr, _ := cg.generateExpression(rng.Start)
			endStr, _ := cg.generateExpression(rng.End)
			return fmt.Sprintf("cobalt_slice(%s, %s, %s)", leftStr, startStr, endStr), nil
		}
		idxStr, err := cg.generateExpression(e.Index)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("cobalt_index(%s, %s)", leftStr, idxStr), nil

	case *ast.RangeExpr:
		startStr, _ := cg.generateExpression(e.Start)
		endStr, _ := cg.generateExpression(e.End)
		return fmt.Sprintf("cobalt_range(%s, %s)", startStr, endStr), nil

	case *ast.AwaitExpr:
		innerStr, err := cg.generateExpression(e.Expr)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s).get()", innerStr), nil

	case *ast.AsmExpr:
		return fmt.Sprintf("__asm__ __volatile__(%q)", e.Instruction), nil

	case *ast.PrefixExpr:
		right, err := cg.generateExpression(e.Right)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s%s)", e.Operator, right), nil

	case *ast.InfixExpr:
		left, err := cg.generateExpression(e.Left)
		if err != nil {
			return "", err
		}
		right, err := cg.generateExpression(e.Right)
		if err != nil {
			return "", err
		}
		op := e.Operator
		if op == "and" {
			op = "&&"
		} else if op == "or" {
			op = "||"
		} else if op == "??" {
			return fmt.Sprintf("([&]() { auto _lhs = %s; return bool(_lhs) ? _lhs : %s; })()", left, right), nil
		}
		return fmt.Sprintf("(%s %s %s)", left, op, right), nil

	case *ast.FnLiteral:
		var params []string
		for _, p := range e.Params {
			pType := mapType(p.Type)
			if pType == "auto" || pType == "" {
				pType = "auto"
			}
			params = append(params, fmt.Sprintf("%s %s", pType, p.Name.Value))
		}
		subCg := New()
		subCg.indentLevel = cg.indentLevel + 1
		subCg.structTypes = cg.structTypes
		subCg.methodReceivers = cg.methodReceivers
		for _, stmt := range e.Body.Statements {
			if err := subCg.generateStatement(stmt); err != nil {
				return "", err
			}
		}
		return fmt.Sprintf("[=](%s) {\n%s%s}", strings.Join(params, ", "), subCg.buf.String(), cg.indent()), nil

	case *ast.CallExpr:
		if mem, ok := e.Function.(*ast.MemberExpr); ok {
			if objId, ok := mem.Object.(*ast.Identifier); ok {
				if objId.Value == "fs" && mem.Member.Value == "read_file" && len(e.Arguments) == 1 {
					argStr, _ := cg.generateExpression(e.Arguments[0])
					return fmt.Sprintf("fs_read_file(%s)", argStr), nil
				}
				if objId.Value == "fs" && mem.Member.Value == "write_file" && len(e.Arguments) == 2 {
					arg1, _ := cg.generateExpression(e.Arguments[0])
					arg2, _ := cg.generateExpression(e.Arguments[1])
					return fmt.Sprintf("fs_write_file(%s, %s)", arg1, arg2), nil
				}
				if objId.Value == "env" && mem.Member.Value == "args" {
					return "env_args()", nil
				}
				if objId.Value == "sys" && mem.Member.Value == "now_millis" {
					return "sys_now_millis()", nil
				}
			}

			if mem.Member.Value == "sort" {
				objStr, _ := cg.generateExpression(mem.Object)
				return fmt.Sprintf("cobalt_sort(%s)", objStr), nil
			}
			if mem.Member.Value == "reverse" {
				objStr, _ := cg.generateExpression(mem.Object)
				return fmt.Sprintf("cobalt_reverse(%s)", objStr), nil
			}
			if mem.Member.Value == "contains" && len(e.Arguments) == 1 {
				objStr, _ := cg.generateExpression(mem.Object)
				argStr, _ := cg.generateExpression(e.Arguments[0])
				return fmt.Sprintf("cobalt_contains(%s, %s)", objStr, argStr), nil
			}
			if mem.Member.Value == "join" && len(e.Arguments) >= 1 {
				objStr, _ := cg.generateExpression(mem.Object)
				argStr, _ := cg.generateExpression(e.Arguments[0])
				return fmt.Sprintf("cobalt_join(%s, %s)", objStr, argStr), nil
			}
			if mem.Member.Value == "find" && len(e.Arguments) == 1 {
				objStr, _ := cg.generateExpression(mem.Object)
				argStr, _ := cg.generateExpression(e.Arguments[0])
				return fmt.Sprintf("cobalt_find(%s, %s)", objStr, argStr), nil
			}

			if mem.Member.Value == "send" && len(e.Arguments) == 1 {
				objStr, _ := cg.generateExpression(mem.Object)
				argStr, _ := cg.generateExpression(e.Arguments[0])
				return fmt.Sprintf("%s->send(%s)", objStr, argStr), nil
			}
			if mem.Member.Value == "recv" {
				objStr, _ := cg.generateExpression(mem.Object)
				return fmt.Sprintf("%s->recv()", objStr), nil
			}

			if mem.Member.Value == "append" && len(e.Arguments) == 1 {
				objStr, err := cg.generateExpression(mem.Object)
				if err != nil {
					return "", err
				}
				var args []string
				for _, arg := range e.Arguments {
					aStr, err := cg.generateExpression(arg)
					if err != nil {
						return "", err
					}
					args = append(args, aStr)
				}
				return fmt.Sprintf("%s.push_back(%s)", objStr, strings.Join(args, ", ")), nil
			}

			if mem.Member.Value == "map" && len(e.Arguments) == 1 {
				objStr, err := cg.generateExpression(mem.Object)
				if err != nil {
					return "", err
				}
				fnStr, err := cg.generateExpression(e.Arguments[0])
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("([&]() {\n%s    auto _src = %s;\n%s    auto _fn = %s;\n%s    using _out_t = decltype(_fn(_src[0]));\n%s    std::vector<_out_t> _res;\n%s    for (auto& _item : _src) _res.push_back(_fn(_item));\n%s    return _res;\n%s})()",
					cg.indent(), objStr, cg.indent(), fnStr, cg.indent(), cg.indent(), cg.indent(), cg.indent(), cg.indent()), nil
			}

			if mem.Member.Value == "filter" && len(e.Arguments) == 1 {
				objStr, err := cg.generateExpression(mem.Object)
				if err != nil {
					return "", err
				}
				fnStr, err := cg.generateExpression(e.Arguments[0])
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("([&]() {\n%s    auto _src = %s;\n%s    auto _fn = %s;\n%s    std::vector<typename decltype(_src)::value_type> _res;\n%s    for (auto& _item : _src) { if (_fn(_item)) _res.push_back(_item); }\n%s    return _res;\n%s})()",
					cg.indent(), objStr, cg.indent(), fnStr, cg.indent(), cg.indent(), cg.indent(), cg.indent()), nil
			}

			if mem.Member.Value == "len" {
				objStr, err := cg.generateExpression(mem.Object)
				if err != nil { return "", err }
				return fmt.Sprintf("(long long)(%s).size()", objStr), nil
			}
			if mem.Member.Value == "trim" {
				objStr, err := cg.generateExpression(mem.Object)
				if err != nil { return "", err }
				return fmt.Sprintf("cobalt_trim(%s)", objStr), nil
			}
			if mem.Member.Value == "split" && len(e.Arguments) == 1 {
				objStr, err := cg.generateExpression(mem.Object)
				if err != nil { return "", err }
				delimStr, _ := cg.generateExpression(e.Arguments[0])
				return fmt.Sprintf("cobalt_split(%s, %s)", objStr, delimStr), nil
			}

			// Struct method call: rect.area() -> Rectangle_area(rect)
			objStr, err := cg.generateExpression(mem.Object)
			if err != nil {
				return "", err
			}

			structTypeName := cg.methodReceivers[mem.Member.Value]

			var args []string
			args = append(args, objStr)
			for _, arg := range e.Arguments {
				aStr, err := cg.generateExpression(arg)
				if err != nil {
					return "", err
				}
				args = append(args, aStr)
			}

			if structTypeName != "" {
				return fmt.Sprintf("%s_%s(%s)", structTypeName, mem.Member.Value, strings.Join(args, ", ")), nil
			}
			return fmt.Sprintf("%s_%s(%s)", objStr, mem.Member.Value, strings.Join(args, ", ")), nil
		}

		fnName, err := cg.generateExpression(e.Function)
		if err != nil {
			return "", err
		}

		if fnName == "Channel" {
			return "create_channel()", nil
		}

		if fnName == "Box" && len(e.Arguments) == 1 {
			argStr, _ := cg.generateExpression(e.Arguments[0])
			return fmt.Sprintf("std::make_unique<long long>(%s)", argStr), nil
		}

		if fnName == "Rc" && len(e.Arguments) == 1 {
			argStr, _ := cg.generateExpression(e.Arguments[0])
			return fmt.Sprintf("std::make_shared<long long>(%s)", argStr), nil
		}

		if fnName == "len" && len(e.Arguments) == 1 {
			argStr, _ := cg.generateExpression(e.Arguments[0])
			return fmt.Sprintf("(long long)(%s).size()", argStr), nil
		}

		if fnName == "field_names" && len(e.Arguments) == 1 {
			targetStruct := ""
			if strLit, ok := e.Arguments[0].(*ast.StringLiteral); ok {
				targetStruct = strLit.Value
			} else if id, ok := e.Arguments[0].(*ast.Identifier); ok {
				targetStruct = id.Value
			}
			var fNames []string
			if fields, ok := cg.structFields[targetStruct]; ok {
				for _, f := range fields {
					fNames = append(fNames, fmt.Sprintf("%q", f.Name.Value))
				}
			}
			return fmt.Sprintf("std::vector<std::string>{ %s }", strings.Join(fNames, ", ")), nil
		}

		if fnName == "field_count" && len(e.Arguments) == 1 {
			targetStruct := ""
			if strLit, ok := e.Arguments[0].(*ast.StringLiteral); ok {
				targetStruct = strLit.Value
			} else if id, ok := e.Arguments[0].(*ast.Identifier); ok {
				targetStruct = id.Value
			}
			count := 0
			if fields, ok := cg.structFields[targetStruct]; ok {
				count = len(fields)
			}
			return fmt.Sprintf("%dLL", count), nil
		}

		if fnName == "type_name" && len(e.Arguments) == 1 {
			if id, ok := e.Arguments[0].(*ast.Identifier); ok {
				return fmt.Sprintf("%q", id.Value), nil
			}
			return fmt.Sprintf("%q", "object"), nil
		}

		var args []string
		for idx, arg := range e.Arguments {
			aStr, err := cg.generateExpression(arg)
			if err != nil {
				return "", err
			}
			if extFn, isExtern := cg.externFns[fnName]; isExtern {
				if idx < len(extFn.Params) && extFn.Params[idx].Type == "string" {
					aStr = fmt.Sprintf("(%s).c_str()", aStr)
				}
			}
			args = append(args, aStr)
		}

		// Struct constructor instantiation Point(10, 20) -> Point{ 10LL, 20LL }
		if cg.structTypes[fnName] {
			return fmt.Sprintf("%s{ %s }", fnName, strings.Join(args, ", ")), nil
		}

		return fmt.Sprintf("%s(%s)", fnName, strings.Join(args, ", ")), nil

	case *ast.MemberExpr:
		objStr, err := cg.generateExpression(e.Object)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.%s", objStr, e.Member.Value), nil


	default:
		return "", fmt.Errorf("unsupported expression: %T", expr)
	}
}

func mapExternType(t string) string {
	switch t {
	case "int":
		return "int"
	case "float":
		return "double"
	case "string":
		return "const char*"
	case "bool":
		return "bool"
	case "":
		return "void"
	default:
		return t
	}
}

func mapType(t string) string {
	if strings.HasSuffix(t, "[]") {
		elemType := strings.TrimSuffix(t, "[]")
		return fmt.Sprintf("std::vector<%s>", mapType(elemType))
	}
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		inner := t[1 : len(t)-1]
		parts := strings.Split(inner, ",")
		var mapped []string
		for _, p := range parts {
			mapped = append(mapped, mapType(strings.TrimSpace(p)))
		}
		return fmt.Sprintf("std::tuple<%s>", strings.Join(mapped, ", "))
	}
	switch t {
	case "int":
		return "long long"
	case "float":
		return "double"
	case "string":
		return "std::string"
	case "bool":
		return "bool"
	case "Channel":
		return "Channel"
	case "Option":
		return "Option"
	case "Result":
		return "Result"
	case "":
		return "void"
	default:
		return t
	}
}

func (cg *CodeGenerator) emitDeferred() {
	for i := len(cg.deferredStmts) - 1; i >= 0; i-- {
		defStr, _ := cg.generateExpression(cg.deferredStmts[i])
		cg.buf.WriteString(fmt.Sprintf("%s%s;\n", cg.indent(), defStr))
	}
	cg.deferredStmts = nil
}
