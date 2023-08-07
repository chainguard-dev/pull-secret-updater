/*
Copyright 2023 Chainguard, Inc.
SPDX-License-Identifier: Apache-2.0
*/

package secret

import (
	"context"

	kubeclient "knative.dev/pkg/client/injection/kube/client"
	secretinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/secret"
	secretreconciler "knative.dev/pkg/client/injection/kube/reconciler/core/v1/secret"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"

	"github.com/imjasonh/pull-secret-updater/pkg/config"
)

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	config.NewStore(ctx).WatchConfigs(cmw) // watch for config changes.

	r := &Reconciler{
		client: kubeclient.Get(ctx).CoreV1(),
	}
	impl := secretreconciler.NewImpl(ctx, r)
	r.enqueueAfter = impl.EnqueueAfter
	secretinformer.Get(ctx).Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))
	return impl
}
