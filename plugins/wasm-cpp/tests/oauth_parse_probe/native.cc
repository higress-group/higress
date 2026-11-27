// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

#include <fstream>
#include <iostream>
#include <sstream>
#include <vector>

#include "tests/oauth_parse_probe/parse.h"

int main(int argument_count, char** arguments) {
  if (argument_count != 2) {
    return 2;
  }
  std::ifstream fixtures(arguments[1]);
  std::string line;
  int case_count = 0;
  int failures = 0;
  while (std::getline(fixtures, line)) {
    std::istringstream record(line);
    std::vector<std::string> fields;
    std::string field;
    while (std::getline(record, field, '\t')) {
      fields.push_back(field);
    }
    if (fields.size() != 3) {
      return 2;
    }
    const auto result = oauth_v0_probe::evaluate(fields[2]);
    const bool passed = result.outcome == fields[1];
    std::cout << fields[0] << '\t' << fields[1] << '\t' << result.outcome
              << '\t' << result.detail << '\t' << (passed ? "PASS" : "FAIL")
              << '\n';
    failures += passed ? 0 : 1;
    case_count++;
  }
  return case_count > 0 && failures == 0 ? 0 : 1;
}
