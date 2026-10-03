package main

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"fmt"
)

// checkBinary says whether data is an executable for t: the right kind of file for the OS (ELF, Mach-O or PE) and the right CPU.
// A binary built with the wrong GOOS or GOARCH, or a file that is not an executable at all, is an error.
func checkBinary(data []byte, t target) error {
	r := bytes.NewReader(data)
	switch t.goos {
	case "linux":
		f, err := elf.NewFile(r)
		if err != nil {
			return fmt.Errorf("%s: not an ELF file: %w", t, err)
		}
		want := map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}[t.goarch]
		if f.Machine != want {
			return fmt.Errorf("%s: the CPU is %v, want %v", t, f.Machine, want)
		}
	case "darwin":
		f, err := macho.NewFile(r)
		if err != nil {
			return fmt.Errorf("%s: not a Mach-O file: %w", t, err)
		}
		want := map[string]macho.Cpu{"amd64": macho.CpuAmd64, "arm64": macho.CpuArm64}[t.goarch]
		if f.Cpu != want {
			return fmt.Errorf("%s: the CPU is %v, want %v", t, f.Cpu, want)
		}
	case "windows":
		f, err := pe.NewFile(r)
		if err != nil {
			return fmt.Errorf("%s: not a PE file: %w", t, err)
		}
		want := map[string]uint16{"amd64": pe.IMAGE_FILE_MACHINE_AMD64, "arm64": pe.IMAGE_FILE_MACHINE_ARM64}[t.goarch]
		if f.Machine != want {
			return fmt.Errorf("%s: the CPU is %#x, want %#x", t, f.Machine, want)
		}
	default:
		return fmt.Errorf("%s: no check for this OS", t)
	}
	return nil
}

// looksExecutable reports whether data begins like an ELF, Mach-O or PE file. The .vsix must not carry the srwr binary
// (the decision of R11), and this finds one whatever it is called.
func looksExecutable(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	switch {
	case bytes.HasPrefix(data, []byte("\x7fELF")):
		return true
	case bytes.HasPrefix(data, []byte("MZ")):
		return true
	}
	magic := [][]byte{{0xfe, 0xed, 0xfa, 0xce}, {0xfe, 0xed, 0xfa, 0xcf}, {0xce, 0xfa, 0xed, 0xfe}, {0xcf, 0xfa, 0xed, 0xfe}, {0xca, 0xfe, 0xba, 0xbe}}
	for _, m := range magic {
		if bytes.HasPrefix(data, m) {
			return true
		}
	}
	return false
}
