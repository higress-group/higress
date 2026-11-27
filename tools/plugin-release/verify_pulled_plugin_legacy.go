// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"errors"
	"fmt"
)

// validateLegacyProxyWasmModule validates the proxy ABI of a module that
// wazero can decode but cannot compile because it uses the legacy exception
// handling encoding. The pinned C++ toolchain emits that encoding for oauth;
// rewriting the artifact would change its reviewed bytes, so the release gate
// checks the module structure and ABI directly instead of compiling it. The
// normal wazero compile path remains the primary validator for every other
// module.
func validateLegacyProxyWasmModule(data []byte) error {
	if len(data) < 8 || !bytes.Equal(data[:4], []byte{0x00, 0x61, 0x73, 0x6d}) || !bytes.Equal(data[4:8], []byte{0x01, 0x00, 0x00, 0x00}) {
		return errors.New("invalid WebAssembly header")
	}

	var types []wasmFuncType
	var importedFuncTypes []uint32
	var definedFuncTypes []uint32
	var importedMemories uint32
	var definedMemories uint32
	var exports []wasmExport

	offset := 8
	for offset < len(data) {
		sectionID := data[offset]
		cursor := &wasmCursor{data: data, offset: offset + 1}
		sectionSize, err := cursor.uleb()
		if err != nil {
			return fmt.Errorf("section %d size: %w", sectionID, err)
		}
		payload, err := cursor.bytes(sectionSize)
		if err != nil {
			return fmt.Errorf("section %d payload: %w", sectionID, err)
		}
		offset = cursor.offset
		if err := parseLegacyWasmSection(sectionID, payload, &types, &importedFuncTypes, &definedFuncTypes, &importedMemories, &definedMemories, &exports); err != nil {
			return err
		}
	}

	funcType := func(index uint32) (wasmFuncType, error) {
		if index < uint32(len(importedFuncTypes)) {
			typeIndex := importedFuncTypes[index]
			if typeIndex >= uint32(len(types)) {
				return wasmFuncType{}, errors.New("imported function type index is out of range")
			}
			return types[typeIndex], nil
		}
		definedIndex := index - uint32(len(importedFuncTypes))
		if definedIndex >= uint32(len(definedFuncTypes)) {
			return wasmFuncType{}, errors.New("function export index is out of range")
		}
		typeIndex := definedFuncTypes[definedIndex]
		if typeIndex >= uint32(len(types)) {
			return wasmFuncType{}, errors.New("defined function type index is out of range")
		}
		return types[typeIndex], nil
	}

	exportedFunctions := map[string]wasmFuncType{}
	exportedMemory := false
	for _, export := range exports {
		switch export.kind {
		case 0:
			tp, err := funcType(export.index)
			if err != nil {
				return err
			}
			exportedFunctions[export.name] = tp
		case 2:
			if export.index >= importedMemories+definedMemories {
				return errors.New("memory export index is out of range")
			}
			exportedMemory = true
		}
	}
	if !exportedMemory {
		return errors.New("required exported memory is missing")
	}

	callbackParams := []byte{0x7f, 0x7f}
	callbackResults := []byte{0x7f}
	for _, name := range []string{"proxy_on_vm_start", "proxy_on_configure"} {
		tp, ok := exportedFunctions[name]
		if !ok {
			return fmt.Errorf("required function export %q is missing", name)
		}
		if !bytes.Equal(tp.params, callbackParams) || !bytes.Equal(tp.results, callbackResults) {
			return fmt.Errorf("required function export %q must have signature (i32,i32)->i32", name)
		}
	}
	for name, tp := range exportedFunctions {
		if len(name) > len("proxy_abi_version_0_2_") && name[:len("proxy_abi_version_0_2_")] == "proxy_abi_version_0_2_" && len(tp.params) == 0 && len(tp.results) == 0 {
			return nil
		}
	}
	return errors.New("required proxy_abi_version_0_2_* function export with signature ()->() is missing")
}

type wasmFuncType struct {
	params  []byte
	results []byte
}

type wasmExport struct {
	name  string
	kind  byte
	index uint32
}

type wasmCursor struct {
	data   []byte
	offset int
}

func (c *wasmCursor) byte() (byte, error) {
	if c.offset >= len(c.data) {
		return 0, errors.New("unexpected end of section")
	}
	value := c.data[c.offset]
	c.offset++
	return value, nil
}

func (c *wasmCursor) uleb() (uint32, error) {
	var value uint32
	var shift uint
	for {
		current, err := c.byte()
		if err != nil {
			return 0, err
		}
		value |= uint32(current&0x7f) << shift
		if current&0x80 == 0 {
			return value, nil
		}
		shift += 7
		if shift >= 35 {
			return 0, errors.New("integer representation is too long")
		}
	}
}

