# Contributing to Flipswitch Go SDK

Thank you for your interest in contributing to the Flipswitch Go SDK!

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/go-sdk.git`
3. Create a feature branch: `git checkout -b feature/your-feature`
4. Make your changes
5. Run tests: `go test ./...`
6. Commit your changes: `git commit -m "Add your feature"`
7. Push to the branch: `git push origin feature/your-feature`
8. Create a Pull Request

## Development Setup

```bash
# Download dependencies
go mod tidy

# Build
go build ./...

# Run tests
go test ./...

# Run vet
go vet ./...
```

## Code Style

- Follow standard Go conventions
- Run `go fmt` before committing
- Ensure `go vet` passes

## Pull Request Guidelines

- Keep changes focused and atomic
- Write clear commit messages
- Include tests for new functionality
- Update documentation as needed
- Ensure all tests pass

## Reporting Issues

Please use GitHub Issues to report bugs or request features. Include:
- Go version
- Operating system
- Steps to reproduce
- Expected vs actual behavior

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
