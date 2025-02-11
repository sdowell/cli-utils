// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

type Strategy string

const (
	NameStrategy  Strategy = "name"
	LabelStrategy Strategy = "label"
)
