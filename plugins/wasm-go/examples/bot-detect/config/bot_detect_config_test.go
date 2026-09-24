/*
 * Copyright (c) 2022 Alibaba Group Holding Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"log"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func toRegexMatch(regexs []string) []*regexp.Regexp {
	re := make([]*regexp.Regexp, 0)
	for _, regex := range regexs {
		c, err := regexp.Compile(regex)
		if err != nil {
			log.Default().Fatal(err.Error())
		}
		re = append(re, c)
	}
	return re
}

func TestBotDetectConfig_ProcessTest(t *testing.T) {

	tests := []struct {
		name         string
		ua           string
		allow        []string
		deny         []string
		blockCode    uint32
		blockMessage string
		want         bool
	}{
		{
			"test empty bot detect",
			"",
			[]string{},
			[]string{},
			401,
			"bot has been blocked",
			false,
		},
		{
			"test default bot detect",
			"Ant-Tailsweep-1",
			[]string{},
			[]string{},
			401,
			"bot has been blocked",
			false,
		},
		{
			"test default bot detect",
			"indexer/1.2",
			[]string{},
			[]string{},
			401,
			"bot has been blocked",
			false,
		},
		{
			"test default bot detect",
			"indexer/1.1.0",
			[]string{},
			[]string{},
			401,
			"bot has been blocked",
			false,
		},
		{
			"test default bot detect",
			"YottaaMonitor",
			[]string{},
			[]string{},
			401,
			"bot has been blocked",
			false,
		},
		{
			"test allow bot detect",
			"BaiduMobaider",
			[]string{"BaiduMobaider"},
			[]string{},
			401,
			"bot has been blocked",
			true,
		},
		{
			"test deny bot detect",
			"Chrome",
			[]string{},
			[]string{"Chrome"},
			401,
			"bot has been blocked",
			false,
		},
		{
			"test allow and deny bot detect",
			"SameBotDetect",
			[]string{"SameBotDetect"},
			[]string{"SameBotDetect"},
			401,
			"bot has been blocked",
			true,
		},
	}

	for _, test := range tests {

		t.Run(test.name, func(t *testing.T) {
			bdc := BotDetectConfig{
				BlockedCode:    test.blockCode,
				BlockedMessage: test.blockMessage,
				Allow:          toRegexMatch(test.allow),
				Deny:           toRegexMatch(test.deny),
			}
			actual, _ := bdc.Process(test.ua)
			assert.Equal(t, test.want, actual, "")
		})

	}

}
