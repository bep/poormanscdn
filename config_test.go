/*
 * Copyright (c) 2017 Salle, Alexandre <atsalle@inf.ufrgs.br>
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of
 * this software and associated documentation files (the "Software"), to deal in
 * the Software without restriction, including without limitation the rights to
 * use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
 * the Software, and to permit persons to whom the Software is furnished to do so,
 * subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
 * FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
 * COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
 * IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
 * CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetConfig(t *testing.T) {
	for _, secretsFromEnv := range []bool{false, true} {
		name := fmt.Sprintf("secretsFromEnv=%t", secretsFromEnv)
		t.Run(name, func(t *testing.T) {
			doTestGetConfig(t, secretsFromEnv)
		})
	}
}

func doTestGetConfig(t *testing.T, secretsFromEnv bool) {
	assert := require.New(t)

	secrets := ""

	if !secretsFromEnv {
		secrets = `
"DefaultAccessKey":  "your3accesskey",
"DefaultSecretKey":  "your3secret",
"Secret": "yourSecretKey",
`
	}

	basicConf := fmt.Sprintf(`
{
	"Listen": 		":8080",
	"TmpDir":       "tmp", 
	"CacheDir":     "cache",
%s
	"CacheSize":    40000000000,
	"DatabaseDir": "db",
	"FreeSpaceBatchSizeInBytes": 2000000000,
	"TLSCertificateDir": "cert",
	"SigRequired":	false,
	"Hosts": {
		"example.org": { "Bucket": "firstbucket", "Path": "site1" },
		"example.com": { "Bucket": "firstbucket", "Path": "site2" },
		"example.net": { "Bucket": "secondbucket", "Path": "", "AccessKey": "example.net.access" }
	}
}
`, secrets)

	// Set environment vars as OS env vars to assert they're not used.
	// OS env vars will only come into play if not set in the JSON config.
	os.Setenv(awsAccessKeyEnv, "awsAccessEnv")
	os.Setenv(awsSecretAccessKeyEnv, "awsSecretEnv")
	os.Setenv(pcdnSecretEnv, "pcdnSecretEnv")

	conf, err := readConfiguration(strings.NewReader(basicConf))
	assert.NoError(err)

	assert.Equal("cache", conf.CacheDir)
	assert.Equal("cert", conf.TLSCertificateDir)

	if secretsFromEnv {
		assert.Equal("awsAccessEnv", conf.DefaultAccessKey)
		assert.Equal("awsSecretEnv", conf.DefaultSecretKey)
		assert.Equal("pcdnSecretEnv", conf.Secret)
	} else {
		assert.Equal("your3accesskey", conf.DefaultAccessKey)
		assert.Equal("your3secret", conf.DefaultSecretKey)
		assert.Equal("yourSecretKey", conf.Secret)
	}

	assert.Len(conf.Hosts, 3)

	firstHost := conf.Hosts["example.org"]
	// Should inherit the defaults
	assert.Equal(conf.DefaultAccessKey, firstHost.AccessKey)
	assert.Equal(conf.DefaultSecretKey, firstHost.SecretKey)

	lastHost := conf.Hosts["example.net"]
	assert.Equal("example.net.access", lastHost.AccessKey)
	assert.Equal(conf.DefaultSecretKey, lastHost.SecretKey)

	assert.Equal([]string{"example.com", "example.net", "example.org"}, conf.hostNames())

}
