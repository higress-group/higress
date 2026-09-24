// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package protocol

import "slices"

// Era identifies the independently validated MCP protocol family.
type Era uint8

const (
	EraLegacy Era = iota
	EraModern
)

// Version is an MCP protocol version carried by a request.
type Version string

const (
	Version20241105 Version = "2024-11-05"
	Version20250326 Version = "2025-03-26"
	Version20250618 Version = "2025-06-18"
	Version20260728 Version = "2026-07-28"
)

var legacyVersions = []Version{
	Version20241105,
	Version20250326,
	Version20250618,
}

// LegacyVersions returns a copy of the versions negotiated by legacy
// initialize. The modern version is intentionally excluded.
func LegacyVersions() []Version {
	return slices.Clone(legacyVersions)
}

// SupportedVersions returns every independently supported protocol profile.
// The modern version remains excluded from legacy initialize negotiation.
func SupportedVersions() []Version {
	versions := slices.Clone(legacyVersions)
	return append(versions, Version20260728)
}

func IsLegacyVersion(version Version) bool {
	return slices.Contains(legacyVersions, version)
}

func IsModernVersion(version Version) bool {
	return version == Version20260728
}
