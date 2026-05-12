This AGENTS.md is designed to act as a "System Prompt" for AI tools working on your Go codebase. 
It focuses on Go-specific idioms
------------------------------
## AGENTS.md - Go Project Guidelines## Project Context

* Runtime: Go 1.23+ (utilizing generics and standard library slog).
* Architecture: Minimalist Service Layer. Avoid deep nesting or "Clean Architecture" over-engineering unless requested.
* Database: Clickhous using sql. Use go-migrate for schema changes. Use same sql as python implementation (copy them from python code)

## 🚀 Boilerplate Avoidance Rules
To keep the codebase lean, follow these rules strictly:

   1. Prefer go generate: Do not write mocks or serializers by hand.
   * Run go generate ./... to update mocks (using mockery) or stringers.
   2. Error Handling: Use fmt.Errorf("context: %w", err) for wrapping. Do not create custom error types unless they require specific behavior/methods.
   3. Table-Driven Tests: Always use anonymous structs for test cases to avoid repetitive if statements in _test.go files.
   4. Functional Options: For configuration-heavy constructors, use the Functional Options pattern instead of multiple New... variants.
   5. Generics: Use generics for collection utilities (e.g., Map, Filter, Keys) instead of duplicating logic for different types.

## 🛠 Coding Standards

* Receiver Names: Use short, 1-3 letter abbreviations (e.g., func (s *Server) not func (server *Server)).
* Zero Values: Return nil, err for pointers and Result{}, err for structs. Do not return "empty" initialized objects on error.
* Naming: Interface names should end in "-er" (e.g., Processor, Storer) when they define a single behavior.
* Logging: Use slog with structured attributes. Prefer slog.Info("msg", "key", value) over string formatting.

## 🚫 What NOT to do

* No init() functions: Use explicit initialization in main.go.
* No interface{}: Use any.
* No Pointer Overuse: Do not use pointers for small structs or slices unless mutation is required or the struct is large.
* No "Global" State: All dependencies (DB, Config) must be passed via constructors.

## Tooling Commands

* Linting: golangci-lint run
* Tidying: go mod tidy
* Formatting: gofmt -s -w .