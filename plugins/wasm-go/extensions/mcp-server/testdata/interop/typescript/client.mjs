// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { Client, StreamableHTTPClientTransport } from '@modelcontextprotocol/client';

const protocolVersion = '2026-07-28';
const root = process.env.MCP_INTEROP_ROOT;
if (!root) {
  throw new Error('MCP_INTEROP_ROOT is required');
}

for (const path of ['direct', 'proxy-modern', 'proxy-legacy']) {
  const client = new Client(
    { name: 'higress-typescript-interop', version: '1.0.0' },
    { versionNegotiation: { mode: { pin: protocolVersion } } },
  );
  const transport = new StreamableHTTPClientTransport(new URL(`${root}/${path}`));
  try {
    await client.connect(transport);
    if (client.getProtocolEra() !== 'modern') {
      throw new Error(`legacy fallback detected: era=${client.getProtocolEra()}`);
    }
    if (client.getNegotiatedProtocolVersion() !== protocolVersion) {
      throw new Error(`negotiated ${client.getNegotiatedProtocolVersion()}, want ${protocolVersion}`);
    }
    if (!client.getDiscoverResult()?.supportedVersions?.includes(protocolVersion)) {
      throw new Error('server/discover result did not advertise the pinned version');
    }

    const listed = await client.listTools();
    if (listed.tools.length !== 1 || listed.tools[0].name !== 'get_weather') {
      throw new Error(`unexpected tools/list result: ${JSON.stringify(listed)}`);
    }
    const called = await client.callTool({
      name: 'get_weather',
      arguments: { location: 'New York' },
    });
    if (!called.content.some(item => item.type === 'text' && item.text.includes('weather for New York'))) {
      throw new Error(`tools/call omitted deterministic text: ${JSON.stringify(called)}`);
    }
    console.log(`typescript-client 2.0.0: ${path} negotiated, listed, and called successfully`);
  } finally {
    await client.close();
  }
}
