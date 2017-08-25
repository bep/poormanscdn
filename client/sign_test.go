/*
 * Copyright (c) 2017 Salle, Alexandre <atsalle@inf.ufrgs.br>
 * Author: Salle, Alexandre <atsalle@inf.ufrgs.br>
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

package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSign(t *testing.T) {
	method := "GET"
	modified := time.Unix(1424, 0)
	expires := time.Unix(3424, 0)
	p := SigParams{
		Method:   method,
		Path:     "somepath",
		UserHost: "someuser",
		Domain:   "somedomain",
		Modified: modified,
		Expires:  expires,
	}
	secret := "dummy"
	signedUrl, err := GetSignedUrl(secret, "http://dummyurl.com/", p)
	assert.Nil(t, err)
	p, err = ParseAndAuthenticateSignedUrl(secret, method, signedUrl)
	assert.Nil(t, err)
	assert.True(t, modified.Equal(p.Modified))
	assert.True(t, expires.Equal(p.Expires))
	// TODO: corrupt signedUrl and check that auth fails
}

func TestZeroTime(t *testing.T) {
	assert.Equal(t, "0", timeToStr(ZeroTime()))

	zeroTimeParsed, err := UnixTimeStrToTime("0")
	assert.Nil(t, err)
	assert.True(t, ZeroTime().Equal(zeroTimeParsed))

	zeroTimeParsed, err = UnixTimeStrToTime("")
	assert.Nil(t, err)
	assert.True(t, ZeroTime().Equal(zeroTimeParsed))
}
