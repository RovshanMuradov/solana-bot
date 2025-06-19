package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// DeadCodeFinder finds potentially unused code in Go projects
type DeadCodeFinder struct {
	fileSet *token.FileSet

	// Track declarations and usage
	declarations map[string]*Declaration
	usages       map[string]bool

	// Integration verification
	phaseIntegrations map[string]bool
}

type Declaration struct {
	Name     string
	Type     string // function, variable, constant, type
	Package  string
	File     string
	Position token.Position
	Exported bool
	Comment  string
}

// Phase integration patterns to verify
var phasePatterns = map[string][]string{
	"Phase1": {
		"GetShutdownHandler", "NewSafeFileWriter", "NewSafeCSVWriter",
		"NewLogBuffer", "RegisterService",
	},
	"Phase2": {
		"NewPriceThrottler", "InitBus", "InitCache", "NewTradeHistory",
		"SendPriceUpdate", "GlobalCache", "SetPosition",
	},
	"Phase3": {
		"NewAlertManager", "NewUIManager", "CheckPosition",
		"exportTradeDataCmd", "AlertEvent",
	},
}

func NewDeadCodeFinder() *DeadCodeFinder {
	return &DeadCodeFinder{
		fileSet:           token.NewFileSet(),
		declarations:      make(map[string]*Declaration),
		usages:            make(map[string]bool),
		phaseIntegrations: make(map[string]bool),
	}
}

func (dcf *DeadCodeFinder) AnalyzeDirectory(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip non-Go files and test files for dead code analysis
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Skip vendor directory
		if strings.Contains(path, "vendor/") {
			return nil
		}

		return dcf.analyzeFile(path)
	})
}

func (dcf *DeadCodeFinder) analyzeFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	// Parse the Go source file
	node, err := parser.ParseFile(dcf.fileSet, filename, content, parser.ParseComments)
	if err != nil {
		return err
	}

	// Extract package name
	packageName := node.Name.Name

	// Walk the AST to find declarations and usages
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			dcf.recordFunction(x, packageName, filename)
		case *ast.GenDecl:
			dcf.recordGenDecl(x, packageName, filename)
		case *ast.CallExpr:
			dcf.recordUsage(x)
		case *ast.Ident:
			dcf.recordIdentUsage(x)
		}
		return true
	})

	// Check for phase integration patterns
	contentStr := string(content)
	dcf.checkPhaseIntegrations(contentStr)

	return nil
}

func (dcf *DeadCodeFinder) recordFunction(fn *ast.FuncDecl, pkg, file string) {
	if fn.Name == nil {
		return
	}

	name := fn.Name.Name
	exported := ast.IsExported(name)

	// Get comment if available
	comment := ""
	if fn.Doc != nil {
		comment = fn.Doc.Text()
	}

	dcf.declarations[pkg+"."+name] = &Declaration{
		Name:     name,
		Type:     "function",
		Package:  pkg,
		File:     file,
		Position: dcf.fileSet.Position(fn.Pos()),
		Exported: exported,
		Comment:  comment,
	}
}

func (dcf *DeadCodeFinder) recordGenDecl(gen *ast.GenDecl, pkg, file string) {
	for _, spec := range gen.Specs {
		switch s := spec.(type) {
		case *ast.ValueSpec:
			for _, name := range s.Names {
				if name.Name == "_" {
					continue
				}

				declType := "variable"
				if gen.Tok == token.CONST {
					declType = "constant"
				}

				dcf.declarations[pkg+"."+name.Name] = &Declaration{
					Name:     name.Name,
					Type:     declType,
					Package:  pkg,
					File:     file,
					Position: dcf.fileSet.Position(name.Pos()),
					Exported: ast.IsExported(name.Name),
				}
			}
		case *ast.TypeSpec:
			if s.Name.Name != "_" {
				dcf.declarations[pkg+"."+s.Name.Name] = &Declaration{
					Name:     s.Name.Name,
					Type:     "type",
					Package:  pkg,
					File:     file,
					Position: dcf.fileSet.Position(s.Name.Pos()),
					Exported: ast.IsExported(s.Name.Name),
				}
			}
		}
	}
}

