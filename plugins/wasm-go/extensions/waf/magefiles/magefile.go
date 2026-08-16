package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/magefile/mage/sh"
	"github.com/tetratelabs/wabin/binary"
	"github.com/tetratelabs/wabin/wasm"
)

var minGoVersion = "1.19"
var tinygoMinorVersion = "0.28"
var Default = Build

const (
	defaultInitialPages = uint32(2100)
	maxWasmMemoryPages  = uint64(65536)
)

func init() {
	for _, check := range []func() error{
		checkTinygoVersion,
		checkGoVersion,
	} {
		if err := check(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	}
}

// checkGoVersion checks the minimum version of Go is supported.
func checkGoVersion() error {
	v, err := sh.Output("go", "version")
	if err != nil {
		return fmt.Errorf("unexpected go error: %v", err)
	}

	// Version can/cannot include patch version e.g.
	// - go version go1.19 darwin/arm64
	// - go version go1.19.2 darwin/amd64
	versionRegex := regexp.MustCompile("go([0-9]+).([0-9]+).?([0-9]+)?")
	compare := versionRegex.FindStringSubmatch(v)
	if len(compare) != 4 {
		return fmt.Errorf("unexpected go semver: %q", v)
	}
	compare = compare[1:]
	if compare[2] == "" {
		compare[2] = "0"
	}

	base := strings.SplitN(minGoVersion, ".", 3)
	if len(base) == 2 {
		base = append(base, "0")
	}
	for i := 0; i < 3; i++ {
		baseN, _ := strconv.Atoi(base[i])
		compareN, _ := strconv.Atoi(compare[i])
		if baseN > compareN {
			return fmt.Errorf("unexpected go version, minimum want %q, have %q", minGoVersion, strings.Join(compare, "."))
		}
	}
	return nil
}

// checkTinygoVersion checks that exactly the right tinygo version is supported because
// tinygo isn't stable yet.
func checkTinygoVersion() error {
	v, err := sh.Output("tinygo", "version")
	if err != nil {
		return fmt.Errorf("unexpected tinygo error: %v", err)
	}

	// Assume a dev build is valid.
	if strings.Contains(v, "-dev") {
		return nil
	}

	if !strings.HasPrefix(v, fmt.Sprintf("tinygo version %s", tinygoMinorVersion)) {
		return fmt.Errorf("unexpected tinygo version, wanted %s", tinygoMinorVersion)
	}

	return nil
}

// Build builds the Coraza wasm plugin.
func Build() error {
	if err := os.MkdirAll("local", 0755); err != nil {
		return err
	}

	buildTags := []string{"custommalloc", "no_fs_access"}
	if os.Getenv("TIMING") == "true" {
		buildTags = append(buildTags, "timing", "proxywasm_timing")
	}
	if os.Getenv("MEMSTATS") == "true" {
		buildTags = append(buildTags, "memstats")
	}

	buildTagArg := fmt.Sprintf("-tags='%s'", strings.Join(buildTags, " "))

	// ~100MB initial heap
	initialPages := defaultInitialPages
	if ipEnv := os.Getenv("INITIAL_PAGES"); ipEnv != "" {
		ip, err := strconv.ParseUint(ipEnv, 10, 32)
		if err != nil || ip == 0 || ip > maxWasmMemoryPages {
			return fmt.Errorf("invalid INITIAL_PAGES %q: must be between 1 and %d", ipEnv, maxWasmMemoryPages)
		}
		initialPages = uint32(ip)
	}

	if err := sh.RunV("tinygo", "build", "-gc=custom", "-opt=2", "-o", filepath.Join("local", "mainraw.wasm"), "-scheduler=none", "-target=wasi", buildTagArg); err != nil {
		return err
	}

	if err := patchWasm(filepath.Join("local", "mainraw.wasm"), filepath.Join("local", "main.wasm"), initialPages); err != nil {
		return err
	}

	if err := sh.RunV("rm", filepath.Join("local", "mainraw.wasm")); err != nil {
		return err
	}

	return nil
}

func patchWasm(inPath, outPath string, initialPages uint32) error {
	raw, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	mod, err := binary.DecodeModule(raw, wasm.CoreFeaturesV2)
	if err != nil {
		return err
	}

	mod.MemorySection.Min = initialPages

	for _, imp := range mod.ImportSection {
		switch {
		case imp.Name == "fd_filestat_get":
			imp.Name = "fd_fdstat_get"
		case imp.Name == "path_filestat_get":
			imp.Module = "env"
			imp.Name = "proxy_get_header_map_value"
		}
	}

	out := binary.EncodeModule(mod)
	if err = os.WriteFile(outPath, out, 0644); err != nil {
		return err
	}

	return nil
}
