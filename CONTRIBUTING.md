# Contributing to callonce-go

Thanks for your interest in contributing!

## Getting started

1. Fork and clone the repo.
2. Make sure tests pass before making changes:

```sh
go test -race -v ./...
```

## Making changes

- Keep changes focused. One feature or fix per PR.
- Format your code with `gofmt`
- Run `go vet ./...` before committing.
- Add or update tests for any new behaviour.

## Submitting a pull request

1. Create a branch from `main`.
2. Commit your changes with a clear message.
3. Open a PR against `main` and describe what it does.

## Code style

This project follows standard Go conventions. If `gofmt` and `go vet` are happy, you're good.
