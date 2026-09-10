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
	"errors"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	wrapper.SetCtx(
		"request-size-limit",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
	)
}

type RequestSizeLimitConfig struct {
	requestSize int64
}

func parseConfig(json gjson.Result, config *RequestSizeLimitConfig, log wrapper.Log) error {
	sizeStr := json.Get("requestSize").String()
	sizeStr = strings.TrimSpace(sizeStr)
	re := regexp.MustCompile("^([+\\-]?\\d+)([a-zA-Z]{0,2})$")
	submatchs := re.FindStringSubmatch(sizeStr)
	if len(submatchs) == 0 {
		config.requestSize = 1024 * 1024
		return nil
	}
	amount, _ := strconv.ParseInt(submatchs[1], 10, 64)
	unitStr := submatchs[2]
	if len(unitStr) == 0 {
		config.requestSize = amount
	} else {
		unitSizeMap := map[string]int64{
			"B":  1,
			"KB": 1024,
			"MB": 1024 * 1024,
			"GB": 1024 * 1024 * 1024,
		}
		unit, ok := unitSizeMap[unitStr]
		if !ok {
			return errors.New("Invalid unit size: " + unitStr)
		}
		config.requestSize = amount * unit
	}
	return nil
}

func onHttpRequestBody(ctx wrapper.HttpContext, config RequestSizeLimitConfig, body []byte, log wrapper.Log) types.Action {
	if int64(len(body)) > config.requestSize {
		proxywasm.SendHttpResponse(413, nil, []byte("The request body is too large"), -1)
		return types.ActionContinue
	}
	return types.ActionContinue
}
