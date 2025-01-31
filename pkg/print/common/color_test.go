// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
)

func TestSprintWithColor(t *testing.T) {
	testCases := map[string]struct {
		color          Color
		msg            string
		expectedResult string
	}{
		"no args with color": {
			color:          GREEN,
			msg:            "This is a test",
			expectedResult: "\x1b[32mThis is a test\x1b[0m",
		},
		"with args and color": {
			color:          YELLOW,
			msg:            "sonic youth",
			expectedResult: "\x1b[33msonic youth\x1b[0m",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			result := SprintWithColor(tc.color, tc.msg)
			if want, got := tc.expectedResult, result; want != got {
				t.Errorf("expected %q, but got %q", want, got)
			}
		})
	}
}

func TestColorForStatus(t *testing.T) {
	testCases := map[string]struct {
		status           status.Status
		expectedSetColor bool
		expectedColor    Color
	}{
		"status with color": {
			status:           status.CurrentStatus,
			expectedSetColor: true,
			expectedColor:    GREEN,
		},
		"status without color": {
			status:           status.NotFoundStatus,
			expectedSetColor: false,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			color, setColor := ColorForStatus(tc.status)
			assert.Equal(t, setColor, tc.expectedSetColor)
			if tc.expectedSetColor {
				assert.Equal(t, color, tc.expectedColor)
			}
		})
	}
}
