# Contributing to DBSwitcher

First off, thank you for considering contributing to DBSwitcher! It's people like you that make DBSwitcher such a great tool.

## 📋 Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Contributions](#making-contributions)
- [Pull Request Process](#pull-request-process)
- [Issue Reporting](#issue-reporting)
- [Development Guidelines](#development-guidelines)
- [Testing](#testing)

## Code of Conduct

This project and everyone participating in it is governed by our Code of Conduct. By participating, you are expected to uphold this code.

## Getting Started

### Ways to Contribute

- 🐛 **Bug Reports**: Found a bug? Help us fix it!
- ✨ **Feature Requests**: Have an idea? We'd love to hear it!
- 📝 **Documentation**: Help improve our docs
- 💻 **Code**: Submit patches and improvements
- 🧪 **Testing**: Help us test on different platforms
- 🌍 **Translations**: Help make DBSwitcher available in more languages

## Development Setup

### Prerequisites

- **Go 1.19+**: [Download Go](https://golang.org/dl/)
- **Git**: [Download Git](https://git-scm.com/downloads)
- **Platform-specific GUI libraries**:
  - **Linux**: `sudo apt-get install libgtk-3-dev libayatana-appindicator3-dev libwebkit2gtk-4.0-dev`
  - **Windows**: No additional dependencies
  - **macOS**: No additional dependencies

### Setup Instructions

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/MariaDBSwitcher.git
   cd MariaDBSwitcher
   ```

3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/AhmedAredah/MariaDBSwitcher.git
   ```

4. **Install dependencies**:
   ```bash
   go mod download
   ```

5. **Build and test**:
   ```bash
   make build
   make test
   ./dbswitcher version
   ```

## Making Contributions

### Branch Naming

Use descriptive branch names:
- `feature/add-new-configuration-type`
- `bugfix/fix-port-detection-windows`
- `docs/improve-installation-guide`
- `refactor/simplify-credential-management`

### Commit Messages

Follow conventional commit format:
```
type(scope): description

[optional body]

[optional footer]
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Formatting, missing semicolons, etc.
- `refactor`: Code restructuring without changing external behavior
- `test`: Adding missing tests
- `chore`: Maintenance tasks

**Examples:**
```bash
feat(gui): add dark mode support
fix(core): resolve port detection on Windows 11
docs(readme): update installation instructions
refactor(cli): simplify command argument parsing
```

## Pull Request Process

1. **Create a new branch** from `develop`:
   ```bash
   git checkout develop
   git pull upstream develop
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes** following our [development guidelines](#development-guidelines)

3. **Test thoroughly**:
   ```bash
   make test
   make lint
   make build-all  # Test cross-platform builds
   ```

4. **Update documentation** if needed:
   - Update README.md
   - Add/update code comments
   - Update help text

5. **Commit your changes**:
   ```bash
   git add .
   git commit -m "feat(scope): description"
   ```

6. **Push to your fork**:
   ```bash
   git push origin feature/your-feature-name
   ```

7. **Open a Pull Request** on GitHub:
   - Use a clear title and description
   - Reference any related issues
   - Add screenshots for UI changes
   - Ensure all CI checks pass

### Pull Request Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update

## Testing
- [ ] Unit tests pass
- [ ] Manual testing completed
- [ ] Cross-platform testing (if applicable)

## Checklist
- [ ] My code follows the style guidelines
- [ ] I have performed a self-review of my code
- [ ] I have commented my code, particularly in hard-to-understand areas
- [ ] I have made corresponding changes to the documentation
- [ ] My changes generate no new warnings
- [ ] New and existing unit tests pass locally
```

## Issue Reporting

### Bug Reports

When reporting bugs, please include:

```markdown
**Environment:**
- OS: [e.g., Windows 11, Ubuntu 22.04]
- Go version: [e.g., 1.21.0]
- DBSwitcher version: [e.g., v0.0.1]

**Steps to Reproduce:**
1. Step one
2. Step two
3. Step three

**Expected Behavior:**
What you expected to happen

**Actual Behavior:**
What actually happened

**Screenshots:**
If applicable, add screenshots

**Logs:**
Include relevant log output from ~/.config/DBSwitcher/dbswitcher.log
```

### Feature Requests

When requesting features:

```markdown
**Is your feature request related to a problem?**
A clear description of the problem

**Describe the solution you'd like**
A clear description of what you want to happen

**Describe alternatives you've considered**
Any alternative solutions or features you've considered

**Additional context**
Any other context, mockups, or screenshots
```

## Development Guidelines

### Code Style

- **Follow Go conventions**: Use `gofmt`, `go vet`, and `golangci-lint`
- **Write clear code**: Self-documenting code is preferred
- **Add comments**: For complex logic or public APIs
- **Error handling**: Always handle errors appropriately
- **Logging**: Use the `core.AppLogger` for consistent logging

### Architecture

- **Separation of concerns**: Keep core logic, GUI, and CLI separate
- **Interface design**: Use interfaces for testability
- **Error handling**: Return errors, don't panic in library code
- **Configuration**: Use the established configuration system
- **Threading**: Be careful with GUI threading (use `fyne.Do()`)

### File Organization

```
├── core/           # Business logic, no UI dependencies
├── gui/            # GUI-specific code using Fyne
├── cli/            # CLI-specific code
└── main.go         # Application entry point
```

### Adding New Features

1. **Core logic** in `core/` package
2. **CLI interface** in `cli/` package
3. **GUI interface** in `gui/` package
4. **Update help text** and documentation
5. **Add tests** for new functionality

## Testing

### Running Tests

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run specific package tests
go test ./core/...

# Run benchmarks
make bench
```

### Writing Tests

- **Unit tests**: Test individual functions
- **Integration tests**: Test component interaction
- **Table-driven tests**: For multiple test cases
- **Mock external dependencies**: Database connections, file system

Example test:
```go
func TestConfigValidation(t *testing.T) {
    tests := []struct {
        name    string
        config  MariaDBConfig
        wantErr bool
    }{
        {
            name: "valid config",
            config: MariaDBConfig{
                Name: "test",
                Port: "3306",
                Path: "/path/to/config",
            },
            wantErr: false,
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateConfig(tt.config)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Manual Testing

Before submitting:
1. **Test all platforms** you can access
2. **Test both GUI and CLI** interfaces
3. **Test edge cases** and error conditions
4. **Test with real MariaDB instances**

## Release Process

Releases are managed by maintainers, but contributors can help by:
1. **Testing release candidates**
2. **Updating documentation**
3. **Reporting issues** in pre-release versions

## Questions?

- 💬 **Discussions**: Use GitHub Discussions for questions
- 🐛 **Issues**: Use GitHub Issues for bug reports
- 📧 **Email**: Contact Ahmed.Aredah@gmail.com for private matters

## Recognition

Contributors are recognized in:
- GitHub contributors list
- Release notes
- Project README acknowledgments

Thank you for contributing to DBSwitcher! 🚀