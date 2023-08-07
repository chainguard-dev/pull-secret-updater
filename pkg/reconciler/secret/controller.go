/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package secret

import (
	"context"

	"github.com/imjasonh/pull-secret-updater/pkg/config"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"

	secretinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/secret"
	secretreconciler "knative.dev/pkg/client/injection/kube/reconciler/core/v1/secret"
)

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	r := &Reconciler{}

	config.NewStore(ctx).WatchConfigs(cmw) // watch for config changes.

	impl := secretreconciler.NewImpl(ctx, r)
	r.enqueueAfter = impl.EnqueueAfter
	secretinformer.Get(ctx).Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))
	return impl
}
