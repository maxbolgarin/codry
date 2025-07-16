package main

import (
	"context"
	"fmt"
	"log"

	"github.com/maxbolgarin/codry/internal/reviewer/astparser"
)

func main() {
	// Test the improved AST parser
	parser := astparser.NewParser()
	
	// Test Go code
	goCode := `
package main

import "fmt"

// User represents a user in the system
type User struct {
	ID    int    ` + "`json:\"id\"`" + `
	Name  string ` + "`json:\"name\"`" + `
	Email string ` + "`json:\"email\"`" + `
}

// NewUser creates a new user
func NewUser(name, email string) *User {
	return &User{
		Name:  name,
		Email: email,
	}
}

// GetName returns the user's name
func (u *User) GetName() string {
	return u.Name
}

// SetName sets the user's name
func (u *User) SetName(name string) {
	u.Name = name
}

func main() {
	user := NewUser("John Doe", "john@example.com")
	fmt.Println("User:", user.GetName())
}
`
	
	ctx := context.Background()
	rootNode, err := parser.ParseFileToAST(ctx, "test.go", goCode)
	if err != nil {
		log.Fatal("Failed to parse Go code:", err)
	}
	
	// Find all symbols
	symbols, err := parser.findAllSymbolsInFile("test.go", goCode)
	if err != nil {
		log.Fatal("Failed to find symbols:", err)
	}
	
	fmt.Println("Found symbols in Go code:")
	for _, symbol := range symbols {
		fmt.Printf("- %s (%s) at lines %d-%d\n", symbol.Name, symbol.Type, symbol.StartLine, symbol.EndLine)
		if len(symbol.Dependencies) > 0 {
			fmt.Printf("  Dependencies: %d\n", len(symbol.Dependencies))
		}
	}
	
	// Test JavaScript code
	jsCode := `
// User management system
class User {
	constructor(name, email) {
		this.name = name;
		this.email = email;
	}
	
	getName() {
		return this.name;
	}
	
	setName(name) {
		this.name = name;
	}
}

function createUser(name, email) {
	return new User(name, email);
}

const user = createUser("Jane Doe", "jane@example.com");
console.log("User:", user.getName());
`
	
	rootNode, err = parser.ParseFileToAST(ctx, "test.js", jsCode)
	if err != nil {
		log.Fatal("Failed to parse JavaScript code:", err)
	}
	
	symbols, err = parser.findAllSymbolsInFile("test.js", jsCode)
	if err != nil {
		log.Fatal("Failed to find symbols:", err)
	}
	
	fmt.Println("\nFound symbols in JavaScript code:")
	for _, symbol := range symbols {
		fmt.Printf("- %s (%s) at lines %d-%d\n", symbol.Name, symbol.Type, symbol.StartLine, symbol.EndLine)
		if len(symbol.Dependencies) > 0 {
			fmt.Printf("  Dependencies: %d\n", len(symbol.Dependencies))
		}
	}
}