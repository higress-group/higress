// Copyright (c) 2022 Alibaba Group Holding Ltd.
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

package main

import (
	"math/rand"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"

	"github.com/higress-group/wasm-go/pkg/wrapper"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"custom-log",
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
	)
}

type CustomLogConfig struct {
}

// Method 1: write custom log
func writeLog(ctx wrapper.HttpContext) {
	ctx.SetUserAttribute("question", "当然可以。在Python中，你可以创建一个函数来计算一系列数字的和。下面是一个简单的例子，该函数接受一个数字列表作为输入，并返回它们的总和。\n\n```python\ndef sum_of_numbers(numbers):\n    \"\"\"\n    计算列表中所有数字的和。\n    \n    参数:\n    numbers (list of int or float): 一个包含数字的列表。\n    \n    返回:\n    int or float: 列表中所有数字的总和。\n    \"\"\"\n    total_sum = sum(numbers)  # 使用Python内置的sum函数计算总和\n    return total_sum\n\n# 示例使用\nnumbers_list = [1, 2, 3, 4, 5]\nprint(\"The sum is:\", sum_of_numbers(numbers_list))  # 输出：The sum is: 15\n```\n\n在这段代码中，我们定义了一个名为 `sum_of_numbers` 的函数，它接收一个参数 `numbers`，这是一个包含整数或浮点数的列表。函数内部使用了Python的内置函数 `sum()` 来计算这些数字的总和，并将结果返回。\n\n你也可以手动实现求和逻辑，而不是使用内置的 `sum()` 函数，如下所示：\n\n```python\ndef sum_of_numbers_manual(numbers):\n    \"\"\"\n    手动计算列表中所有数字的和。\n    \n    参数:\n    numbers (list of int or float): 一个包含数字的列表。\n    \n    返回:\n    int or float: 列表中所有数字的总和。\n    \"\"\"\n    total_sum = 0\n    for number in numbers:\n        total_sum += number\n    return total_sum\n\n# 示例使用\nnumbers_list = [1, 2, 3, 4, 5]\nprint(\"The sum is:\", sum_of_numbers_manual(numbers_list))  # 输出：The sum is: 15\n```\n\n在这个版本中，我们初始化 `total_sum` 为0，然后遍历列表中的每个元素，并将其加到 `total_sum` 上。最后返回这个累加的结果。这两种方法都可以达到相同的目的，但是使用内置函数通常更简洁且效率更高。")
	ctx.SetUserAttribute("k2", 2213.22)
	ctx.WriteUserAttributeToLog()
}

// Methods 2: write custom log with specific key
func writeLogWithKey(ctx wrapper.HttpContext, key string) {
	ctx.SetUserAttribute("k2", 2213.22)
	_ = ctx.WriteUserAttributeToLogWithKey(key)
	ctx.SetUserAttribute("k2", 212939.22)
	ctx.SetUserAttribute("k3", 123)
	_ = ctx.WriteUserAttributeToLogWithKey(key)
}

// Methods 2: write custom log with specific key
func writeTraceAttribute(ctx wrapper.HttpContext) {
	ctx.SetUserAttribute("question", "当然可以。在Python中，你可以创建一个函数来计算一系列数字的和。下面是一个简单的例子，该函数接受一个数字列表作为输入，并返回它们的总和。\n\n```python\ndef sum_of_numbers(numbers):\n    \"\"\"\n    计算列表中所有数字的和。\n    \n    参数:\n    numbers (list of int or float): 一个包含数字的列表。\n    \n    返回:\n    int or float: 列表中所有数字的总和。\n    \"\"\"\n    total_sum = sum(numbers)  # 使用Python内置的sum函数计算总和\n    return total_sum\n\n# 示例使用\nnumbers_list = [1, 2, 3, 4, 5]\nprint(\"The sum is:\", sum_of_numbers(numbers_list))  # 输出：The sum is: 15\n```\n\n在这段代码中，我们定义了一个名为 `sum_of_numbers` 的函数，它接收一个参数 `numbers`，这是一个包含整数或浮点数的列表。函数内部使用了Python的内置函数 `sum()` 来计算这些数字的总和，并将结果返回。\n\n你也可以手动实现求和逻辑，而不是使用内置的 `sum()` 函数，如下所示：\n\n```python\ndef sum_of_numbers_manual(numbers):\n    \"\"\"\n    手动计算列表中所有数字的和。\n    \n    参数:\n    numbers (list of int or float): 一个包含数字的列表。\n    \n    返回:\n    int or float: 列表中所有数字的总和。\n    \"\"\"\n    total_sum = 0\n    for number in numbers:\n        total_sum += number\n    return total_sum\n\n# 示例使用\nnumbers_list = [1, 2, 3, 4, 5]\nprint(\"The sum is:\", sum_of_numbers_manual(numbers_list))  # 输出：The sum is: 15\n```\n\n在这个版本中，我们初始化 `total_sum` 为0，然后遍历列表中的每个元素，并将其加到 `total_sum` 上。最后返回这个累加的结果。这两种方法都可以达到相同的目的，但是使用内置函数通常更简洁且效率更高。")
	ctx.SetUserAttribute("k2", 2213.22)
	ctx.WriteUserAttributeToTrace()
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config CustomLogConfig) types.Action {
	if rand.Intn(10)%3 == 1 {
		writeLog(ctx)
	} else if rand.Intn(10)%3 == 2 {
		writeLogWithKey(ctx, "ai_log")
	} else {
		writeTraceAttribute(ctx)
	}
	return types.ActionContinue
}