func (dcf *DeadCodeFinder) recordUsage(call *ast.CallExpr) {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		dcf.usages[fun.Name] = true
	case *ast.SelectorExpr:
		if ident, ok := fun.X.(*ast.Ident); ok {
			dcf.usages[ident.Name+"."+fun.Sel.Name] = true
		}
		dcf.usages[fun.Sel.Name] = true
	}
}

func (dcf *DeadCodeFinder) recordIdentUsage(ident *ast.Ident) {
	if ident.Name != "_" {
		dcf.usages[ident.Name] = true
	}
}

func (dcf *DeadCodeFinder) checkPhaseIntegrations(content string) {
	for phase, patterns := range phasePatterns {
		for _, pattern := range patterns {
			if strings.Contains(content, pattern) {
				dcf.phaseIntegrations[phase+"."+pattern] = true
			}
		}
	}
}

func (dcf *DeadCodeFinder) FindDeadCode() []*Declaration {
	var deadCode []*Declaration

	for key, decl := range dcf.declarations {
		// Skip exported functions/types as they might be used externally
		if decl.Exported {
			continue
		}

		// Skip main functions
		if decl.Name == "main" || decl.Name == "init" {
			continue
		}

		// Skip functions with specific prefixes that indicate they're used by frameworks
		if strings.HasPrefix(decl.Name, "Test") ||
			strings.HasPrefix(decl.Name, "Benchmark") ||
			strings.HasPrefix(decl.Name, "Example") {
			continue
		}

		// Check if used
		used := dcf.usages[decl.Name] || dcf.usages[key]
		if !used {
			deadCode = append(deadCode, decl)
		}
	}

	return deadCode
}

func (dcf *DeadCodeFinder) CheckPhaseIntegration() map[string]map[string]bool {
	results := make(map[string]map[string]bool)

	for phase, patterns := range phasePatterns {
		results[phase] = make(map[string]bool)
		for _, pattern := range patterns {
			results[phase][pattern] = dcf.phaseIntegrations[phase+"."+pattern]
		}
	}

	return results
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run find_dead_code.go <directory>")
	}

	dir := os.Args[1]
	finder := NewDeadCodeFinder()

	fmt.Println("🔍 Dead Code Analysis")
	fmt.Println("====================")

	// Analyze the directory
	if err := finder.AnalyzeDirectory(dir); err != nil {
		log.Fatalf("Error analyzing directory: %v", err)
	}

	// Find dead code
	deadCode := finder.FindDeadCode()

	fmt.Printf("\n📊 Found %d potentially unused declarations:\n", len(deadCode))
	for _, decl := range deadCode {
		fmt.Printf("  • %s %s in %s:%d\n",
			decl.Type, decl.Name,
			filepath.Base(decl.File),
			decl.Position.Line)

		if decl.Comment != "" {
			comment := strings.TrimSpace(decl.Comment)
			if len(comment) > 50 {
				comment = comment[:50] + "..."
			}
			fmt.Printf("    Comment: %s\n", comment)
		}
	}

	// Check phase integration
	fmt.Println("\n🔄 Phase Integration Status:")
	fmt.Println("============================")

	phaseResults := finder.CheckPhaseIntegration()

	for phase, patterns := range phaseResults {
		fmt.Printf("\n%s:\n", phase)
		allIntegrated := true

		for pattern, integrated := range patterns {
			status := "❌"
			if integrated {
				status = "✅"
			} else {
				allIntegrated = false
			}
			fmt.Printf("  %s %s\n", status, pattern)
		}

		if allIntegrated {
			fmt.Printf("  🎉 %s FULLY INTEGRATED\n", phase)
		} else {
			fmt.Printf("  ⚠️  %s INCOMPLETE\n", phase)
		}
	}

	fmt.Println("\n✨ Analysis complete!")

	if len(deadCode) == 0 {
		fmt.Println("🎉 No dead code found!")
	} else {
		fmt.Printf("⚠️  Found %d potential dead code items for review\n", len(deadCode))
	}
}
