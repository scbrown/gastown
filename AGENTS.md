# Agent Instructions

See **CLAUDE.md** for complete agent context and instructions.

This file exists for compatibility with tools that look for AGENTS.md.

> **Recovery**: Run `gt prime` after compaction, clear, or new session

Full context is injected by `gt prime` at session start.

---

## BUILD GUARDRAILS — READ THIS FIRST

**NEVER run `go build` or `go install` on this repo. ALWAYS use `make build` or `make install`.**

Raw `go build` and `go install` skip the Makefile's `-ldflags`, producing a gt binary that:
- Has a **broken mail subsystem** (no version/commit metadata)
- Is **unsigned on macOS** (will be killed by Gatekeeper)
- Will **refuse to run most commands** (fatal BuiltProperly check)

### Correct Build Commands

```bash
make build     # Build gt binary in repo root
make install   # Build and install to ~/.local/bin/gt
make test      # Run tests
```

### WRONG (will produce a broken binary)

```bash
go build ./cmd/gt          # WRONG — missing ldflags
go install ./cmd/gt        # WRONG — missing ldflags
go build -o gt ./cmd/gt    # WRONG — missing ldflags
```

### If You Accidentally Built Wrong

```bash
make install   # Rebuilds properly and installs to the correct location
```

The binary enforces this at runtime: commands will refuse to execute if
the binary was not built via `make build`.