func (c *wasmCursor) bytes(length uint32) ([]byte, error) {
	if uint64(length) > uint64(len(c.data)-c.offset) {
		return nil, errors.New("unexpected end of section")
	}
	value := c.data[c.offset : c.offset+int(length)]
	c.offset += int(length)
	return value, nil
}

func (c *wasmCursor) name() (string, error) {
	length, err := c.uleb()
	if err != nil {
		return "", err
	}
	value, err := c.bytes(length)
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func (c *wasmCursor) valueTypes() ([]byte, error) {
	count, err := c.uleb()
	if err != nil {
		return nil, err
	}
	return c.bytes(count)
}

func (c *wasmCursor) limits() error {
	flags, err := c.uleb()
	if err != nil {
		return err
	}
	if _, err := c.uleb(); err != nil {
		return err
	}
	if flags&1 != 0 {
		if _, err := c.uleb(); err != nil {
			return err
		}
	}
	return nil
}

func parseLegacyWasmSection(id byte, payload []byte, types *[]wasmFuncType, importedFuncTypes, definedFuncTypes *[]uint32, importedMemories, definedMemories *uint32, exports *[]wasmExport) error {
	cursor := &wasmCursor{data: payload}
	switch id {
	case 0:
		return nil
	case 1:
		count, err := cursor.uleb()
		if err != nil {
			return fmt.Errorf("type section: %w", err)
		}
		for i := uint32(0); i < count; i++ {
			form, err := cursor.byte()
			if err != nil {
				return fmt.Errorf("type section: %w", err)
			}
			if form != 0x60 {
				return errors.New("type section contains a non-function type")
			}
			params, err := cursor.valueTypes()
			if err != nil {
				return fmt.Errorf("type section parameters: %w", err)
			}
			results, err := cursor.valueTypes()
			if err != nil {
				return fmt.Errorf("type section results: %w", err)
			}
			*types = append(*types, wasmFuncType{params: params, results: results})
		}
	case 2:
		count, err := cursor.uleb()
		if err != nil {
			return fmt.Errorf("import section: %w", err)
		}
		for i := uint32(0); i < count; i++ {
			if _, err := cursor.name(); err != nil {
				return fmt.Errorf("import section module: %w", err)
			}
			if _, err := cursor.name(); err != nil {
				return fmt.Errorf("import section name: %w", err)
			}
			kind, err := cursor.byte()
			if err != nil {
				return fmt.Errorf("import section kind: %w", err)
			}
			switch kind {
			case 0:
				typeIndex, err := cursor.uleb()
				if err != nil {
					return fmt.Errorf("import section function type: %w", err)
				}
				*importedFuncTypes = append(*importedFuncTypes, typeIndex)
			case 1:
				if _, err := cursor.byte(); err != nil {
					return fmt.Errorf("import section table type: %w", err)
				}
				if err := cursor.limits(); err != nil {
					return fmt.Errorf("import section table limits: %w", err)
				}
			case 2:
				if err := cursor.limits(); err != nil {
					return fmt.Errorf("import section memory limits: %w", err)
				}
				*importedMemories = *importedMemories + 1
			case 3:
				if _, err := cursor.byte(); err != nil {
					return fmt.Errorf("import section global type: %w", err)
				}
				if _, err := cursor.byte(); err != nil {
					return fmt.Errorf("import section global mutability: %w", err)
				}
			case 4:
				if _, err := cursor.byte(); err != nil {
					return fmt.Errorf("import section tag attribute: %w", err)
				}
				if _, err := cursor.uleb(); err != nil {
					return fmt.Errorf("import section tag type: %w", err)
				}
			default:
				return errors.New("import section contains an unknown import kind")
			}
		}
	case 3:
		count, err := cursor.uleb()
		if err != nil {
			return fmt.Errorf("function section: %w", err)
		}
		for i := uint32(0); i < count; i++ {
			typeIndex, err := cursor.uleb()
			if err != nil {
				return fmt.Errorf("function section type: %w", err)
			}
			*definedFuncTypes = append(*definedFuncTypes, typeIndex)
		}
	case 5:
		count, err := cursor.uleb()
		if err != nil {
			return fmt.Errorf("memory section: %w", err)
		}
		for i := uint32(0); i < count; i++ {
			if err := cursor.limits(); err != nil {
				return fmt.Errorf("memory section limits: %w", err)
			}
		}
		*definedMemories += count
	case 7:
		count, err := cursor.uleb()
		if err != nil {
			return fmt.Errorf("export section: %w", err)
		}
		for i := uint32(0); i < count; i++ {
			name, err := cursor.name()
			if err != nil {
				return fmt.Errorf("export section name: %w", err)
			}
			kind, err := cursor.byte()
			if err != nil {
				return fmt.Errorf("export section kind: %w", err)
			}
			index, err := cursor.uleb()
			if err != nil {
				return fmt.Errorf("export section index: %w", err)
			}
			*exports = append(*exports, wasmExport{name: name, kind: kind, index: index})
		}
	}
	return nil
}