//go:build !cgo

package gui

// Run returns ErrGUINotCompiled when the binary was built without CGO.
// This keeps the service/CLI binary buildable without a C toolchain.
func Run(opts Options) error {
	return ErrGUINotCompiled
}
