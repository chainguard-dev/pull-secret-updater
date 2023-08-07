/*
Copyright 2023 Chainguard, Inc.
SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"knative.dev/pkg/injection/sharedmain"

	"github.com/imjasonh/pull-secret-updater/pkg/reconciler/secret"
)

func main() {
	sharedmain.Main("controller",
		secret.NewController,
	)
}
