// Package protonnode is the Android-facing root of the Go core.
//
// P0 scope: the module exists so that `cd core && go test ./...` is a real,
// runnable gate from the first commit. The gomobile contract from PROMPT.md
// section 5.1 (Init/Login/Start/Stop/Status/...) is implemented in P1 on top of
// the code audited in docs/P0-AUDIT.md; nothing is stubbed here on purpose,
// because a stub that pretends to work is worse than an absent function.
//
// Build constraints for the ported code:
//   - GOOS=android, GOARCH=arm64, CGO off (gomobile handles the cgo shim).
//   - no os/exec, no console, no DPAPI, no wintun, no TUN device.
package protonnode
