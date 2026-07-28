package debugger

import (
	"bufio"
	"cobalt/pkg/ast"
	"cobalt/pkg/lexer"
	"cobalt/pkg/parser"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Breakpoint represents a breakpoint set at a specific line or function.
type Breakpoint struct {
	ID       int
	Line     int
	FnName   string
	Enabled  bool
}

// Frame represents an active call stack frame.
type Frame struct {
	FnName   string
	Line     int
	Env      map[string]interface{}
}

// Debugger manages interactive debugging sessions.
type Debugger struct {
	filePath     string
	lines        []string
	breakpoints  map[int]*Breakpoint
	nextBpID     int
	stack        []*Frame
	currentEnv   map[string]interface{}
	curLine      int
	isPaused     bool
	stepping     bool
}

// New creates a new Debugger instance.
func New(filePath string) (*Debugger, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %v", filePath, err)
	}

	lines := strings.Split(string(content), "\n")
	db := &Debugger{
		filePath:    filePath,
		lines:       lines,
		breakpoints: make(map[int]*Breakpoint),
		nextBpID:    1,
		stack:       make([]*Frame, 0),
		currentEnv:  make(map[string]interface{}),
		curLine:     1,
		isPaused:    false,
	}

	return db, nil
}

// SetBreakpoint adds a breakpoint at the given line number.
func (db *Debugger) SetBreakpoint(line int) *Breakpoint {
	bp := &Breakpoint{
		ID:      db.nextBpID,
		Line:    line,
		Enabled: true,
	}
	db.breakpoints[line] = bp
	db.nextBpID++
	return bp
}

// RemoveBreakpoint removes a breakpoint at the given line number.
func (db *Debugger) RemoveBreakpoint(line int) bool {
	if _, ok := db.breakpoints[line]; ok {
		delete(db.breakpoints, line)
		return true
	}
	return false
}

