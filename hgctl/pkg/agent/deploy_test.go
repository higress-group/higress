// Copyright (c) 2025 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// setHomeWithAgent points HOME at a temporary directory holding just what
// util.GetSpecificAgentDir looks for, and reports the agent directory back so a
// caller can drop files into it.
func setHomeWithAgent(t *testing.T, name string, files ...string) string {
	t.Helper()
	home := t.TempDir()
	agentDir := filepath.Join(home, ".hgctl", "agents", name)
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("create agent dir: %v", err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(agentDir, f), []byte("fastapi\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	t.Setenv("HOME", home)
	return agentDir
}

func TestGetAgentTypeKeepsAnExplicitDeployType(t *testing.T) {
	cases := []struct {
		desc   string
		preset DeployType
		files  []string
		want   DeployType
	}{
		{
			// `hgctl agent deploy demo --agentrun` sets Type before Deploy() runs, and an agent
			// created without a requirements.txt has nothing to infer from. Type used to be
			// overwritten here, so the flag decided the *message* the CLI printed and nothing
			// else: the agent went out through the local path anyway.
			desc:   "agentrun is kept when the agent has no requirements.txt",
			preset: AgentRun,
			want:   AgentRun,
		},
		{
			// Same precedence in the other direction: a requirements.txt in the agent directory is
			// no evidence about what the operator asked for, so local stays local.
			desc:   "local is kept even when the agent has a requirements.txt",
			preset: Local,
			files:  []string{"requirements.txt"},
			want:   Local,
		},
		{
			desc:  "without a deploy type, requirements.txt still means agentrun",
			files: []string{"requirements.txt"},
			want:  AgentRun,
		},
		{
			desc: "without a deploy type, no requirements.txt still means local",
			want: Local,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			agentDir := setHomeWithAgent(t, "demo", tc.files...)
			h := &DeployHandler{Name: "demo", Type: tc.preset}

			if err := h.getAgentType(); err != nil {
				t.Fatalf("getAgentType() returned an error: %v", err)
			}
			if h.Type != tc.want {
				t.Errorf("getAgentType() left Type = %q, want %q", h.Type, tc.want)
			}
			// The early return must not skip the directory lookup: Deploy() reads AgentDir for
			// every later step, including the environment check and the install commands.
			if h.AgentDir != agentDir {
				t.Errorf("getAgentType() set AgentDir = %q, want %q", h.AgentDir, agentDir)
			}
		})
	}
}
