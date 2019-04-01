/*
	Copyright 2019 Andrew C. Young <andrew@vaelen.org>

	This file is part of Serial2Network.

	Serial2Network is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	Serial2Network is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with Serial2Network.  If not, see <https://www.gnu.org/licenses/>.
*/

package serial2network

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestCreateSplitFunc(t *testing.T) {
	var input string

	input = "one\ntwo\rthree\r\nfour\nfive\rsix\r\nseven\reight"

	testSplit(t, input, "\n")
	testSplit(t, input, "\r")
	testSplit(t, input, "\r\n")

}

func testSplit(t *testing.T, input string, lineEnding string) {
	f := createSplitFunc([]byte(lineEnding))
	results := split(f, input)
	expected := strings.Split(input, lineEnding)
	if !equal(results, expected) {
		t.Logf("createSplitFunc() failed.  Line Ending: %q, Input: %q, Expected: %q, Got: %q\n", lineEnding, input, expected, results)
		t.Fail()
	}
}

func split(f bufio.SplitFunc, input string) []string {
	results := make([]string, 0)
	s := bufio.NewScanner(bytes.NewReader([]byte(input)))
	s.Split(f)
	for s.Scan() {
		results = append(results, s.Text())
	}
	return results
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