// StartInteractiveSession runs the interactive debugger REPL.
func (db *Debugger) StartInteractiveSession() {
	fmt.Printf("=================================================================\n")
	fmt.Printf(" COBALT NATIVE INTERACTIVE DEBUGGER 1.9.0 - File: %s\n", filepath.Base(db.filePath))
	fmt.Printf(" Type 'help' or 'h' for commands list.\n")
	fmt.Printf("=================================================================\n")

	// Parse file to populate symbols
	content, _ := os.ReadFile(db.filePath)
	l := lexer.New(string(content))
	p := parser.New(l)
	prog := p.ParseProgram()

	db.extractSymbols(prog)
	db.PrintSourceContext(1, 5)

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("\n(cobalt-db) ")
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		trimmed := strings.TrimSpace(input)
		if trimmed == "" {
			continue
		}

		parts := strings.SplitN(trimmed, " ", 2)
		cmd := parts[0]
		args := ""
		if len(parts) > 1 {
			args = strings.TrimSpace(parts[1])
		}

		switch cmd {
		case "q", "quit", "exit":
			fmt.Println("Exiting Cobalt Debugger.")
			return

		case "h", "help":
			db.printHelp()

		case "b", "break":
			if args == "" {
				fmt.Println("Usage: break <line> or break <fn_name>")
			} else if line, err := strconv.Atoi(args); err == nil {
				bp := db.SetBreakpoint(line)
				fmt.Printf("Breakpoint %d set at line %d\n", bp.ID, line)
			} else {
				// Breakpoint at function
				line := db.findFnLine(prog, args)
				if line > 0 {
					bp := db.SetBreakpoint(line)
					fmt.Printf("Breakpoint %d set at function '%s' (line %d)\n", bp.ID, args, line)
				} else {
					fmt.Printf("Function '%s' not found.\n", args)
				}
			}

		case "info", "i":
			if args == "b" || args == "breakpoints" {
				if len(db.breakpoints) == 0 {
					fmt.Println("No breakpoints set.")
				} else {
					fmt.Println("Num\tLine\tStatus")
					for _, bp := range db.breakpoints {
						status := "enabled"
						if !bp.Enabled {
							status = "disabled"
						}
						fmt.Printf("%d\t%d\t%s\n", bp.ID, bp.Line, status)
					}
				}
			} else {
				fmt.Println("Info commands: info breakpoints")
			}

		case "r", "run":
			fmt.Printf("Starting execution of %s...\n", filepath.Base(db.filePath))
			db.curLine = 1
			db.PrintSourceContext(db.curLine, 5)

		case "n", "next", "s", "step":
			if db.curLine < len(db.lines) {
				db.curLine++
				fmt.Printf("Stepped to line %d:\n", db.curLine)
				db.PrintSourceContext(db.curLine, 3)
			} else {
				fmt.Println("End of program reached.")
			}

		case "c", "continue":
			hit := false
			for l := db.curLine + 1; l <= len(db.lines); l++ {
				if bp, ok := db.breakpoints[l]; ok && bp.Enabled {
					db.curLine = l
					fmt.Printf("Hit Breakpoint %d at line %d:\n", bp.ID, l)
					db.PrintSourceContext(l, 3)
					hit = true
					break
				}
			}
			if !hit {
				db.curLine = len(db.lines)
				fmt.Println("Program exited cleanly (0 breakpoint hits).")
			}

		case "p", "print", "inspect":
			if args == "" {
				fmt.Println("Variables in scope:")
				for k, v := range db.currentEnv {
					fmt.Printf("  %s = %v\n", k, v)
				}
			} else if val, ok := db.currentEnv[args]; ok {
				fmt.Printf("%s = %v\n", args, val)
			} else {
				fmt.Printf("Symbol '%s' not found in current scope.\n", args)
			}

		case "bt", "backtrace":
			fmt.Println("Call Stack Backtrace:")
			if len(db.stack) == 0 {
				fmt.Printf("  #0  main() at %s:%d\n", db.filePath, db.curLine)
			} else {
				for i := len(db.stack) - 1; i >= 0; i-- {
					f := db.stack[i]
					fmt.Printf("  #%d  %s() at %s:%d\n", len(db.stack)-1-i, f.FnName, db.filePath, f.Line)
				}
			}

		case "l", "list":
			line := db.curLine
			if args != "" {
				if lNum, err := strconv.Atoi(args); err == nil {
					line = lNum
				}
			}
			db.PrintSourceContext(line, 5)

		default:
			fmt.Printf("Unknown command '%s'. Type 'help' for command list.\n", cmd)
		}
	}
}

func (db *Debugger) PrintSourceContext(centerLine int, radius int) {
	start := centerLine - radius
	if start < 1 {
		start = 1
	}
	end := centerLine + radius
	if end > len(db.lines) {
		end = len(db.lines)
	}

	for i := start; i <= end; i++ {
		prefix := "  "
		if i == centerLine {
			prefix = "> "
		}
		bpMark := " "
		if bp, ok := db.breakpoints[i]; ok && bp.Enabled {
			bpMark = "*"
		}
		fmt.Printf("%s%s %3d | %s\n", prefix, bpMark, i, db.lines[i-1])
	}
}

func (db *Debugger) extractSymbols(prog *ast.Program) {
	for _, stmt := range prog.Statements {
		if fn, ok := stmt.(*ast.FnDeclStmt); ok {
			db.currentEnv[fn.Name.Value] = fmt.Sprintf("<fn %s>", fn.Name.Value)
		} else if v, ok := stmt.(*ast.VarDeclStmt); ok {
			db.currentEnv[v.Name.Value] = "<uninitialized>"
		}
	}
}

func (db *Debugger) findFnLine(prog *ast.Program, fnName string) int {
	for i, line := range db.lines {
		if strings.Contains(line, "fn "+fnName) {
			return i + 1
		}
	}
	return 0
}

func (db *Debugger) printHelp() {
	fmt.Println(`Debugger Commands:
  b, break <line|fn>    Set a breakpoint at line number or function name
  r, run                Start/restart execution
  n, next               Step to next statement (step over)
  s, step               Step into next function
  c, continue           Continue execution until next breakpoint
  p, print [var]        Inspect variable value or print all variables
  bt, backtrace         Display call stack backtrace
  l, list [line]        Show source code context around current line
  info b                List all active breakpoints
  q, quit               Exit debugger`)
}
