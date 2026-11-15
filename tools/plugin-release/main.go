// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: plugin-release <validate-catalog|validate-console-recovery|capture-bootstrap-evidence|bootstrap-snapshot|plan|apply-plan|render-snapshot|migration-preflight|verify-snapshot|verify-pulled-plugin|append-emergency-lineage|semver-compare|emergency-input-hash> [flags]")
	}
	var err error
	switch os.Args[1] {
	case "validate-catalog":
		err = commandValidate(os.Args[2:])
	case "validate-console-recovery":
		err = commandValidateConsoleRecovery(os.Args[2:])
	case "plan":
		err = commandPlan(os.Args[2:])
	case "capture-bootstrap-evidence":
		err = commandCaptureBootstrapEvidence(os.Args[2:])
	case "bootstrap-snapshot":
		err = commandBootstrap(os.Args[2:])
	case "apply-plan":
		err = commandApply(os.Args[2:])
	case "render-snapshot":
		err = commandRender(os.Args[2:])
	case "migration-preflight":
		err = commandMigrationPreflight(os.Args[2:])
	case "verify-snapshot":
		err = commandVerify(os.Args[2:])
	case "verify-pulled-plugin":
		err = commandVerifyPulledPlugin(os.Args[2:])
	case "append-emergency-lineage":
		err = commandAppendEmergencyLineage(os.Args[2:])
	case "semver-compare":
		err = commandCompare(os.Args[2:])
	case "emergency-input-hash":
		err = commandEmergencyInputHash(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "plugin-release: "+format+"\n", args...)
	os.Exit(1)
}
